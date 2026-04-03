package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"dogclaw/pkg/types"
)

// WebFetchTool implements URL content fetching
type WebFetchTool struct{}

func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{}
}

func (t *WebFetchTool) Name() string      { return "WebFetch" }
func (t *WebFetchTool) Aliases() []string { return []string{"fetch", "url_fetch", "web_fetch"} }

func (t *WebFetchTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The URL to fetch content from",
			},
			"max_length": map[string]any{
				"type":        "integer",
				"description": "Maximum characters to return (default 5000)",
			},
		},
		Required: []string{"url"},
	}
}

func (t *WebFetchTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Fetch and extract readable content from a URL. " +
		"Use this to read web pages, articles, and online documentation."
}

func (t *WebFetchTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	urlStr, ok := input["url"].(string)
	if !ok || urlStr == "" {
		return &types.ToolResult{
			Data:    "Error: 'url' parameter is required",
			IsError: true,
		}, nil
	}

	maxLength := 5000
	if maxLen, ok := input["max_length"].(int); ok && maxLen > 0 {
		maxLength = maxLen
	}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error creating request: %v", err),
			IsError: true,
		}, nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; DogClaw/1.0)")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error fetching URL: %v", err),
			IsError: true,
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: HTTP %d - %s", resp.StatusCode, resp.Status),
			IsError: true,
		}, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error reading response: %v", err),
			IsError: true,
		}, nil
	}

	content := string(body)
	// Simple HTML stripping
	content = stripHTML(content)

	if len(content) > maxLength {
		content = content[:maxLength] + "... [truncated]"
	}

	return &types.ToolResult{
		Data:    fmt.Sprintf("URL: %s\nContent:\n%s", urlStr, content),
		IsError: false,
	}, nil
}

func stripHTML(html string) string {
	// Very simple HTML stripping - in production you'd use a proper parser
	var result strings.Builder
	inTag := false
	for _, char := range html {
		if char == '<' {
			inTag = true
			continue
		}
		if char == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(char)
		}
	}
	return strings.TrimSpace(result.String())
}

func (t *WebFetchTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *WebFetchTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *WebFetchTool) IsDestructive(input map[string]any) bool     { return false }
func (t *WebFetchTool) IsEnabled() bool                             { return true }
func (t *WebFetchTool) SearchHint() string                          { return "fetch url webpage article download content" }
