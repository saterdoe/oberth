package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initRepo creates a temporary directory, initializes it as a git repo,
// and configures a known user so commits work.
func initRepo(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "oberth-git-test-*")
	require.NoError(t, err)

	t.Cleanup(func() { os.RemoveAll(dir) })

	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
	}

	run("init")
	run("config", "user.email", "test@oberth.test")
	run("config", "user.name", "Test User")

	return dir
}

func TestGetRepoName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/projects/my-repo", "my-repo"},
		{"/tmp/foo", "foo"},
		{"", "."},
	}
	for _, tt := range tests {
		got := GetRepoName(tt.path)
		assert.Equal(t, tt.want, got, "GetRepoName(%q)", tt.path)
	}
}

func TestDetectRepo(t *testing.T) {
	dir := initRepo(t)

	// make an initial commit so git has a branch name
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0644)
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
	}
	run("add", ".")
	run("commit", "-m", "initial")

	resolvedDir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	info, err := DetectRepo(dir)
	require.NoError(t, err)
	assert.Equal(t, resolvedDir, info.Root)
	assert.NotEmpty(t, info.Branch)
	assert.True(t, info.IsClean)
}

func TestDetectRepo_NotARepo(t *testing.T) {
	dir := t.TempDir()

	_, err := DetectRepo(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}

func TestCreateBranch(t *testing.T) {
	dir := initRepo(t)

	// need at least one commit to create a branch
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0644)
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
	}
	run("add", ".")
	run("commit", "-m", "initial")

	err := CreateBranch(dir, "feature/test")
	require.NoError(t, err)

	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	require.NoError(t, err)
	assert.Equal(t, "feature/test", strings.TrimSpace(string(out)))
}

func TestCreateBranch_InvalidName(t *testing.T) {
	dir := initRepo(t)

	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0644)
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
	}
	run("add", ".")
	run("commit", "-m", "initial")

	// a branch name with .. is invalid
	err := CreateBranch(dir, "..")
	assert.Error(t, err)
}

func TestCommit(t *testing.T) {
	dir := initRepo(t)

	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("world"), 0644)

	hash, err := CommitApproved(dir, []string{"hello.txt"}, "initial commit", Approval{Granted: true, Actor: "test"})
	require.NoError(t, err)
	assert.NotEmpty(t, hash)

	info, err := DetectRepo(dir)
	require.NoError(t, err)
	assert.Equal(t, hash, info.LastCommit)
}

func TestWorkingTreeFiles_Modified(t *testing.T) {
	dir := initRepo(t)

	os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("v1"), 0644)
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
	}
	run("add", ".")
	run("commit", "-m", "initial")

	os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("v2"), 0644)
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new file"), 0644)

	files, err := WorkingTreeFiles(dir)
	require.NoError(t, err)
	assert.Contains(t, files, "existing.txt")
	assert.Contains(t, files, "new.txt")
}

func TestWorkingTreeFiles_Clean(t *testing.T) {
	dir := initRepo(t)

	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("content"), 0644)
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
	}
	run("add", ".")
	run("commit", "-m", "initial")

	files, err := WorkingTreeFiles(dir)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestGetDiffStat(t *testing.T) {
	dir := initRepo(t)

	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
	}
	run("commit", "--allow-empty", "-m", "initial")

	files, lines, err := GetDiffStat(dir)
	require.NoError(t, err)
	assert.Equal(t, 0, files)
	assert.Equal(t, 0, lines)

	// modify a tracked file so git diff --stat shows changes
	os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("content"), 0644)
	run("add", ".")
	run("commit", "-m", "add file")
	os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("modified content\nline2\nline3\n"), 0644)

	files, lines, err = GetDiffStat(dir)
	require.NoError(t, err)
	assert.Greater(t, files, 0)
	assert.Greater(t, lines, 0)
}

func TestGetDiff_NoChanges(t *testing.T) {
	dir := initRepo(t)

	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
	}
	run("commit", "--allow-empty", "-m", "initial")

	diffs, err := GetDiff(dir)
	require.NoError(t, err)
	assert.Empty(t, diffs)
}

func TestGetDiff_WithChanges(t *testing.T) {
	dir := initRepo(t)

	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
	}
	run("commit", "--allow-empty", "-m", "initial")

	// create a tracked file, commit it, then modify it to produce a diff
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v1"), 0644)
	run("add", ".")
	run("commit", "-m", "add file")
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v2"), 0644)

	diffs, err := GetDiff(dir)
	require.NoError(t, err)
	assert.Len(t, diffs, 1)
	assert.Equal(t, "file.txt", diffs[0].Path)
	assert.Contains(t, diffs[0].Content, "file.txt")
}

