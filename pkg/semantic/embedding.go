// Package semantic provides LLM-based semantic search for memory files.
//
// It uses the LLM to generate lightweight text embeddings and performs
// cosine similarity search to find relevant memories.
package semantic

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"

	"dogclaw/internal/api"
)

const (
	// DefaultEmbeddingDim is the default embedding vector dimension.
	// We use a relatively small dimension since we parse float values
	// from LLM text output rather than using a real embedding model.
	DefaultEmbeddingDim = 32

	// MaxMemoriesToEmbed is the max number of memories to embed in one batch.
	MaxMemoriesToEmbed = 50
)

// EmbeddingConfig holds configuration for semantic search.
type EmbeddingConfig struct {
	// Dimension of the embedding vector.
	Dimension int
	// ModelOverride allows using a different model for embeddings.
	ModelOverride string
	// MaxTokens for the embedding generation request.
	MaxTokens int
}

// DefaultEmbeddingConfig returns sensible defaults.
func DefaultEmbeddingConfig() *EmbeddingConfig {
	return &EmbeddingConfig{
		Dimension: DefaultEmbeddingDim,
		MaxTokens: 4096,
	}
}

// MemoryIndex stores embeddings for a set of memory files.
type MemoryIndex struct {
	mu            sync.RWMutex
	entries       []IndexEntry
	dimension     int
	isInitialized bool
}

// IndexEntry represents a single indexed memory with its embedding.
type IndexEntry struct {
	// Path is the absolute path to the memory file.
	Path string
	// Name is the memory name from frontmatter.
	Name string
	// Description is the description from frontmatter.
	Description string
	// Content is the memory content (without frontmatter).
	Content string
	// Embedding is the vector representation.
	Embedding []float64
	// MtimeMs is the modification time.
	MtimeMs int64
}

// NewMemoryIndex creates a new empty memory index.
func NewMemoryIndex(dimension int) *MemoryIndex {
	if dimension <= 0 {
		dimension = DefaultEmbeddingDim
	}
	return &MemoryIndex{
		entries:   make([]IndexEntry, 0),
		dimension: dimension,
	}
}

// SearchResult holds a search result with similarity score.
type SearchResult struct {
	Entry      IndexEntry
	Similarity float64
}

// Search finds memories semantically matching the query.
// If the index is not initialized with embeddings, falls back to keyword search.
func (idx *MemoryIndex) Search(query string, topK int) []SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if len(idx.entries) == 0 {
		return nil
	}

	// For now, use keyword-based relevance scoring.
	// Real embeddings require an external model API.
	results := keywordSearch(idx.entries, query)

	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}

	return results
}

// keywordSearch performs relevance scoring using term frequency.
func keywordSearch(entries []IndexEntry, query string) []SearchResult {
	queryTerms := strings.Fields(strings.ToLower(query))
	if len(queryTerms) == 0 {
		return nil
	}

	var results []SearchResult

	for _, entry := range entries {
		score := scoreEntry(entry, queryTerms)
		if score > 0 {
			results = append(results, SearchResult{
				Entry:      entry,
				Similarity: score,
			})
		}
	}

	// Sort by similarity descending
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Similarity > results[i].Similarity {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}

// scoreEntry computes a relevance score for an entry against query terms.
func scoreEntry(entry IndexEntry, queryTerms []string) float64 {
	nameLower := strings.ToLower(entry.Name)
	descLower := strings.ToLower(entry.Description)
	contentLower := strings.ToLower(entry.Content)

	var score float64

	for _, term := range queryTerms {
		if len(term) < 2 {
			continue
		}

		// Weighted scoring: name > description > content
		if strings.Contains(nameLower, term) {
			score += 10.0
		}
		if strings.Contains(descLower, term) {
			score += 5.0
		}

		// Count occurrences in content
		count := strings.Count(contentLower, term)
		if count > 0 {
			score += float64(count) * 1.0
		}
	}

	// Normalize by content length to avoid bias toward longer memories
	contentWords := len(strings.Fields(entry.Content))
	if contentWords > 0 {
		score /= math.Log10(float64(contentWords) + 1)
	}

	return score
}

// AddEntries adds entries to the index.
func (idx *MemoryIndex) AddEntries(entries []IndexEntry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.entries = append(idx.entries, entries...)
	idx.isInitialized = true
}

// Clear removes all entries from the index.
func (idx *MemoryIndex) Clear() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.entries = make([]IndexEntry, 0)
	idx.isInitialized = false
}

// Count returns the number of indexed entries.
func (idx *MemoryIndex) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.entries)
}

// IsInitialized returns true if the index has entries.
func (idx *MemoryIndex) IsInitialized() bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.isInitialized
}

// GenerateEmbedding uses the LLM to generate a numeric embedding for text.
// Returns a vector of the configured dimension.
func GenerateEmbedding(ctx context.Context, client *api.Client, text string, dim int) ([]float64, error) {
	if dim <= 0 {
		dim = DefaultEmbeddingDim
	}

	// Truncate text to avoid context overflow
	if len(text) > 4000 {
		text = text[:4000]
	}

	prompt := fmt.Sprintf(`Generate a %d-dimensional numeric embedding vector for the following text.
The vector should capture the semantic meaning. Each value should be between -1 and 1.
Output ONLY a JSON array of %d floats, nothing else.

Text: %s

JSON array:`, dim, dim, text)

	req := &api.MessageRequest{
		Model:     client.Model,
		MaxTokens: dim * 10, // Rough estimate for JSON array output
		Messages: []api.MessageParam{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.1,
	}

	resp, err := client.SendMessage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("embedding generation failed: %w", err)
	}

	var textContent string
	for _, block := range resp.Content {
		if block.Type == "text" {
			textContent += block.Text
		}
	}

	return parseEmbeddingJSON(textContent, dim)
}

// parseEmbeddingJSON extracts a float array from LLM output.
func parseEmbeddingJSON(output string, expectedDim int) ([]float64, error) {
	// Find JSON array in output
	start := strings.Index(output, "[")
	end := strings.LastIndex(output, "]")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON array found in output")
	}

	jsonStr := output[start : end+1]

	var values []float64
	if err := json.Unmarshal([]byte(jsonStr), &values); err != nil {
		return nil, fmt.Errorf("failed to parse embedding JSON: %w", err)
	}

	// Pad or truncate to expected dimension
	for len(values) < expectedDim {
		values = append(values, 0.0)
	}
	if len(values) > expectedDim {
		values = values[:expectedDim]
	}

	return values, nil
}

// CosineSimilarity computes the cosine similarity between two vectors.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
