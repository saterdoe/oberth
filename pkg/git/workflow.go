package git

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	ErrApprovalRequired = errors.New("explicit approval is required")
	ErrDirtyWorktree    = errors.New("worktree contains uncommitted changes")
	ErrNoChanges        = errors.New("worktree has no changes to accept")
	ErrUnsafePath       = errors.New("path escapes repository")
	ErrUserChanges      = errors.New("files changed after the agent edit")
)

type Approval struct {
	Granted bool   `json:"granted"`
	Actor   string `json:"actor"`
	Reason  string `json:"reason,omitempty"`
}

type SessionWorktree struct {
	Repository string `json:"repository"`
	Path       string `json:"path"`
	Branch     string `json:"branch"`
}

type PromotionResult struct {
	Commit string `json:"commit"`
	Branch string `json:"branch"`
}

// CheckPromotionReadiness verifies conditions that can be checked before the
// user asks to promote an isolated worktree. The same checks are repeated by
// promotion itself, because the repository can change between requests.
func CheckPromotionReadiness(worktree SessionWorktree, baseCommit string) error {
	repository, err := DetectRepo(worktree.Repository)
	if err != nil {
		return err
	}
	if !sameDirectory(repository.Root, worktree.Repository) {
		return ErrUnsafePath
	}
	status, err := runCmd(context.Background(), "git", "-C", repository.Root, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(status)) != "" {
		return ErrDirtyWorktree
	}
	if baseCommit != "" {
		current, err := CurrentCommit(repository.Root)
		if err != nil {
			return err
		}
		if current != baseCommit {
			return fmt.Errorf("base branch advanced from %s to %s", baseCommit, current)
		}
	}
	return nil
}

type SessionChangeSet struct {
	Repository string            `json:"repository"`
	Files      map[string]string `json:"files"`
}

