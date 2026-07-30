package verification

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Execution struct {
	ExitCode int
	Output   string
	Duration time.Duration
}

type Executor interface {
	Execute(context.Context, string, string) Execution
}

type Evidence struct {
	Phase      Phase  `json:"phase"`
	Command    string `json:"command"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output"`
	DurationMS int64  `json:"duration_ms"`
}

type Scorecard struct {
	Status         string     `json:"status"`
	DiffID         string     `json:"diff_id"`
	ProfileVersion string     `json:"profile_version"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        time.Time  `json:"ended_at"`
	Evidence       []Evidence `json:"evidence"`
	Warnings       []string   `json:"warnings"`
	TestsNotRun    bool       `json:"tests_not_run"`
}

type Runner struct {
	executor Executor
	store    string
}

func NewRunner(executor Executor, store string) *Runner {
	return &Runner{executor: executor, store: store}
}

func (r *Runner) Run(ctx context.Context, plan Plan, selected []Phase) (Scorecard, error) {
	score := Scorecard{Status: "READY", DiffID: plan.DiffID, ProfileVersion: plan.ProfileVersion, StartedAt: time.Now(), Evidence: []Evidence{}, Warnings: []string{}}
	selection := map[Phase]bool{}
	for _, phase := range selected {
		selection[phase] = true
	}
	testRun := false
	for _, step := range plan.Steps {
		if len(selection) > 0 && !selection[step.Phase] {
			continue
		}
		if step.Phase == PhaseTest {
			testRun = true
		}
		execution := r.executor.Execute(ctx, step.Command, plan.Root)
		status := "passed"
		if execution.ExitCode != 0 {
			status = "failed"
			score.Status = "NOT_READY"
		}
		score.Evidence = append(score.Evidence, Evidence{step.Phase, step.Command, status, execution.ExitCode, execution.Output, execution.Duration.Milliseconds()})
		if status == "failed" && step.Required {
			break
		}
	}
	score.TestsNotRun = !testRun
	if score.TestsNotRun {
		score.Warnings = append(score.Warnings, "tests were not executed")
	}
	score.EndedAt = time.Now()
	if err := persistScorecard(r.store, score); err != nil {
		return score, err
	}
	return score, nil
}

func persistScorecard(path string, score Scorecard) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(score, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".verification-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(name, path)
}

func LoadScorecard(path string) (Scorecard, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Scorecard{}, err
	}
	var score Scorecard
	if err := json.Unmarshal(data, &score); err != nil {
		return Scorecard{}, err
	}
	return score, nil
}

type Finding struct {
	Category   string `json:"category"`
	Severity   string `json:"severity"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Evidence   string `json:"evidence"`
	Suggestion string `json:"suggestion"`
}

type Review struct {
	Findings []Finding `json:"findings"`
}

func (r Review) Validate() error {
	for index, finding := range r.Findings {
		if finding.Category == "" || finding.Severity == "" || finding.File == "" || finding.Line <= 0 || finding.Evidence == "" || finding.Suggestion == "" {
			return fmt.Errorf("finding %d lacks structured evidence", index)
		}
	}
	return nil
}

type PatchReviewer interface {
	Review(context.Context, string) (Review, error)
}

func ReviewPatch(ctx context.Context, reviewer PatchReviewer, patch string) (Review, error) {
	review, err := reviewer.Review(ctx, patch)
	if err != nil {
		return Review{}, err
	}
	if err := review.Validate(); err != nil {
		return Review{}, err
	}
	return review, nil
}
