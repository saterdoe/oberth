package vault

import (
	"errors"
	"testing"
)

func TestErrors_AreSentinelErrors(t *testing.T) {
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Error("ErrNotFound should wrap itself")
	}
	if !errors.Is(ErrPathTraversal, ErrPathTraversal) {
		t.Error("ErrPathTraversal should wrap itself")
	}
}

func TestErrors_ErrorMessage(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{ErrNotFound, "vault: note not found"},
		{ErrPathTraversal, "vault: path traversal detected"},
	}
	for _, tt := range tests {
		if tt.err.Error() != tt.want {
			t.Errorf("got %q, want %q", tt.err.Error(), tt.want)
		}
	}
}

func TestErrors_AreDistinct(t *testing.T) {
	if errors.Is(ErrNotFound, ErrPathTraversal) {
		t.Error("ErrNotFound should not match ErrPathTraversal")
	}
	if errors.Is(ErrPathTraversal, ErrNotFound) {
		t.Error("ErrPathTraversal should not match ErrNotFound")
	}
}
