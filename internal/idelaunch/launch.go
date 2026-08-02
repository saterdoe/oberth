package idelaunch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type Command struct {
	Executable string
	Args       []string
}

type Launcher struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Message   string `json:"message"`
}

type LauncherSpec struct {
	ID                 string
	Name               string
	Executable         string
	WindowsExecutable  string
	ArgsPattern        []string
	DefaultArgsPattern []string
	WindowsPaths       []string
	WindowsGlobs       []string
	DarwinPaths        []string
	LinuxPaths         []string
}

func DefaultSpecs() []LauncherSpec {
	local := os.Getenv("LOCALAPPDATA")
	programFiles := os.Getenv("ProgramFiles")
	userProfile := os.Getenv("USERPROFILE")

	return []LauncherSpec{
		{
			ID:                 "folder",
			Name:               "Explorador de archivos",
			Executable:         "explorer.exe",
			DefaultArgsPattern: []string{"{target}"},
		},
		{
			ID:                 "vscode",
			Name:               "VS Code",
			Executable:         "code",
			DefaultArgsPattern: []string{"{target}"},
			ArgsPattern:        []string{"--goto", "{target}:{line}"},
			WindowsPaths: []string{
				filepath.Join(local, "Programs", "Microsoft VS Code", "Code.exe"),
				filepath.Join(programFiles, "Microsoft VS Code", "Code.exe"),
			},
		},
		{
			ID:                 "vscode-insiders",
			Name:               "VS Code Insiders",
			Executable:         "code-insiders",
			DefaultArgsPattern: []string{"{target}"},
			ArgsPattern:        []string{"--goto", "{target}:{line}"},
			WindowsPaths: []string{
				filepath.Join(local, "Programs", "Microsoft VS Code Insiders", "Code - Insiders.exe"),
				filepath.Join(programFiles, "Microsoft VS Code Insiders", "Code - Insiders.exe"),
			},
		},
		{
			ID:                 "antigravity",
			Name:               "Antigravity",
			Executable:         "antigravity",
			DefaultArgsPattern: []string{"{target}"},
			ArgsPattern:        []string{"--goto", "{target}:{line}"},
			WindowsPaths: []string{
				filepath.Join(userProfile, ".gemini", "antigravity-ide", "antigravity.exe"),
				filepath.Join(userProfile, ".gemini", "antigravity-ide", "bin", "antigravity.exe"),
				filepath.Join(local, "Programs", "Antigravity", "Antigravity.exe"),
				filepath.Join(local, "Programs", "antigravity-ide", "Antigravity.exe"),
			},
		},
		{
			ID:                 "cursor",
			Name:               "Cursor",
			Executable:         "cursor",
			DefaultArgsPattern: []string{"{target}"},
			ArgsPattern:        []string{"--goto", "{target}:{line}"},
			WindowsPaths: []string{
				filepath.Join(local, "Programs", "cursor", "Cursor.exe"),
				filepath.Join(local, "Programs", "Cursor", "Cursor.exe"),
			},
		},
		{
			ID:                 "windsurf",
			Name:               "Windsurf",
			Executable:         "windsurf",
			DefaultArgsPattern: []string{"{target}"},
			ArgsPattern:        []string{"--goto", "{target}:{line}"},
			WindowsPaths: []string{
				filepath.Join(local, "Programs", "Windsurf", "Windsurf.exe"),
			},
		},
		{
			ID:                 "intellij",
			Name:               "IntelliJ IDEA",
			Executable:         "idea",
			WindowsExecutable:  "idea64.exe",
			DefaultArgsPattern: []string{"{target}"},
			ArgsPattern:        []string{"--line", "{line}", "{target}"},
			WindowsGlobs: []string{
				filepath.Join(programFiles, "JetBrains", "IntelliJ IDEA *", "bin", "idea64.exe"),
				filepath.Join(local, "Programs", "IntelliJ IDEA *", "bin", "idea64.exe"),
				filepath.Join(local, "JetBrains", "Toolbox", "apps", "IDEA-*", "*", "*", "bin", "idea64.exe"),
			},
		},
		{
			ID:                 "zed",
			Name:               "Zed",
			Executable:         "zed",
			DefaultArgsPattern: []string{"{target}"},
			ArgsPattern:        []string{"{target}:{line}"},
			WindowsPaths: []string{
				filepath.Join(local, "Programs", "Zed", "zed.exe"),
			},
		},
	}
}

