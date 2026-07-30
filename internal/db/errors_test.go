package db

import (
	"errors"
	"testing"
)

func TestErrNotFound_IsSentinel(t *testing.T) {
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Error("ErrNotFound should wrap itself")
	}
}

func TestErrNotFound_Message(t *testing.T) {
	if ErrNotFound.Error() != "db: not found" {
		t.Errorf("got %q, want %q", ErrNotFound.Error(), "db: not found")
	}
}
