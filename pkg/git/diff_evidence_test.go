package git

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGetDiffIncludesUntrackedFilesAndStatus(t *testing.T) {
	repo := initDiffFixture(t)
	if err := os.WriteFile(filepath.Join(repo, "new file.txt"), []byte("candidate\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	diff, err := GetDiff(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff) != 1 {
		t.Fatalf("expected one untracked diff, got %+v", diff)
	}
	if diff[0].Path != "new file.txt" || diff[0].Status != "added" {
		t.Fatalf("unexpected untracked evidence: %+v", diff[0])
	}
	if diff[0].Content == "" {
		t.Fatal("untracked evidence must contain the patch")
	}
}

func TestGetDiffBindsBinaryContents(t *testing.T) {
	repo := initDiffFixture(t)
	path := filepath.Join(repo, "artifact.bin")
	first := append([]byte{0, 1, 2, 3}, bytes.Repeat([]byte{7}, 128)...)
	if err := os.WriteFile(path, first, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := GetDiff(repo)
	if err != nil {
		t.Fatal(err)
	}
	second := append([]byte{0, 1, 2, 4}, bytes.Repeat([]byte{9}, 128)...)
	if err := os.WriteFile(path, second, 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := GetDiff(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 || len(after) != 1 || before[0].Content == after[0].Content {
		t.Fatal("binary content changes must alter recorded evidence")
	}
}

func TestGetDiffDoesNotDuplicateStagedAndUnstagedPath(t *testing.T) {
	repo := initDiffFixture(t)
	path := filepath.Join(repo, "tracked.txt")
	if err := os.WriteFile(path, []byte("staged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runDiffGit(t, repo, "add", "tracked.txt")
	if err := os.WriteFile(path, []byte("final working tree\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	diff, err := GetDiff(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff) != 1 || diff[0].Path != "tracked.txt" || diff[0].Status != "modified" {
		t.Fatalf("expected one effective tracked diff, got %+v", diff)
	}
}

func initDiffFixture(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runDiffGit(t, repo, "init")
	runDiffGit(t, repo, "config", "user.email", "qa@oberth.local")
	runDiffGit(t, repo, "config", "user.name", "oberth QA")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runDiffGit(t, repo, "add", ".")
	runDiffGit(t, repo, "commit", "-m", "base")
	return repo
}

func runDiffGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