func Available() []Launcher {
	result := []Launcher{}
	for _, spec := range DefaultSpecs() {
		if spec.ID == "folder" {
			result = append(result, Launcher{ID: "folder", Name: spec.Name, Available: true, Message: "Disponible en el sistema"})
			continue
		}
		_, err := resolve(spec.ID, runtime.GOOS)
		item := Launcher{ID: spec.ID, Name: spec.Name, Available: err == nil, Message: "Instalado"}
		if err != nil {
			item.Message = "No detectado"
		}
		result = append(result, item)
	}
	return result
}

func getSpec(ide string) (LauncherSpec, bool) {
	targetID := strings.ToLower(strings.TrimSpace(ide))
	if targetID == "" {
		targetID = "vscode"
	}
	for _, spec := range DefaultSpecs() {
		if strings.ToLower(spec.ID) == targetID {
			return spec, true
		}
	}
	return LauncherSpec{}, false
}

func Build(ide, worktree, file string, line int, goos string) (Command, error) {
	if strings.TrimSpace(worktree) == "" {
		return Command{}, fmt.Errorf("invalid worktree")
	}
	root, err := filepath.Abs(worktree)
	if err != nil || strings.TrimSpace(root) == "" {
		return Command{}, fmt.Errorf("invalid worktree")
	}
	target := root
	if strings.TrimSpace(file) != "" {
		candidate, err := filepath.Abs(filepath.Join(root, filepath.Clean(file)))
		if err != nil {
			return Command{}, fmt.Errorf("invalid file")
		}
		relative, err := filepath.Rel(root, candidate)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return Command{}, fmt.Errorf("file escapes the worktree")
		}
		target = candidate
	}

	spec, ok := getSpec(ide)
	if !ok {
		return Command{}, fmt.Errorf("unsupported IDE %q", ide)
	}

	if spec.ID == "folder" {
		switch goos {
		case "windows":
			return Command{Executable: "explorer.exe", Args: []string{target}}, nil
		case "darwin":
			return Command{Executable: "open", Args: []string{target}}, nil
		default:
			return Command{Executable: "xdg-open", Args: []string{target}}, nil
		}
	}

	executable := spec.Executable
	if goos == "windows" && spec.WindowsExecutable != "" {
		executable = spec.WindowsExecutable
	}

	pattern := spec.DefaultArgsPattern
	if file != "" && line > 0 && len(spec.ArgsPattern) > 0 {
		pattern = spec.ArgsPattern
	}

	args := make([]string, len(pattern))
	for i, arg := range pattern {
		arg = strings.ReplaceAll(arg, "{target}", target)
		arg = strings.ReplaceAll(arg, "{line}", strconv.Itoa(line))
		arg = strings.ReplaceAll(arg, "{worktree}", root)
		args[i] = arg
	}

	return Command{Executable: executable, Args: args}, nil
}

func Open(ide, worktree, file string, line int) error {
	launch, err := Build(ide, worktree, file, line, runtime.GOOS)
	if err != nil {
		return err
	}
	path, err := resolve(ide, runtime.GOOS)
	if err != nil {
		return err
	}
	command := exec.Command(path, launch.Args...)
	command.Dir = worktree
	setPlatformAttributes(command)
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}

func resolve(ide, goos string) (string, error) {
	spec, ok := getSpec(ide)
	if !ok {
		return "", fmt.Errorf("unsupported IDE %q", ide)
	}
	return resolveSpec(spec, goos)
}

func resolveSpec(spec LauncherSpec, goos string) (string, error) {
	launch, err := Build(spec.ID, ".", "", 0, goos)
	if err != nil {
		return "", err
	}

	if path, lookupErr := exec.LookPath(launch.Executable); lookupErr == nil {
		return path, nil
	}

	candidates := []string{}
	switch goos {
	case "windows":
		candidates = append(candidates, spec.WindowsPaths...)
		for _, pattern := range spec.WindowsGlobs {
			matches, _ := filepath.Glob(pattern)
			candidates = append(candidates, matches...)
		}
	case "darwin":
		candidates = append(candidates, spec.DarwinPaths...)
	default:
		candidates = append(candidates, spec.LinuxPaths...)
	}

	for index := len(candidates) - 1; index >= 0; index-- {
		if candidates[index] == "" {
			continue
		}
		if info, statErr := os.Stat(candidates[index]); statErr == nil && !info.IsDir() {
			return candidates[index], nil
		}
	}
	return "", fmt.Errorf("%s no está instalado o no pudo detectarse", launch.Executable)
}

