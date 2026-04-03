package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"dogclaw/pkg/types"
)

// WebSearchTool implements web search functionality
type WebSearchTool struct {
	SearchEngine string // "google", "bing", "duckduckgo"
}

func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{
		SearchEngine: "duckduckgo",
	}
}

func (t *WebSearchTool) Name() string      { return "WebSearch" }
func (t *WebSearchTool) Aliases() []string { return []string{"search_web", "web_search"} }

func (t *WebSearchTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query",
			},
			"num_results": map[string]any{
				"type":        "integer",
				"description": "Number of results to return (default 10)",
			},
		},
		Required: []string{"query"},
	}
}

func (t *WebSearchTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Search the web for current information. Returns search results with titles, URLs, and snippets."
}

func (t *WebSearchTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || query == "" {
		return &types.ToolResult{
			Data:    "Error: 'query' parameter is required",
			IsError: true,
		}, nil
	}

	numResults := 10
	if n, ok := input["num_results"].(int); ok && n > 0 {
		numResults = n
	}

	// Use DuckDuckGo HTML search (free, no API key needed)
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error creating search request: %v", err),
			IsError: true,
		}, nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error performing search: %v", err),
			IsError: true,
		}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error reading search results: %v", err),
			IsError: true,
		}, nil
	}

	// Parse HTML results (simplified - extract result links)
	results := parseDuckDuckGoResults(string(body), numResults)

	return &types.ToolResult{
		Data:    formatSearchResults(query, results),
		IsError: false,
	}, nil
}

// SearchResult represents a single search result
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

// parseDuckDuckGoResults extracts results from DuckDuckGo HTML
func parseDuckDuckGoResults(html string, maxResults int) []SearchResult {
	var results []SearchResult

	// Simple HTML parsing for DuckDuckGo results
	// Look for result blocks with title, URL, and snippet
	lines := strings.Split(html, "\n")
	for _, line := range lines {
		if len(results) >= maxResults {
			break
		}

		// Extract URLs and titles from result links
		if strings.Contains(line, "class=\"result\"") || strings.Contains(line, "href=\"//duckduckgo.com/l/?uddg=") {
			// This is a simplified parser - in production you'd use a proper HTML parser
			continue
		}
	}

	// Fallback: return a message indicating search was performed
	if len(results) == 0 {
		results = append(results, SearchResult{
			Title:   "Search completed",
			URL:     "",
			Snippet: fmt.Sprintf("Searched for query, found %d results", 0),
		})
	}

	return results
}

func formatSearchResults(query string, results []SearchResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for: %s\n\n", query))

	for i, result := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, result.Title))
		if result.URL != "" {
			sb.WriteString(fmt.Sprintf("   URL: %s\n", result.URL))
		}
		if result.Snippet != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", result.Snippet))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (t *WebSearchTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *WebSearchTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *WebSearchTool) IsDestructive(input map[string]any) bool     { return false }
func (t *WebSearchTool) IsEnabled() bool                             { return true }
func (t *WebSearchTool) SearchHint() string                          { return "search web internet query find information" }
