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

func Available() []Launcher {
	result := []Launcher{{ID: "folder", Name: "Explorador de archivos", Available: true, Message: "Disponible en el sistema"}}
	for _, candidate := range []struct{ id, name string }{{"vscode", "VS Code"}, {"intellij", "IntelliJ IDEA"}} {
		_, err := resolve(candidate.id, runtime.GOOS)
		item := Launcher{ID: candidate.id, Name: candidate.name, Available: err == nil, Message: "Instalado"}
		if err != nil {
			item.Message = "No detectado"
		}
		result = append(result, item)
	}
	return result
}

func Build(ide, worktree, file string, line int, goos string) (Command, error) {
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
	switch strings.ToLower(strings.TrimSpace(ide)) {
	case "folder":
		switch goos {
		case "windows":
			return Command{Executable: "explorer.exe", Args: []string{target}}, nil
		case "darwin":
			return Command{Executable: "open", Args: []string{target}}, nil
		default:
			return Command{Executable: "xdg-open", Args: []string{target}}, nil
		}
	case "", "vscode":
		args := []string{target}
		if file != "" && line > 0 {
			args = []string{"--goto", target + ":" + strconv.Itoa(line)}
		}
		return Command{Executable: "code", Args: args}, nil
	case "intellij":
		executable := "idea"
		if goos == "windows" {
			executable = "idea64.exe"
		}
		args := []string{target}
		if file != "" && line > 0 {
			args = []string{"--line", strconv.Itoa(line), target}
		}
		return Command{Executable: executable, Args: args}, nil
	default:
		return Command{}, fmt.Errorf("unsupported IDE %q", ide)
	}
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
	launch, err := Build(ide, ".", "", 0, goos)
	if err != nil {
		return "", err
	}
	if path, lookupErr := exec.LookPath(launch.Executable); lookupErr == nil {
		return path, nil
	}
	if goos != "windows" {
		return "", fmt.Errorf("%s no está instalado o no está disponible en PATH", launch.Executable)
	}
	local := os.Getenv("LOCALAPPDATA")
	programFiles := os.Getenv("ProgramFiles")
	candidates := []string{}
	switch strings.ToLower(strings.TrimSpace(ide)) {
	case "", "vscode":
		candidates = append(candidates,
			filepath.Join(local, "Programs", "Microsoft VS Code", "Code.exe"),
			filepath.Join(programFiles, "Microsoft VS Code", "Code.exe"))
	case "intellij":
		patterns := []string{
			filepath.Join(programFiles, "JetBrains", "IntelliJ IDEA *", "bin", "idea64.exe"),
			filepath.Join(local, "Programs", "IntelliJ IDEA *", "bin", "idea64.exe"),
			filepath.Join(local, "JetBrains", "Toolbox", "apps", "IDEA-*", "*", "*", "bin", "idea64.exe"),
		}
		for _, pattern := range patterns {
			matches, _ := filepath.Glob(pattern)
			candidates = append(candidates, matches...)
		}
	}
	for index := len(candidates) - 1; index >= 0; index-- {
		if info, statErr := os.Stat(candidates[index]); statErr == nil && !info.IsDir() {
			return candidates[index], nil
		}
	}
	return "", fmt.Errorf("%s no está instalado o no pudo detectarse", launch.Executable)
}
