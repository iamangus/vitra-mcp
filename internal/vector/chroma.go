package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ChromaStore implements VectorStore using ChromaDB REST API
type ChromaStore struct {
	BaseURL      string
	Collection   string
	CollectionID string
	Tenant       string
	Database     string
	Client       *http.Client
	Embedder     *EmbeddingClient
}

// NewChromaStore creates a new ChromaDB store
func NewChromaStore() *ChromaStore {
	baseURL := os.Getenv("CHROMADB_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}

	tenant := os.Getenv("CHROMADB_TENANT")
	if tenant == "" {
		tenant = "default_tenant"
	}

	database := os.Getenv("CHROMADB_DATABASE")
	if database == "" {
		database = "default_database"
	}

	return &ChromaStore{
		BaseURL:    baseURL,
		Collection: "vitra_notes",
		Tenant:     tenant,
		Database:   database,
		Client:     &http.Client{Timeout: 30 * time.Second},
		Embedder:   NewEmbeddingClient(),
	}
}

// collectionResponse represents a collection from the list/get API
type collectionResponse struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Metadata map[string]interface{} `json:"metadata"`
}

// chromaRequest/Response types for API calls
type chromaAddRequest struct {
	IDs        []string                 `json:"ids"`
	Embeddings [][]float32              `json:"embeddings"`
	Documents  []string                 `json:"documents"`
	Metadatas  []map[string]interface{} `json:"metadatas"`
}

type chromaQueryRequest struct {
	QueryEmbeddings [][]float32            `json:"query_embeddings,omitempty"`
	QueryTexts      []string               `json:"query_texts,omitempty"`
	NResults        int                    `json:"n_results"`
	Where           map[string]interface{} `json:"where,omitempty"`
	Include         []string               `json:"include"`
}

type chromaQueryResponse struct {
	IDs        [][]string                 `json:"ids"`
	Distances  [][]float32                `json:"distances"`
	Documents  [][]string                 `json:"documents"`
	Metadatas  [][]map[string]interface{} `json:"metadatas"`
	Embeddings [][][]float32              `json:"embeddings"`
}

// basePath returns the base API path for tenant/database
type basePath string

func (c *ChromaStore) basePath() string {
	return fmt.Sprintf("%s/api/v2/tenants/%s/databases/%s", c.BaseURL, c.Tenant, c.Database)
}

// ensureCollection creates the collection if it doesn't exist and stores the collection ID
func (c *ChromaStore) ensureCollection(ctx context.Context) error {
	if c.CollectionID != "" {
		return nil
	}

	// Try to list collections to find existing one
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s/collections", c.basePath()), nil)
	if err != nil {
		return err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var collections []collectionResponse
		if err := json.NewDecoder(resp.Body).Decode(&collections); err == nil {
			for _, col := range collections {
				if col.Name == c.Collection {
					c.CollectionID = col.ID
					return nil
				}
			}
		}
	}

	// Create collection
	createBody, _ := json.Marshal(map[string]interface{}{
		"name": c.Collection,
		"metadata": map[string]interface{}{
			"description": "Vitra notes vector store",
		},
		"get_or_create": false,
	})

	req, err = http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/collections", c.basePath()),
		bytes.NewBuffer(createBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err = c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to create collection: %d - %s", resp.StatusCode, string(body))
	}

	var created collectionResponse
	if err := json.Unmarshal(body, &created); err != nil {
		return fmt.Errorf("failed to parse collection response: %w", err)
	}

	c.CollectionID = created.ID
	return nil
}

// collectionPath returns the path for collection-specific operations
func (c *ChromaStore) collectionPath() string {
	return fmt.Sprintf("%s/collections/%s", c.basePath(), c.CollectionID)
}

// IndexNote indexes a note's chunks into ChromaDB
func (c *ChromaStore) IndexNote(ctx context.Context, path string, chunks []Chunk) error {
	if err := c.ensureCollection(ctx); err != nil {
		return fmt.Errorf("failed to ensure collection: %w", err)
	}

	if len(chunks) == 0 {
		return nil
	}

	// Generate embeddings for all chunks
	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		texts[i] = chunk.Text
	}

	embeddings, err := c.Embedder.EmbedTexts(texts)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}

	// Build request
	ids := make([]string, len(chunks))
	documents := make([]string, len(chunks))
	metadatas := make([]map[string]interface{}, len(chunks))

	for i, chunk := range chunks {
		ids[i] = fmt.Sprintf("%s#%d", path, chunk.Index)
		documents[i] = chunk.Text
		metadatas[i] = map[string]interface{}{
			"path":        path,
			"title":       getTitleFromPath(path),
			"heading":     chunk.Heading,
			"chunk_index": chunk.Index,
		}
	}

	addReq := chromaAddRequest{
		IDs:        ids,
		Embeddings: embeddings,
		Documents:  documents,
		Metadatas:  metadatas,
	}

	body, _ := json.Marshal(addReq)
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/add", c.collectionPath()),
		bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to add documents: %d - %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// DeleteNote removes all chunks for a note