func TestApplyDiff(t *testing.T) {
	dir := initRepo(t)

	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
	}
	run("commit", "--allow-empty", "-m", "initial")

	patch := `diff --git a/hello.txt b/hello.txt
new file mode 100644
index 0000000..3b18e51
--- /dev/null
+++ b/hello.txt
@@ -0,0 +1 @@
+Hello, World!
`
	err := ApplyDiff(dir, patch)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	require.NoError(t, err)
	assert.Equal(t, "Hello, World!\n", strings.ReplaceAll(string(data), "\r\n", "\n"))
}

func TestApplyDiff_InvalidPatch(t *testing.T) {
	dir := initRepo(t)

	err := ApplyDiff(dir, "this is not a valid git patch")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "apply failed")
}

func TestCreateAndCleanupSessionWorktree(t *testing.T) {
	dir := initRepo(t)
	writeAndCommit(t, dir, "README.md", "base", "initial")
	worktrees := t.TempDir()

	workspace, err := CreateSessionWorktree(dir, worktrees, "session-42")
	require.NoError(t, err)
	assert.DirExists(t, workspace.Path)
	assert.Equal(t, "oberth/session-42", workspace.Branch)
	require.NoError(t, os.WriteFile(filepath.Join(workspace.Path, "task.txt"), []byte("change"), 0600))
	assert.ErrorIs(t, CleanupSessionWorktree(workspace, false), ErrDirtyWorktree)
	require.NoError(t, CleanupSessionWorktree(workspace, true))
	assert.NoDirExists(t, workspace.Path)
}

func TestPromoteSessionWorktreeRequiresApprovalAndPromotesCommit(t *testing.T) {
	dir := initRepo(t)
	writeAndCommit(t, dir, "README.md", "base", "initial")
	workspace, err := CreateSessionWorktree(dir, t.TempDir(), "promotion")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workspace.Path, "accepted.txt"), []byte("accepted"), 0600))

	_, err = PromoteSessionWorktree(workspace, "accept change", Approval{})
	require.ErrorIs(t, err, ErrApprovalRequired)
	result, err := PromoteSessionWorktree(workspace, "accept change", Approval{Granted: true, Actor: "user:test"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Commit)
	promoted, err := CurrentCommit(dir)
	require.NoError(t, err)
	assert.Equal(t, result.Commit, promoted)
	data, err := os.ReadFile(filepath.Join(dir, "accepted.txt"))
	require.NoError(t, err)
	assert.Equal(t, "accepted", string(data))
}

func TestReviewedPromotionRejectsChangedWorktreeAndBase(t *testing.T) {
	dir := initRepo(t)
	writeAndCommit(t, dir, "README.md", "base", "initial")
	base, err := CurrentCommit(dir)
	require.NoError(t, err)
	workspace, err := CreateSessionWorktree(dir, t.TempDir(), "stale-review")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workspace.Path, "change.txt"), []byte("reviewed"), 0600))
	reviewedHash, err := DiffHash(workspace.Path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workspace.Path, "change.txt"), []byte("changed later"), 0600))
	_, err = PromoteReviewedSessionWorktree(workspace, base, reviewedHash, "promote", Approval{Granted: true, Actor: "user:test"})
	require.ErrorIs(t, err, ErrStaleApproval)

	require.NoError(t, os.WriteFile(filepath.Join(workspace.Path, "change.txt"), []byte("reviewed"), 0600))
	reviewedHash, err = DiffHash(workspace.Path)
	require.NoError(t, err)
	writeAndCommit(t, dir, "other.txt", "advance", "advance base")
	_, err = PromoteReviewedSessionWorktree(workspace, base, reviewedHash, "promote", Approval{Granted: true, Actor: "user:test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base branch advanced")
}

func TestReviewedPromotionRejectsDirtyCheckout(t *testing.T) {
	dir := initRepo(t)
	writeAndCommit(t, dir, "README.md", "base", "initial")
	base, _ := CurrentCommit(dir)
	workspace, err := CreateSessionWorktree(dir, t.TempDir(), "dirty-checkout")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workspace.Path, "change.txt"), []byte("reviewed"), 0600))
	hash, err := DiffHash(workspace.Path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("dirty"), 0600))
	_, err = PromoteReviewedSessionWorktree(workspace, base, hash, "promote", Approval{Granted: true, Actor: "user:test"})
	require.ErrorIs(t, err, ErrDirtyWorktree)
}

