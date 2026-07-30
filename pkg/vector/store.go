package vector

import "context"

// Point represents a vector point with an ID, vector, and optional payload.
type Point struct {
	ID      string         `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload,omitempty"`
}

// SearchResult represents a search result with a point ID, score, and optional payload.
type SearchResult struct {
	ID      string         `json:"id"`
	Score   float32        `json:"score"`
	Payload map[string]any `json:"payload,omitempty"`
}

// VectorStore defines the interface for vector database operations.
type VectorStore interface {
	// Ping verifies that the vector database is reachable.
	Ping(ctx context.Context) error

	// Upsert inserts or updates points in the collection.
	Upsert(ctx context.Context, points []Point) error

	// Search finds the top-K most similar vectors.
	Search(ctx context.Context, vector []float32, limit int) ([]SearchResult, error)

	// Delete removes points by ID.
	Delete(ctx context.Context, ids []string) error

	// RecreateCollection drops and recreates the collection.
	RecreateCollection(ctx context.Context) error
}
