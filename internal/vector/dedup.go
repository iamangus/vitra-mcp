package vector

import (
	"context"
	"fmt"
)

// DedupChecker provides duplicate detection functionality
type DedupChecker struct {
	store     VectorStore
	threshold float32
}

// NewDedupChecker creates a new deduplication checker
func NewDedupChecker(store VectorStore, threshold float32) *DedupChecker {
	if threshold <= 0 {
		threshold = 0.95
	}
	return &DedupChecker{
		store:     store,
		threshold: threshold,
	}
}

// CheckDuplicate checks if content is similar to existing notes
func (d *DedupChecker) CheckDuplicate(ctx context.Context, content string) (*SearchResult, error) {
	result, err := d.store.CheckDuplicate(ctx, content, d.threshold)
	if err != nil {
		return nil, fmt.Errorf("duplicate check failed: %w", err)
	}
	return result, nil
}

// CheckDuplicateWithNote checks if a note's content is similar to existing notes
func (d *DedupChecker) CheckDuplicateWithNote(ctx context.Context, path string, content string) (*SearchResult, error) {
	// First check if content is similar to any existing note
	result, err := d.store.CheckDuplicate(ctx, content, d.threshold)
	if err != nil {
		return nil, err
	}

	// If found and it's not the same note, return it
	if result != nil && result.Path != path {
		return result, nil
	}

	return nil, nil
}