func TestConcurrentPromotionsAreSerialized(t *testing.T) {
	dir := initRepo(t)
	writeAndCommit(t, dir, "README.md", "base", "initial")
	base, _ := CurrentCommit(dir)
	root := t.TempDir()
	one, err := CreateSessionWorktree(dir, root, "concurrent-one")
	require.NoError(t, err)
	two, err := CreateSessionWorktree(dir, root, "concurrent-two")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(one.Path, "one.txt"), []byte("one"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(two.Path, "two.txt"), []byte("two"), 0600))
	oneHash, _ := DiffHash(one.Path)
	twoHash, _ := DiffHash(two.Path)

	errs := make(chan error, 2)
	for _, candidate := range []struct {
		worktree SessionWorktree
		hash     string
	}{{one, oneHash}, {two, twoHash}} {
		go func(candidate struct {
			worktree SessionWorktree
			hash     string
		}) {
			_, promoteErr := PromoteReviewedSessionWorktree(candidate.worktree, base, candidate.hash, "promote", Approval{Granted: true, Actor: "user:test"})
			errs <- promoteErr
		}(candidate)
	}
	var succeeded, rejected int
	for range 2 {
		if err := <-errs; err == nil {
			succeeded++
		} else if strings.Contains(err.Error(), "base branch advanced") {
			rejected++
		} else {
			t.Fatalf("unexpected promotion error: %v", err)
		}
	}
	assert.Equal(t, 1, succeeded)
	assert.Equal(t, 1, rejected)
}

func TestPromotionConflictAbortsCherryPick(t *testing.T) {
	dir := initRepo(t)
	writeAndCommit(t, dir, "shared.txt", "base", "initial")
	workspace, err := CreateSessionWorktree(dir, t.TempDir(), "conflict")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workspace.Path, "shared.txt"), []byte("candidate"), 0600))
	writeAndCommit(t, dir, "shared.txt", "main change", "advance")
	_, err = PromoteSessionWorktree(workspace, "promote", Approval{Granted: true, Actor: "user:test"})
	require.Error(t, err)
	status, statusErr := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	require.NoError(t, statusErr)
	assert.Empty(t, strings.TrimSpace(string(status)), "failed promotion must abort the cherry-pick")
}

func TestRevertFilesIsSelectiveAndTraversalSafe(t *testing.T) {
	dir := initRepo(t)
	writeAndCommit(t, dir, "a.txt", "one", "initial")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("keep"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("changed"), 0600))

	reverted, err := RevertFiles(dir, []string{"a.txt"})
	require.NoError(t, err)
	assert.Equal(t, []string{"a.txt"}, reverted)
	a, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	b, _ := os.ReadFile(filepath.Join(dir, "b.txt"))
	assert.Equal(t, "one", string(a))
	assert.Equal(t, "keep", string(b))
	_, err = RevertFiles(dir, []string{"../outside.txt"})
	assert.ErrorIs(t, err, ErrUnsafePath)
}

func TestSessionRevertRefusesToOverwriteLaterUserChanges(t *testing.T) {
	dir := initRepo(t)
	writeAndCommit(t, dir, "tracked.txt", "base", "initial")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("agent change"), 0600))
	changes, err := CaptureSessionChanges(dir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("user follow-up"), 0600))

	_, err = RevertSession(changes, Approval{Granted: true, Actor: "user:local"})
	assert.ErrorIs(t, err, ErrUserChanges)
	data, _ := os.ReadFile(filepath.Join(dir, "tracked.txt"))
	assert.Equal(t, "user follow-up", string(data))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("agent change"), 0600))
	reverted, err := RevertSession(changes, Approval{Granted: true, Actor: "user:local"})
	require.NoError(t, err)
	assert.Equal(t, []string{"tracked.txt"}, reverted)
	data, _ = os.ReadFile(filepath.Join(dir, "tracked.txt"))
	assert.Equal(t, "base", string(data))
}

func TestCommitAndPushRequireExplicitApproval(t *testing.T) {
	dir := initRepo(t)
	writeAndCommit(t, dir, "README.md", "base", "initial")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "change.txt"), []byte("approved"), 0600))

	_, err := CommitApproved(dir, []string{"change.txt"}, "feat: approved", Approval{})
	assert.ErrorIs(t, err, ErrApprovalRequired)
	hash, err := CommitApproved(dir, []string{"change.txt"}, "feat: approved", Approval{Granted: true, Actor: "user:local"})
	require.NoError(t, err)
	assert.NotEmpty(t, hash)

	remote := filepath.Join(t.TempDir(), "remote.git")
	out, err := exec.Command("git", "init", "--bare", remote).CombinedOutput()
	require.NoError(t, err, string(out))
	out, err = exec.Command("git", "-C", dir, "remote", "add", "origin", remote).CombinedOutput()
	require.NoError(t, err, string(out))
	branchOut, err := exec.Command("git", "-C", dir, "branch", "--show-current").Output()
	require.NoError(t, err)
	branch := strings.TrimSpace(string(branchOut))

	err = PushApproved(dir, "origin", branch, Approval{})
	assert.ErrorIs(t, err, ErrApprovalRequired)
	require.NoError(t, PushApproved(dir, "origin", branch, Approval{Granted: true, Actor: "user:local"}))
	remoteHash, err := exec.Command("git", "--git-dir", remote, "rev-parse", "refs/heads/"+branch).Output()
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(hash), strings.TrimSpace(string(remoteHash))[:len(hash)])
}

func writeAndCommit(t *testing.T, dir, name, content, message string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0600))
	for _, args := range [][]string{{"add", name}, {"commit", "-m", message}} {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
}
