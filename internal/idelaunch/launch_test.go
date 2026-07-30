package idelaunch

import (
	"path/filepath"
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
