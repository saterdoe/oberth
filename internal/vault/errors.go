package vault

import "errors"

var (
	ErrNotFound      = errors.New("vault: note not found")
	ErrPathTraversal = errors.New("vault: path traversal detected")
)
