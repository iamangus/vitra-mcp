package vector

import "context"

// Chunk represents a piece of a note with context
type Chunk struct {
	Text    string
	Index   int
	Heading string // breadcrumb path like "Parent > Child"
	Path    string // note path
}

// SearchResult represents a semantic search result
type SearchResult struct {
	Path     string  `json:"path"`
	Title    string  `json:"title"`
	Heading  string  `json:"heading"`
	Chunk    string  `json:"chunk"`
	Distance float32 `json:"distance"`
}

// VectorStore defines the interface for vector database operations
type VectorStore interface {
	IndexNote(ctx context.Context, path string, chunks []Chunk) error
	DeleteNote(ctx context.Context, path string) error
	SemanticSearch(ctx context.Context, query string, limit int) ([]SearchResult, error)
	FindSimilarFiles(ctx context.Context, path string, limit int) ([]SearchResult, error)
	CheckDuplicate(ctx context.Context, content string, threshold float32) (*SearchResult, error)
	ReindexVault(ctx context.Context, vaultPath string) error
	Close() error
}
