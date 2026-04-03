package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dogclaw/pkg/types"
)

// FileEditTool implements partial file editing (string replacement)
type FileEditTool struct{}

func NewFileEditTool() *FileEditTool {
	return &FileEditTool{}
}

func (t *FileEditTool) Name() string      { return "Edit" }
func (t *FileEditTool) Aliases() []string { return []string{"file_edit", "edit_file"} }

func (t *FileEditTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Path to the file to edit",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "The exact text to find and replace",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "The text to replace with",
			},
		},
		Required: []string{"file_path", "old_string", "new_string"},
	}
}

func (t *FileEditTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Perform a partial edit on a file by replacing old_string with new_string. " +
		"The old_string must match exactly (including whitespace). " +
		"Use Write to create or completely overwrite a file."
}

func (t *FileEditTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	filePath, ok := input["file_path"].(string)
	if !ok || filePath == "" {
		return &types.ToolResult{
			Data:    "Error: 'file_path' parameter is required",
			IsError: true,
		}, nil
	}

	oldStr, ok := input["old_string"].(string)
	if !ok || oldStr == "" {
		return &types.ToolResult{
			Data:    "Error: 'old_string' parameter is required",
			IsError: true,
		}, nil
	}

	newStr, ok := input["new_string"].(string)
	if !ok {
		return &types.ToolResult{
			Data:    "Error: 'new_string' parameter is required",
			IsError: true,
		}, nil
	}

	// Resolve path
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(toolCtx.Cwd, filePath)
	}
	filePath = filepath.Clean(filePath)

	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Cannot read file '%s': %v", filePath, err),
			IsError: true,
		}, nil
	}

	contentStr := string(content)

	// Check if old_string exists
	if !strings.Contains(contentStr, oldStr) {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Could not find the exact text in %s. The text must match exactly (including whitespace).", filePath),
			IsError: true,
		}, nil
	}

	// Check for multiple occurrences
	count := strings.Count(contentStr, oldStr)
	if count > 1 {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: The text appears %d times in the file. Please provide more context to make it unique.", count),
			IsError: true,
		}, nil
	}

	// Perform replacement
	newContent := strings.Replace(contentStr, oldStr, newStr, 1)

	// Write back
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Failed to write file '%s': %v", filePath, err),
			IsError: true,
		}, nil
	}

	return &types.ToolResult{
		Data:    fmt.Sprintf("Successfully edited %s", filePath),
		IsError: false,
	}, nil
}

func (t *FileEditTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *FileEditTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *FileEditTool) IsDestructive(input map[string]any) bool     { return false }
func (t *FileEditTool) IsEnabled() bool                             { return true }
func (t *FileEditTool) SearchHint() string                          { return "edit modify replace string partial" }