var safeSessionID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func CurrentCommit(repository string) (string, error) {
	repo, err := DetectRepo(repository)
	if err != nil {
		return "", err
	}
	output, err := runCmd(context.Background(), "git", "-C", repo.Root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func CreateSessionWorktree(repository, baseDir, sessionID string) (SessionWorktree, error) {
	return CreateSessionWorktreeContext(context.Background(), repository, baseDir, sessionID)
}

func CreateSessionWorktreeContext(parent context.Context, repository, baseDir, sessionID string) (SessionWorktree, error) {
	if !safeSessionID.MatchString(sessionID) || strings.Contains(sessionID, "..") {
		return SessionWorktree{}, fmt.Errorf("invalid session ID")
	}
	repo, err := DetectRepo(repository)
	if err != nil {
		return SessionWorktree{}, err
	}
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return SessionWorktree{}, err
	}
	path := filepath.Join(base, sessionID)
	if _, err := os.Stat(path); err == nil {
		return SessionWorktree{}, fmt.Errorf("worktree path already exists")
	} else if !os.IsNotExist(err) {
		return SessionWorktree{}, err
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		return SessionWorktree{}, err
	}
	branch := "oberth/" + sessionID
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if _, err := runCmd(ctx, "git", "-C", repo.Root, "check-ref-format", "refs/heads/"+branch); err != nil {
		return SessionWorktree{}, fmt.Errorf("invalid worktree branch: %w", err)
	}
	if _, err := runCmd(ctx, "git", "-C", repo.Root, "worktree", "add", "-b", branch, path, "HEAD"); err != nil {
		return SessionWorktree{}, fmt.Errorf("create worktree: %w", err)
	}
	return SessionWorktree{Repository: repo.Root, Path: path, Branch: branch}, nil
}

func CleanupSessionWorktree(worktree SessionWorktree, force bool) error {
	status, err := runCmd(context.Background(), "git", "-C", worktree.Path, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(status)) != "" && !force {
		return ErrDirtyWorktree
	}
	args := []string{"-C", worktree.Repository, "worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, worktree.Path)
	if _, err := runCmd(context.Background(), "git", args...); err != nil {
		return fmt.Errorf("remove worktree: %w", err)
	}
	if _, err := runCmd(context.Background(), "git", "-C", worktree.Repository, "branch", "-D", worktree.Branch); err != nil {
		return fmt.Errorf("remove worktree branch: %w", err)
	}
	return nil
}

// PromoteSessionWorktree commits the isolated changes and applies that commit
// to the registered checkout only after an explicit user approval.
func PromoteSessionWorktree(worktree SessionWorktree, message string, approval Approval) (PromotionResult, error) {
	if err := requireApproval(approval); err != nil {
		return PromotionResult{}, err
	}
	if strings.TrimSpace(message) == "" {
		return PromotionResult{}, fmt.Errorf("commit message is required")
	}
	repository, err := DetectRepo(worktree.Repository)
	if err != nil {
		return PromotionResult{}, err
	}
	isolated, err := DetectRepo(worktree.Path)
	if err != nil {
		return PromotionResult{}, err
	}
	if !sameDirectory(repository.Root, worktree.Repository) ||
		!sameDirectory(isolated.Root, worktree.Path) {
		return PromotionResult{}, ErrUnsafePath
	}
	if err := CheckPromotionReadiness(worktree, ""); err != nil {
		return PromotionResult{}, err
	}
	changed, err := runCmd(context.Background(), "git", "-C", worktree.Path, "status", "--porcelain")
	if err != nil {
		return PromotionResult{}, err
	}
	if strings.TrimSpace(string(changed)) == "" {
		return PromotionResult{}, ErrNoChanges
	}
	if _, err := runCmd(context.Background(), "git", "-C", worktree.Path, "add", "--all"); err != nil {
		return PromotionResult{}, err
	}
	if _, err := runCmd(context.Background(), "git", "-C", worktree.Path, "commit", "-m", message); err != nil {
		return PromotionResult{}, fmt.Errorf("commit isolated changes: %w", err)
	}
	hashOutput, err := runCmd(context.Background(), "git", "-C", worktree.Path, "rev-parse", "HEAD")
	if err != nil {
		return PromotionResult{}, err
	}
	hash := strings.TrimSpace(string(hashOutput))
	if _, err := runCmd(context.Background(), "git", "-C", repository.Root, "cherry-pick", hash); err != nil {
		_, _ = runCmd(context.Background(), "git", "-C", repository.Root, "cherry-pick", "--abort")
		return PromotionResult{}, fmt.Errorf("promote isolated commit: %w", err)
	}
	return PromotionResult{Commit: hash, Branch: worktree.Branch}, nil
}

// sameDirectory compares filesystem identity instead of path spelling. Git and
// the OS may report the same directory through different aliases (for example
// /var vs /private/var on macOS or long vs short paths on Windows).
func sameDirectory(left, right string) bool {
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	if leftErr == nil && rightErr == nil {
		return os.SameFile(leftInfo, rightInfo)
	}

	leftAbs, leftAbsErr := filepath.Abs(left)
	rightAbs, rightAbsErr := filepath.Abs(right)
	return leftAbsErr == nil && rightAbsErr == nil &&
		strings.EqualFold(filepath.Clean(leftAbs), filepath.Clean(rightAbs))
}

func RevertFiles(repository string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("at least one path is required")
	}
	repo, err := DetectRepo(repository)
	if err != nil {
		return nil, err
	}
	clean, err := safeRelativePaths(repo.Root, paths)
	if err != nil {
		return nil, err
	}
	for _, path := range clean {
		tracked, trackErr := runCmd(context.Background(), "git", "-C", repo.Root, "ls-files", "--error-unmatch", "--", path)
		if trackErr == nil && len(tracked) > 0 {
			if _, err := runCmd(context.Background(), "git", "-C", repo.Root, "restore", "--source=HEAD", "--staged", "--worktree", "--", path); err != nil {
				return nil, err
			}
			continue
		}
		target := filepath.Join(repo.Root, filepath.FromSlash(path))
		info, statErr := os.Lstat(target)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return nil, statErr
		}
		if info.IsDir() {
			return nil, fmt.Errorf("refusing to recursively remove untracked directory %q", path)
		}
		if err := os.Remove(target); err != nil {
			return nil, err
		}
	}
	return clean, nil
}

