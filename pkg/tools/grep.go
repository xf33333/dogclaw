package tools

import (
	"context"
	"fmt"
	"strings"

	"dogclaw/pkg/types"
)

// GrepTool implements content search using ripgrep
type GrepTool struct{}

func NewGrepTool() *GrepTool {
	return &GrepTool{}
}

func (t *GrepTool) Name() string      { return "Grep" }
func (t *GrepTool) Aliases() []string { return []string{"search", "rg", "ripgrep"} }

func (t *GrepTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Regex pattern to search for",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File or directory to search in",
			},
			"glob": map[string]any{
				"type":        "string",
				"description": "Optional glob to filter files (e.g., '*.go')",
			},
			"output_mode": map[string]any{
				"type":        "string",
				"description": "Output mode: 'content' shows matching lines, 'files_with_matches' shows file names only",
				"enum":        []string{"content", "files_with_matches"},
			},
			"case_sensitive": map[string]any{
				"type":        "boolean",
				"description": "Whether to use case-sensitive matching",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return",
			},
		},
		Required: []string{"pattern"},
	}
}

func (t *GrepTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Search for a regex pattern in files using ripgrep. " +
		"Fast content search across directories with regex support. " +
		"Returns matching lines with file paths and line numbers."
}

func (t *GrepTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	pattern, ok := input["pattern"].(string)
	if !ok || pattern == "" {
		return &types.ToolResult{
			Data:    "Error: 'pattern' parameter is required",
			IsError: true,
		}, nil
	}

	// Build ripgrep command
	args := []string{"rg"}

	if outputMode, ok := input["output_mode"].(string); ok && outputMode == "files_with_matches" {
		args = append(args, "-l")
	}

	if caseSensitive, ok := input["case_sensitive"].(bool); !ok || !caseSensitive {
		args = append(args, "-i") // case insensitive by default
	}

	if maxResults, ok := input["max_results"].(int); ok && maxResults > 0 {
		args = append(args, "-m", fmt.Sprintf("%d", maxResults))
	} else {
		args = append(args, "-m", "50") // default limit
	}

	if glob, ok := input["glob"].(string); ok && glob != "" {
		args = append(args, "-g", glob)
	}

	args = append(args, "--line-number")
	args = append(args, pattern)

	if path, ok := input["path"].(string); ok && path != "" {
		args = append(args, path)
	} else {
		args = append(args, ".")
	}

	cmd := fmt.Sprintf("%s 2>&1", strings.Join(args, " "))

	// Execute via bash
	bashTool := NewBashTool()
	result, err := bashTool.Call(ctx, map[string]any{"command": cmd}, toolCtx, nil)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error executing grep: %v", err),
			IsError: true,
		}, nil
	}

	output := result.Data.(string)
	if strings.Contains(output, "No matches found") || output == "" {
		return &types.ToolResult{
			Data:    fmt.Sprintf("No matches found for pattern: %s", pattern),
			IsError: false,
		}, nil
	}

	return &types.ToolResult{
		Data:    output,
		IsError: false,
	}, nil
}

func (t *GrepTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *GrepTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *GrepTool) IsDestructive(input map[string]any) bool     { return false }
func (t *GrepTool) IsEnabled() bool                             { return true }
func (t *GrepTool) SearchHint() string                          { return "search content regex ripgrep find text" }
