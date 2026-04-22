package vector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// EmbeddingClient handles communication with OpenAI-compatible embedding APIs
type EmbeddingClient struct {
	BaseURL string
	APIKey  string
	Model   string
	Client  *http.Client
}

// NewEmbeddingClient creates a new embedding client from environment variables
func NewEmbeddingClient() *EmbeddingClient {
	baseURL := os.Getenv("EMBEDDING_API_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	apiKey := os.Getenv("EMBEDDING_API_KEY")
	model := os.Getenv("EMBEDDING_MODEL")
	if model == "" {
		model = "text-embedding-3-small"
	}

	return &EmbeddingClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		Client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// embeddingRequest represents the request body for embedding API
type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// embeddingResponse represents the response from embedding API
type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// EmbedTexts generates embeddings for a batch of texts
func (c *EmbeddingClient) EmbedTexts(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := embeddingRequest{
		Model: c.Model,
		Input: texts,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/embeddings", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp embeddingResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != nil {
			return nil, fmt.Errorf("embedding API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("embedding API returned status %d", resp.StatusCode)
	}

	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("embedding API error: %s", result.Error.Message)
	}

	// Sort embeddings by index to maintain order
	embeddings := make([][]float32, len(texts))
	for _, data := range result.Data {
		if data.Index < len(embeddings) {
			embeddings[data.Index] = data.Embedding
		}
	}

	return embeddings, nil
}

// EmbedText generates embedding for a single text
func (c *EmbeddingClient) EmbedText(text string) ([]float32, error) {
	embeddings, err := c.EmbedTexts([]string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}