func CaptureSessionChanges(repository string, selected ...string) (SessionChangeSet, error) {
	repo, err := DetectRepo(repository)
	if err != nil {
		return SessionChangeSet{}, err
	}
	paths := selected
	if len(paths) == 0 {
		paths, err = WorkingTreeFiles(repo.Root)
		if err != nil {
			return SessionChangeSet{}, err
		}
	}
	clean, err := safeRelativePaths(repo.Root, paths)
	if err != nil {
		return SessionChangeSet{}, err
	}
	changes := SessionChangeSet{Repository: repo.Root, Files: map[string]string{}}
	for _, path := range clean {
		hash, err := fileState(filepath.Join(repo.Root, filepath.FromSlash(path)))
		if err != nil {
			return SessionChangeSet{}, err
		}
		changes.Files[path] = hash
	}
	return changes, nil
}

func RevertSession(changes SessionChangeSet, approval Approval) ([]string, error) {
	if err := requireApproval(approval); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(changes.Files))
	for path := range changes.Files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	clean, err := safeRelativePaths(changes.Repository, paths)
	if err != nil {
		return nil, err
	}
	for _, path := range clean {
		current, err := fileState(filepath.Join(changes.Repository, filepath.FromSlash(path)))
		if err != nil {
			return nil, err
		}
		if current != changes.Files[path] {
			return nil, fmt.Errorf("%w: %s", ErrUserChanges, path)
		}
	}
	return RevertFiles(changes.Repository, clean)
}

func fileState(path string) (string, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "deleted", nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("unsupported changed file type: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data)), nil
}

func CommitApproved(repository string, paths []string, message string, approval Approval) (string, error) {
	if err := requireApproval(approval); err != nil {
		return "", err
	}
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("commit message is required")
	}
	repo, err := DetectRepo(repository)
	if err != nil {
		return "", err
	}
	clean, err := safeRelativePaths(repo.Root, paths)
	if err != nil {
		return "", err
	}
	args := append([]string{"-C", repo.Root, "add", "--"}, clean...)
	if _, err := runCmd(context.Background(), "git", args...); err != nil {
		return "", fmt.Errorf("git add failed: %w", err)
	}
	if _, err := runCmd(context.Background(), "git", "-C", repo.Root, "commit", "-m", message); err != nil {
		return "", fmt.Errorf("commit failed: %w", err)
	}
	hash, err := runCmd(context.Background(), "git", "-C", repo.Root, "rev-parse", "--short", "HEAD")
	return strings.TrimSpace(string(hash)), err
}

func PushApproved(repository, remote, branch string, approval Approval) error {
	if err := requireApproval(approval); err != nil {
		return err
	}
	if remote == "" || strings.HasPrefix(remote, "-") || strings.ContainsAny(remote, " \t\r\n") {
		return fmt.Errorf("invalid remote")
	}
	repo, err := DetectRepo(repository)
	if err != nil {
		return err
	}
	if _, err := runCmd(context.Background(), "git", "-C", repo.Root, "check-ref-format", "refs/heads/"+branch); err != nil {
		return fmt.Errorf("invalid branch: %w", err)
	}
	if _, err := runCmd(context.Background(), "git", "-C", repo.Root, "remote", "get-url", remote); err != nil {
		return fmt.Errorf("unknown remote: %w", err)
	}
	_, err = runCmd(context.Background(), "git", "-C", repo.Root, "push", "--porcelain", remote, "HEAD:refs/heads/"+branch)
	return err
}

func requireApproval(approval Approval) error {
	if !approval.Granted || strings.TrimSpace(approval.Actor) == "" {
		return ErrApprovalRequired
	}
	return nil
}

func safeRelativePaths(root string, paths []string) ([]string, error) {
	clean := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" || filepath.IsAbs(path) {
			return nil, ErrUnsafePath
		}
		normalized := filepath.Clean(filepath.FromSlash(path))
		if normalized == ".." || strings.HasPrefix(normalized, ".."+string(filepath.Separator)) {
			return nil, ErrUnsafePath
		}
		absolute := filepath.Join(root, normalized)
		rel, err := filepath.Rel(root, absolute)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, ErrUnsafePath
		}
		clean = append(clean, filepath.ToSlash(normalized))
	}
	return clean, nil
}
