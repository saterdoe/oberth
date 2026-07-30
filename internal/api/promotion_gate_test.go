package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saterdoe/oberth/internal/reasoning"
	gitpkg "github.com/saterdoe/oberth/pkg/git"
)

func TestPromotionEvidenceIsBoundToCurrentDiff(t *testing.T) {
	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	runGit("init")
	runGit("config", "user.email", "qa@oberth.local")
	runGit("config", "user.name", "oberth QA")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-m", "base")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("candidate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	diff, err := gitpkg.GetDiff(repo)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(diff)
	hash := fmt.Sprintf("sha256:%x", sha256.Sum256(encoded))
	bundle, _ := json.Marshal(map[string]any{"verification_status": "passed", "diff_hash": hash})
	if err := validatePromotionEvidence(repo, bundle); err != nil {
		t.Fatalf("matching evidence must pass: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("changed after qa\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validatePromotionEvidence(repo, bundle); err == nil {
		t.Fatal("stale QA evidence must block promotion")
	}
}

func TestPromotionEvidenceEnforcesRequiredEpistemicGates(t *testing.T) {
	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	runGit("init")
	runGit("config", "user.email", "qa@oberth.local")
	runGit("config", "user.name", "oberth QA")
	path := filepath.Join(repo, "file.txt")
	if err := os.WriteFile(path, []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-m", "base")
	if err := os.WriteFile(path, []byte("candidate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	diff, err := gitpkg.GetDiff(repo)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(diff)
	diffHash := fmt.Sprintf("sha256:%x", sha256.Sum256(encoded))

	blockedCase := &reasoning.CaseV1{
		SchemaVersion: reasoning.SchemaVersion,
		Records: []reasoning.Record{{
			ID: "p1", Kind: reasoning.KindProperty, Statement: "retry is idempotent",
			Status: reasoning.StatusUnknown, Required: true,
		}},
		Evidence: []reasoning.EvidenceRef{},
	}
	blockedCase.Assessment = reasoning.Assess(blockedCase)
	blockedBundle, _ := json.Marshal(map[string]any{
		"verification_status": "passed", "diff_hash": diffHash, "reasoning": blockedCase,
	})
	err = validatePromotionEvidence(repo, blockedBundle)
	if err == nil || !strings.Contains(err.Error(), "evidencia epistemológica") {
		t.Fatalf("required unknown property must block promotion, got %v", err)
	}

	passedCase := &reasoning.CaseV1{
		SchemaVersion: reasoning.SchemaVersion,
		Records: []reasoning.Record{{
			ID: "p1", Kind: reasoning.KindProperty, Statement: "retry is idempotent",
			Status: reasoning.StatusPassed, Required: true, EvidenceIDs: []string{"e1"},
		}},
		Evidence: []reasoning.EvidenceRef{{
			ID: "e1", Source: "command:go test ./...", Subject: "diff", SubjectHash: diffHash,
			Hash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}},
	}
	passedCase.Assessment = reasoning.Assess(passedCase)
	passedBundle, _ := json.Marshal(map[string]any{
		"verification_status": "passed", "diff_hash": diffHash, "reasoning": passedCase,
	})
	if err := validatePromotionEvidence(repo, passedBundle); err != nil {
		t.Fatalf("required property with current evidence must pass: %v", err)
	}
}

func TestPromotionEvidenceRejectsUntrackedFileAddedAfterQA(t *testing.T) {
	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	runGit("init")
	runGit("config", "user.email", "qa@oberth.local")
	runGit("config", "user.name", "oberth QA")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-m", "base")

	diff, err := gitpkg.GetDiff(repo)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(diff)
	hash := fmt.Sprintf("sha256:%x", sha256.Sum256(encoded))
	bundle, _ := json.Marshal(map[string]any{"verification_status": "passed", "diff_hash": hash})
	if err := validatePromotionEvidence(repo, bundle); err != nil {
		t.Fatalf("clean verified state must pass: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "unreviewed.txt"), []byte("not reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validatePromotionEvidence(repo, bundle); err == nil {
		t.Fatal("an untracked file added after QA must invalidate promotion evidence")
	}
}
