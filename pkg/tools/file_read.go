package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dogclaw/pkg/claudemd"
	"dogclaw/pkg/types"
)

const maxFileReadChars = 50000 // Max characters to read by default

// FileReadTool implements file reading functionality
type FileReadTool struct{}

func NewFileReadTool() *FileReadTool {
	return &FileReadTool{}
}

func (t *FileReadTool) Name() string      { return "Read" }
func (t *FileReadTool) Aliases() []string { return []string{"file_read", "read_file"} }

func (t *FileReadTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Optional byte offset to start reading from",
			},
			"length": map[string]any{
				"type":        "integer",
				"description": "Optional maximum bytes to read",
			},
		},
		Required: []string{"file_path"},
	}
}

func (t *FileReadTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Read the contents of a file at the specified path. " +
		"Supports reading text files, and can optionally specify offset and length. " +
		"Large files are automatically truncated."
}

func (t *FileReadTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	filePath, ok := input["file_path"].(string)
	if !ok || filePath == "" {
		return &types.ToolResult{
			Data:    "Error: 'file_path' parameter is required",
			IsError: true,
		}, nil
	}

	// Resolve path
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(toolCtx.Cwd, filePath)
	}
	filePath = filepath.Clean(filePath)

	// Check file exists
	info, err := os.Stat(filePath)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Cannot access file '%s': %v", filePath, err),
			IsError: true,
		}, nil
	}

	if info.IsDir() {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: '%s' is a directory, not a file", filePath),
			IsError: true,
		}, nil
	}

	// Read raw content
	rawContent, err := os.ReadFile(filePath)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Failed to read file '%s': %v", filePath, err),
			IsError: true,
		}, nil
	}

	content := string(rawContent)

	// Process @include directives for memory files
	isMemoryFile := claudemd.IsMemoryFilePath(filePath)
	if isMemoryFile {
		// Extract @include references
		processedFiles, err := claudemd.ProcessMemoryFile(filePath, claudemd.Project, make(map[string]bool), 0, "")
		if err == nil && len(processedFiles) > 0 {
			// Build content with includes expanded
			var parts []string
			for _, f := range processedFiles {
				if f.Content != "" {
					parts = append(parts, fmt.Sprintf("## Included file: %s\n\n%s", f.Path, f.Content))
				}
			}
			if len(parts) > 0 {
				content = strings.Join(parts, "\n\n")
			}
		}
	}

	// Handle offset/length
	offset, _ := input["offset"].(int)
	length, _ := input["length"].(int)

	if offset > 0 {
		if offset >= len(content) {
			content = ""
		} else {
			content = content[offset:]
		}
	}
	if length > 0 && length < len(content) {
		content = content[:length]
	}

	// Truncate if too large
	truncated := false
	if len(content) > maxFileReadChars {
		content = content[:maxFileReadChars]
		truncated = true
	}

	result := fmt.Sprintf("File: %s\nSize: %d bytes\nContent:\n%s", filePath, info.Size(), content)
	if truncated {
		result += fmt.Sprintf("\n\n... (truncated to %d characters. Use offset/length to read more)", maxFileReadChars)
	}

	return &types.ToolResult{
		Data:    result,
		IsError: false,
	}, nil
}

func (t *FileReadTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *FileReadTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *FileReadTool) IsDestructive(input map[string]any) bool     { return false }
func (t *FileReadTool) IsEnabled() bool                             { return true }
func (t *FileReadTool) SearchHint() string                          { return "read file contents view source" }
