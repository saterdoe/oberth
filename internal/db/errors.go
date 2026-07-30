package db

import "errors"

// ErrNotFound is returned when a requested resource is not found in the database.
var ErrNotFound = errors.New("db: not found")

// ErrConflict is returned when a state transition cannot be applied.
var ErrConflict = errors.New("db: conflict")
