package verification

import (
	"os"
	"path/filepath"
)

type Phase string

const (
	PhaseBuild     Phase = "build"
	PhaseTypecheck Phase = "typecheck"
	PhaseLint      Phase = "lint"
	PhaseTest      Phase = "test"
	PhaseSecurity  Phase = "security"
	PhaseDiff      Phase = "diff"
)

type Step struct {
	Phase    Phase  `json:"phase"`
	Command  string `json:"command"`
	Required bool   `json:"required"`
}

type Plan struct {
	Root           string `json:"root"`
	ProfileVersion string `json:"profile_version"`
	DiffID         string `json:"diff_id"`
	Steps          []Step `json:"steps"`
}

func (p Plan) Command(phase Phase) string {
	for _, step := range p.Steps {
		if step.Phase == phase {
			return step.Command
		}
	}
	return ""
}

type Config struct {
	Commands map[Phase]string
}

func Discover(root string, config Config) Plan {
	commands := map[Phase]string{}
	if exists(filepath.Join(root, "go.mod")) {
		commands[PhaseBuild] = "go build ./..."
		commands[PhaseLint] = "go vet ./..."
		commands[PhaseTest] = "go test ./..."
	}
	if exists(filepath.Join(root, "package.json")) {
		commands[PhaseBuild] = "npm run build"
		commands[PhaseLint] = "npm run lint"
		commands[PhaseTest] = "npm test"
		commands[PhaseTypecheck] = "npm run typecheck"
	}
	if exists(filepath.Join(root, "pom.xml")) {
		commands[PhaseBuild] = "mvn package -DskipTests"
		commands[PhaseTest] = "mvn test"
	}
	if exists(filepath.Join(root, "build.gradle")) || exists(filepath.Join(root, "build.gradle.kts")) {
		commands[PhaseBuild] = "gradle assemble"
		commands[PhaseTest] = "gradle test"
	}
	if exists(filepath.Join(root, "pyproject.toml")) || exists(filepath.Join(root, "requirements.txt")) {
		commands[PhaseLint] = "ruff check ."
		commands[PhaseTest] = "pytest"
		commands[PhaseTypecheck] = "mypy ."
	}
	for phase, command := range config.Commands {
		commands[phase] = command
	}
	order := []Phase{PhaseBuild, PhaseTypecheck, PhaseLint, PhaseTest, PhaseSecurity, PhaseDiff}
	plan := Plan{Root: filepath.Clean(root), ProfileVersion: "single-task/v1"}
	for _, phase := range order {
		if command := commands[phase]; command != "" {
			plan.Steps = append(plan.Steps, Step{Phase: phase, Command: command, Required: true})
		}
	}
	return plan
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
