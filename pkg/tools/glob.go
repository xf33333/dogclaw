package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"dogclaw/pkg/types"
)

// GlobTool implements file pattern matching search
type GlobTool struct{}

func NewGlobTool() *GlobTool {
	return &GlobTool{}
}

func (t *GlobTool) Name() string      { return "Glob" }
func (t *GlobTool) Aliases() []string { return []string{"file_glob", "pattern_search"} }

func (t *GlobTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern to match files (e.g., '**/*.go')",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Optional directory to search in (defaults to cwd)",
			},
		},
		Required: []string{"pattern"},
	}
}

func (t *GlobTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Find files matching a glob pattern. Supports ** for recursive matching. " +
		"Use this to find files by name pattern."
}

func (t *GlobTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	pattern, ok := input["pattern"].(string)
	if !ok || pattern == "" {
		return &types.ToolResult{
			Data:    "Error: 'pattern' parameter is required",
			IsError: true,
		}, nil
	}

	searchPath := toolCtx.Cwd
	if path, ok := input["path"].(string); ok && path != "" {
		if !filepath.IsAbs(path) {
			searchPath = filepath.Join(toolCtx.Cwd, path)
		} else {
			searchPath = path
		}
	}

	// Use bash to execute glob (Go's filepath.Glob doesn't support **)
	cmd := fmt.Sprintf("find %s -path %s 2>/dev/null | head -n 100", QuoteShellArg(searchPath), QuoteShellArg(pattern))

	// Execute via bash tool
	bashTool := NewBashTool()
	result, err := bashTool.Call(ctx, map[string]any{"command": cmd}, toolCtx, nil)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error executing glob: %v", err),
			IsError: true,
		}, nil
	}

	// Correctly pass through result data and error status
	if result.IsError {
		return result, nil
	}

	// Parse results
	output := result.Data.(string)
	lines := strings.Split(output, "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.Contains(line, "**Bash**") && !strings.Contains(line, "command:") && !strings.Contains(line, "output:") {
			files = append(files, line)
		}
	}

	if len(files) == 0 {
		return &types.ToolResult{
			Data:    fmt.Sprintf("No files found matching pattern: %s in %s", pattern, searchPath),
			IsError: false,
		}, nil
	}

	return &types.ToolResult{
		Data:    fmt.Sprintf("Found %d file(s) matching '%s' in %s:\n%s", len(files), pattern, searchPath, strings.Join(files, "\n")),
		IsError: false,
	}, nil
}

func (t *GlobTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *GlobTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *GlobTool) IsDestructive(input map[string]any) bool     { return false }
func (t *GlobTool) IsEnabled() bool                             { return true }
func (t *GlobTool) SearchHint() string                          { return "find files pattern glob wildcard" }
