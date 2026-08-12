package git

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func DiffHash(repoRoot string) (string, error) {
	diff, err := GetDiff(repoRoot)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(diff)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(encoded)), nil
}

func DiffHashBetween(repoRoot, base, head string) (string, error) {
	repo, err := DetectRepo(repoRoot)
	if err != nil {
		return "", err
	}
	out, err := runCmd(context.Background(), "git", "-C", repo.Root, "-c", "core.quotePath=false", "diff", "--binary", base, head, "--")
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(parseDiff(string(out)))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(encoded)), nil
}

type RepoInfo struct {
	Root       string `json:"root"`
	Branch     string `json:"branch"`
	Remote     string `json:"remote"`
	IsClean    bool   `json:"is_clean"`
	LastCommit string `json:"last_commit"`
}

type DiffFile struct {
	Path    string `json:"path"`
	Status  string `json:"status"` // added, modified, deleted
	Content string `json:"content"`
}

func DetectRepo(path string) (*RepoInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rootB, err := runCmd(ctx, "git", "-C", path, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}
	root := filepath.FromSlash(strings.TrimSpace(string(rootB)))

	branchB, _ := runCmd(ctx, "git", "-C", root, "rev-parse", "--abbrev-ref", "HEAD")
	branch := strings.TrimSpace(string(branchB))

	remoteB, _ := runCmd(ctx, "git", "-C", root, "remote", "get-url", "origin")
	remote := strings.TrimSpace(string(remoteB))

	statusB, _ := runCmd(ctx, "git", "-C", root, "status", "--porcelain")
	isClean := len(strings.TrimSpace(string(statusB))) == 0

	commitB, _ := runCmd(ctx, "git", "-C", root, "rev-parse", "--short", "HEAD")
	commit := strings.TrimSpace(string(commitB))

	return &RepoInfo{
		Root:       root,
		Branch:     branch,
		Remote:     remote,
		IsClean:    isClean,
		LastCommit: commit,
	}, nil
}

func CreateBranch(repoRoot, branchName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := runCmd(ctx, "git", "-C", repoRoot, "checkout", "-b", branchName)
	if err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}
	return nil
}

func GetDiff(repoRoot string) ([]DiffFile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// A single HEAD-based diff represents the effective tracked-file state and
	// avoids emitting the same path twice when it has staged and unstaged edits.
	out, err := runCmd(ctx, "git", "-C", repoRoot, "-c", "core.quotePath=false", "diff", "--binary", "HEAD", "--")
	if err != nil {
		return nil, err
	}
	allDiffs := string(out)

	// Git deliberately omits untracked files from `git diff`. They still become
	// part of the promoted commit (`git add -A`), so evidence must include them
	// as well or a file could be introduced after QA without changing DiffHash.
	out, err = runCmd(ctx, "git", "-C", repoRoot, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	for _, path := range strings.Split(string(out), "\x00") {
		if path == "" {
			continue
		}
		untrackedDiff, diffErr := runDiffNoIndex(ctx, repoRoot, path)
		if diffErr != nil {
			return nil, diffErr
		}
		allDiffs += string(untrackedDiff)
	}
	return parseDiff(allDiffs), nil
}

func parseDiff(allDiffs string) []DiffFile {
	if allDiffs == "" {
		return []DiffFile{}
	}
	var files []DiffFile
	lines := strings.Split(allDiffs, "\n")
	var current DiffFile
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			if current.Path != "" && current.Content != "" {
				files = append(files, current)
			}
			parts := strings.Split(line, " b/")
			path := ""
			if len(parts) >= 2 {
				path = strings.TrimSpace(parts[len(parts)-1])
			}
			current = DiffFile{Path: path, Status: "modified", Content: line + "\n"}
		} else if strings.HasPrefix(line, "new file mode ") {
			current.Status = "added"
			current.Content += line + "\n"
		} else if strings.HasPrefix(line, "deleted file mode ") {
			current.Status = "deleted"
			current.Content += line + "\n"
		} else if current.Path != "" {
			// Preserve the complete patch, including mode, rename and binary
			// payload lines. DiffHash is a security boundary for promotion.
			current.Content += line + "\n"
		}
	}
	if current.Path != "" && current.Content != "" {
		files = append(files, current)
	}
	return files
}

// CheckDiff runs Git's non-mutating whitespace/error validation directly.
// Runtime-owned release gates use this instead of an agent command so the
// result cannot depend on an interactive command approval policy.
func CheckDiff(repository string) (string, error) {
	repo, err := DetectRepo(repository)
	if err != nil {
		return "", err
	}
	output, err := runCmd(context.Background(), "git", "-C", repo.Root, "diff", "--check")
	return string(output), err
}

func runDiffNoIndex(ctx context.Context, repoRoot, path string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "-c", "core.quotePath=false",
		"diff", "--binary", "--no-index", "--", "/dev/null", filepath.ToSlash(path))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return stdout.Bytes(), nil // `git diff --no-index` uses 1 for differences.
	}
	return nil, fmt.Errorf("git diff untracked %q: %w\n%s", path, err, stderr.String())
}

func GetDiffStat(repoRoot string) (int, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := runCmd(ctx, "git", "-C", repoRoot, "diff", "--stat")
	if err != nil {
		return 0, 0, err
	}
	lines := strings.TrimSpace(string(out))
	if lines == "" {
		return 0, 0, nil
	}
	return strings.Count(lines, "\n") + 1, len(out), nil
}

func ApplyDiff(repoRoot, patch string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "apply")
	cmd.Stdin = strings.NewReader(patch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("apply failed: %w\n%s", err, stderr.String())
	}
	return nil
}

func WorkingTreeFiles(repoRoot string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := runCmd(ctx, "git", "-C", repoRoot, "ls-files", "--modified", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []string{}, nil
	}
	return lines, nil
}

func GetRepoName(repoRoot string) string {
	return filepath.Base(repoRoot)
}

func runCmd(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %w\n%s", name, err, stderr.String())
	}
	return stdout.Bytes(), nil
}
