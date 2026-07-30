package api

import "testing"

func TestTaskTransitions(t *testing.T) {
	tests := []struct {
		from, to string
		allowed  bool
	}{
		{"pending", "running", true}, {"running", "review", true},
		{"review", "completed", true}, {"failed", "running", true},
		{"completed", "running", false}, {"pending", "completed", false},
	}
	for _, tt := range tests {
		if got := taskTransitions[tt.from][tt.to]; got != tt.allowed {
			t.Errorf("transition %s -> %s: got %v, want %v", tt.from, tt.to, got, tt.allowed)
		}
	}
}
