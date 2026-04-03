package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"dogclaw/pkg/types"
)

// FileWriteTool implements file writing functionality
type FileWriteTool struct{}

func NewFileWriteTool() *FileWriteTool {
	return &FileWriteTool{}
}

func (t *FileWriteTool) Name() string      { return "Write" }
func (t *FileWriteTool) Aliases() []string { return []string{"file_write", "create_file"} }

func (t *FileWriteTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Path to the file to create/overwrite",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		Required: []string{"file_path", "content"},
	}
}

func (t *FileWriteTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Create a new file or completely overwrite an existing file. " +
		"Use Edit for partial modifications. Creates parent directories if needed."
}

func (t *FileWriteTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	filePath, ok := input["file_path"].(string)
	if !ok || filePath == "" {
		return &types.ToolResult{
			Data:    "Error: 'file_path' parameter is required",
			IsError: true,
		}, nil
	}

	content, ok := input["content"].(string)
	if !ok {
		return &types.ToolResult{
			Data:    "Error: 'content' parameter is required and must be a string",
			IsError: true,
		}, nil
	}

	// Resolve path
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(toolCtx.Cwd, filePath)
	}
	filePath = filepath.Clean(filePath)

	// Create parent directories
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Failed to create directory '%s': %v", dir, err),
			IsError: true,
		}, nil
	}

	// Write file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Failed to write file '%s': %v", filePath, err),
			IsError: true,
		}, nil
	}

	return &types.ToolResult{
		Data:    fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), filePath),
		IsError: false,
	}, nil
}

func (t *FileWriteTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *FileWriteTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *FileWriteTool) IsDestructive(input map[string]any) bool     { return true }
func (t *FileWriteTool) IsEnabled() bool                             { return true }
func (t *FileWriteTool) SearchHint() string                          { return "create write file new content" }
