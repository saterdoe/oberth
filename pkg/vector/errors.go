package vector

import "errors"

var (
	// ErrCollectionNotFound is returned when the requested collection does not exist.
	ErrCollectionNotFound = errors.New("collection not found")

	// ErrConnectionFailed is returned when the connection to the vector store fails.
	ErrConnectionFailed = errors.New("connection to vector store failed")
)
