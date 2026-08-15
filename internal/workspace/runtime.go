package workspace

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/saterdoe/oberth/internal/permission"
	"github.com/saterdoe/oberth/internal/toolrunner"
	gitpkg "github.com/saterdoe/oberth/pkg/git"
)

type SearchQuery struct {
	Text       string `json:"text"`
	MaxFiles   int    `json:"max_files,omitempty"`
	MaxBytes   int    `json:"max_bytes,omitempty"`
	MaxMatches int    `json:"max_matches,omitempty"`
}

type SearchHit = toolrunner.Match

type Patch struct {
	Path         string `json:"path"`
	Operation    string `json:"operation,omitempty"`
	OldText      string `json:"old_text"`
	NewText      string `json:"new_text"`
	ExpectedHash string `json:"expected_hash,omitempty"`
}

type Command struct {
	Program string        `json:"program"`
	Args    []string      `json:"args,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty"`
}

type CommandResult = toolrunner.CommandResult
type Diff = []gitpkg.DiffFile

type Workspace interface {
	ID() string
	Root() string
	Read(context.Context, string) ([]byte, error)
	Search(context.Context, SearchQuery) ([]SearchHit, error)
	ApplyPatch(context.Context, Patch) (ChangeSet, error)
	Run(context.Context, Command) (CommandResult, error)
	Diff(context.Context) (Diff, error)
	Rollback(context.Context, string) error
}

type Runtime struct {
	id      string
	files   *Service
	reader  *toolrunner.Reader
	runner  *toolrunner.CommandRunner
	mu      sync.Mutex
	changes map[string]ChangeSet
}

func NewRuntime(id, root string, policy *permission.Engine) (*Runtime, error) {
	files, err := New(root)
	if err != nil {
		return nil, err
	}
	guard, err := permission.NewWorkspaceGuard(root)
	if err != nil {
		return nil, err
	}
	if id == "" {
		id = uuid.NewString()
	}
	return &Runtime{
		id: id, files: files,
		reader:  toolrunner.NewReader(root, guard, toolrunner.Limits{MaxFiles: 500, MaxBytes: 256 * 1024, MaxMatches: 100}),
		runner:  toolrunner.NewCommandRunner(root, policy, toolrunner.CommandLimits{Timeout: 5 * time.Minute, MaxOutputBytes: 256 * 1024, AllowedEnv: []string{"CI", "NO_COLOR"}}),
		changes: map[string]ChangeSet{},
	}, nil
}

func (w *Runtime) ID() string   { return w.id }
func (w *Runtime) Root() string { return w.files.Root() }

func (w *Runtime) Read(ctx context.Context, path string) ([]byte, error) {
	data, _, err := w.files.Read(ctx, path)
	return data, err
}

func (w *Runtime) Search(ctx context.Context, query SearchQuery) ([]SearchHit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(query.Text) == "" {
		return nil, fmt.Errorf("search text is required")
	}
	return w.reader.Search(query.Text)
}

func (w *Runtime) ApplyPatch(ctx context.Context, patch Patch) (ChangeSet, error) {
	if patch.Operation == "create" {
		if patch.OldText != "" {
			return ChangeSet{}, fmt.Errorf("create patch cannot include old_text")
		}
		id := uuid.NewString()
		set, err := w.files.Apply(ctx, id, []Change{{Path: patch.Path, Content: []byte(patch.NewText)}})
		if err != nil {
			return ChangeSet{}, err
		}
		w.mu.Lock()
		w.changes[id] = set
		w.mu.Unlock()
		return set, nil
	}
	current, err := w.Read(ctx, patch.Path)
	if err != nil {
		return ChangeSet{}, err
	}
	if strings.Count(string(current), patch.OldText) != 1 {
		return ChangeSet{}, fmt.Errorf("old_text must match exactly once")
	}
	id := uuid.NewString()
	set, err := w.files.Apply(ctx, id, []Change{{
		Path: patch.Path, Content: []byte(strings.Replace(string(current), patch.OldText, patch.NewText, 1)),
		ExpectedHash: patch.ExpectedHash,
	}})
	if err != nil {
		return ChangeSet{}, err
	}
	w.mu.Lock()
	w.changes[id] = set
	w.mu.Unlock()
	return set, nil
}

func (w *Runtime) Run(ctx context.Context, command Command) (CommandResult, error) {
	return w.runner.Run(ctx, toolrunner.Command{Program: command.Program, Args: command.Args, Cwd: w.Root()})
}

func (w *Runtime) Diff(ctx context.Context) (Diff, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return gitpkg.GetDiff(w.Root())
}

func (w *Runtime) Rollback(ctx context.Context, id string) error {
	w.mu.Lock()
	set, ok := w.changes[id]
	w.mu.Unlock()
	if !ok {
		return fmt.Errorf("change set %q not found", id)
	}
	if err := w.files.Rollback(ctx, set); err != nil {
		return err
	}
	w.mu.Lock()
	delete(w.changes, id)
	w.mu.Unlock()
	return nil
}

var _ Workspace = (*Runtime)(nil)
