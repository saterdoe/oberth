package tasktype

import "testing"

func TestNormalizeAndInfer(t *testing.T) {
	if Normalize("debug") != BugFix || Normalize("code") != Implementation {
		t.Fatal("legacy values were not normalized")
	}
	if Infer("corrige el bug de login") != BugFix {
		t.Fatal("failed to infer bug fix")
	}
}
