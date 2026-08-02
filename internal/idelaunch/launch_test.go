package idelaunch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildStaysInsideWorktree(t *testing.T) {
	root := t.TempDir()
	launch, err := Build("vscode", root, filepath.Join("internal", "api", "server.go"), 42, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if launch.Executable != "code" || len(launch.Args) != 2 || launch.Args[0] != "--goto" {
		t.Fatalf("unexpected launch: %+v", launch)
	}
	if _, err := Build("vscode", root, filepath.Join("..", "secret.txt"), 1, "windows"); err == nil {
		t.Fatal("IDE target must not escape the worktree")
	}
	if _, err := Build("unknown", root, "", 0, "windows"); err == nil {
		t.Fatal("unsupported IDE must fail")
	}
}

func TestBuildUsesNativeFolderLauncher(t *testing.T) {
	launch, err := Build("folder", t.TempDir(), "", 0, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if launch.Executable != "explorer.exe" {
		t.Fatalf("unexpected folder launcher: %+v", launch)
	}
}

func TestBuildGenericLaunchers(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join("internal", "api", "server.go")

	// Antigravity
	antigravityLaunch, err := Build("antigravity", root, file, 100, "windows")
	if err != nil {
		t.Fatalf("antigravity build failed: %v", err)
	}
	if antigravityLaunch.Executable != "antigravity" || len(antigravityLaunch.Args) != 2 || antigravityLaunch.Args[0] != "--goto" {
		t.Fatalf("unexpected antigravity launch: %+v", antigravityLaunch)
	}

	// Cursor
	cursorLaunch, err := Build("cursor", root, file, 15, "windows")
	if err != nil {
		t.Fatalf("cursor build failed: %v", err)
	}
	if cursorLaunch.Executable != "cursor" || cursorLaunch.Args[0] != "--goto" {
		t.Fatalf("unexpected cursor launch: %+v", cursorLaunch)
	}

	// IntelliJ IDEA
	ijLaunch, err := Build("intellij", root, file, 50, "windows")
	if err != nil {
		t.Fatalf("intellij build failed: %v", err)
	}
	if ijLaunch.Executable != "idea64.exe" || ijLaunch.Args[0] != "--line" || ijLaunch.Args[1] != "50" {
		t.Fatalf("unexpected intellij launch: %+v", ijLaunch)
	}
}

func TestBuildUsesNativeFolderLauncherPlatforms(t *testing.T) {
	root := t.TempDir()
	darwin, err := Build("folder", root, "", 0, "darwin")
	if err != nil || darwin.Executable != "open" {
		t.Fatalf("unexpected darwin folder launch: %+v, %v", darwin, err)
	}
	linux, err := Build("folder", root, "", 0, "linux")
	if err != nil || linux.Executable != "xdg-open" {
		t.Fatalf("unexpected linux folder launch: %+v, %v", linux, err)
	}
}

func TestBuildValidation(t *testing.T) {
	if _, err := Build("vscode", "", "", 0, "windows"); err == nil {
		t.Fatal("expected error for empty worktree")
	}
	// Empty IDE defaults to vscode
	cmd, err := Build("", t.TempDir(), "", 0, "windows")
	if err != nil || cmd.Executable != "code" {
		t.Fatalf("expected empty IDE to default to code, got %+v, %v", cmd, err)
	}
}

func TestAvailableReturnsRegisteredSpecs(t *testing.T) {
	launchers := Available()
	if len(launchers) < 3 {
		t.Fatalf("expected at least 3 launchers, got %d", len(launchers))
	}
	foundAntigravity := false
	foundVSCode := false
	foundFolder := false
	for _, l := range launchers {
		if l.ID == "antigravity" {
			foundAntigravity = true
		}
		if l.ID == "vscode" {
			foundVSCode = true
		}
		if l.ID == "folder" {
			foundFolder = true
		}
	}
	if !foundAntigravity || !foundVSCode || !foundFolder {
		t.Fatalf("missing expected launcher in Available(): %+v", launchers)
	}
}

func TestResolveSpec(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "custom_antigravity.exe")
	if err := os.WriteFile(exePath, []byte("echo"), 0755); err != nil {
		t.Fatal(err)
	}

	spec := LauncherSpec{
		ID:                 "antigravity",
		Name:               "Antigravity",
		Executable:         "nonexistent_custom_cmd_xyz",
		DefaultArgsPattern: []string{"{target}"},
		WindowsPaths:       []string{exePath},
		DarwinPaths:        []string{exePath},
		LinuxPaths:         []string{exePath},
	}

	resWin, err := resolveSpec(spec, "windows")
	if err != nil || resWin != exePath {
		t.Fatalf("expected windows resolve to find %s, got %s, %v", exePath, resWin, err)
	}

	resMac, err := resolveSpec(spec, "darwin")
	if err != nil || resMac != exePath {
		t.Fatalf("expected darwin resolve to find %s, got %s, %v", exePath, resMac, err)
	}

	resLinux, err := resolveSpec(spec, "linux")
	if err != nil || resLinux != exePath {
		t.Fatalf("expected linux resolve to find %s, got %s, %v", exePath, resLinux, err)
	}
}

func TestResolveFailures(t *testing.T) {
	if _, err := resolve("nonexistent_ide_id", "windows"); err == nil || !strings.Contains(err.Error(), "unsupported IDE") {
		t.Fatalf("expected unsupported IDE error, got %v", err)
	}
}

func TestOpenFailure(t *testing.T) {
	if err := Open("nonexistent_ide_id", t.TempDir(), "", 0); err == nil {
		t.Fatal("expected error opening nonexistent IDE")
	}
	if err := Open("antigravity", "", "", 0); err == nil {
		t.Fatal("expected error for invalid worktree in Open")
	}
}

func TestSetPlatformAttributes(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "echo test")
	setPlatformAttributes(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("expected SysProcAttr to be configured")
	}
}