func (c *ChromaStore) DeleteNote(ctx context.Context, path string) error {
	if err := c.ensureCollection(ctx); err != nil {
		return err
	}

	// Delete by metadata filter
	deleteBody, _ := json.Marshal(map[string]interface{}{
		"where": map[string]interface{}{
			"path": path,
		},
	})

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/delete", c.collectionPath()),
		bytes.NewBuffer(deleteBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// SemanticSearch performs vector similarity search
func (c *ChromaStore) SemanticSearch(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if err := c.ensureCollection(ctx); err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 5
	}

	// Generate query embedding
	embedding, err := c.Embedder.EmbedText(query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	queryReq := chromaQueryRequest{
		QueryEmbeddings: [][]float32{embedding},
		NResults:        limit,
		Include:         []string{"documents", "metadatas", "distances"},
	}

	body, _ := json.Marshal(queryReq)
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/query", c.collectionPath()),
		bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("query failed: %d - %s", resp.StatusCode, string(respBody))
	}

	var result chromaQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return c.parseResults(result)
}

// FindSimilarFiles finds files similar to a given file
func (c *ChromaStore) FindSimilarFiles(ctx context.Context, path string, limit int) ([]SearchResult, error) {
	if err := c.ensureCollection(ctx); err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 5
	}

	// Get the file's embeddings from Chroma
	getBody, _ := json.Marshal(map[string]interface{}{
		"where": map[string]interface{}{
			"path": path,
		},
		"include": []string{"embeddings"},
	})

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/get", c.collectionPath()),
		bytes.NewBuffer(getBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get failed: %d - %s", resp.StatusCode, string(respBody))
	}

	var getResult struct {
		IDs        []string    `json:"ids"`
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&getResult); err != nil {
		return nil, err
	}

	if len(getResult.Embeddings) == 0 {
		return nil, fmt.Errorf("file not found in vector store: %s", path)
	}

	// Use average embedding of all chunks
	avgEmbedding := averageEmbeddings(getResult.Embeddings)

	// Query for similar files (excluding self)
	queryReq := chromaQueryRequest{
		QueryEmbeddings: [][]float32{avgEmbedding},
		NResults:        limit + 10, // Get extra to filter out self
		Include:         []string{"documents", "metadatas", "distances"},
	}

	body, _ := json.Marshal(queryReq)
	req, err = http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/query", c.collectionPath()),
		bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err = c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result chromaQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	results, err := c.parseResults(result)
	if err != nil {
		return nil, err
	}

	// Filter out self and deduplicate by path
	var filtered []SearchResult
	seen := make(map[string]bool)
	for _, r := range results {
		if r.Path == path || seen[r.Path] {
			continue
		}
		seen[r.Path] = true
		filtered = append(filtered, r)
		if len(filtered) >= limit {
			break
		}
	}

	return filtered, nil
}

// CheckDuplicate checks if content is similar to existing notes
func (c *ChromaStore) CheckDuplicate(ctx context.Context, content string, threshold float32) (*SearchResult, error) {
	if threshold <= 0 {
		threshold = 0.95
	}

	results, err := c.SemanticSearch(ctx, content, 1)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	// Convert distance to similarity (assuming cosine distance)
	similarity := 1.0 - results[0].Distance
	if similarity >= threshold {
		return &results[0], nil
	}

	return nil, nil
}

// ReindexVault rebuilds the entire vector index
func (c *ChromaStore) ReindexVault(ctx context.Context, vaultPath string) error {
	if c.CollectionID == "" {
		return nil
	}

	// Delete existing collection
	req, err := http.NewRequestWithContext(ctx, "DELETE",
		fmt.Sprintf("%s/collections/%s", c.basePath(), c.CollectionID), nil)
	if err != nil {
		return err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	c.CollectionID = ""
	return nil
}

// Close implements VectorStore
func (c *ChromaStore) Close() error {
	return nil
}

// parseResults converts Chroma response to SearchResults
func (c *ChromaStore) parseResults(result chromaQueryResponse) ([]SearchResult, error) {
	var results []SearchResult

	if len(result.IDs) == 0 || len(result.IDs[0]) == 0 {
		return results, nil
	}

	for i := 0; i < len(result.IDs[0]); i++ {
		if len(result.Documents) == 0 || len(result.Documents[0]) <= i {
			continue
		}

		metadata := map[string]interface{}{}
		if len(result.Metadatas) > 0 && len(result.Metadatas[0]) > i {
			metadata = result.Metadatas[0][i]
		}

		distance := float32(0)
		if len(result.Distances) > 0 && len(result.Distances[0]) > i {
			distance = result.Distances[0][i]
		}

		path := ""
		if p, ok := metadata["path"].(string); ok {
			path = p
		}

		title := ""
		if t, ok := metadata["title"].(string); ok {
			title = t
		}

		heading := ""
		if h, ok := metadata["heading"].(string); ok {
			heading = h
		}

		results = append(results, SearchResult{
			Path:     path,
			Title:    title,
			Heading:  heading,
			Chunk:    result.Documents[0][i],
			Distance: distance,
		})
	}

	return results, nil
}

// averageEmbeddings calculates the average of multiple embeddings
func averageEmbeddings(embeddings [][]float32) []float32 {
	if len(embeddings) == 0 {
		return nil
	}

	dim := len(embeddings[0])
	avg := make([]float32, dim)

	for _, emb := range embeddings {
		for i, v := range emb {
			avg[i] += v
		}
	}

	for i := range avg {
		avg[i] /= float32(len(embeddings))
	}

	return avg
}

// getTitleFromPath extracts title from path
func getTitleFromPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}
