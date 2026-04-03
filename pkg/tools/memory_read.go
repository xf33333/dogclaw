package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dogclaw/pkg/core"
	"dogclaw/pkg/memory"
	"dogclaw/pkg/types"
)

// MemoryReadTool reads memory entries with freshness annotations.
// It supports scanning the entire memory directory or reading specific files.
type MemoryReadTool struct{}

func NewMemoryReadTool() *MemoryReadTool {
	return &MemoryReadTool{}
}

func (t *MemoryReadTool) Name() string      { return "MemoryRead" }
func (t *MemoryReadTool) Aliases() []string { return []string{"memory_read", "read_memory"} }

func (t *MemoryReadTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"memory_dir": map[string]any{
				"type":        "string",
				"description": "Path to the memory directory (optional, defaults to memory/ under project root)",
			},
			"file_path": map[string]any{
				"type":        "string",
				"description": "Read a specific memory file by path (optional)",
			},
			"type_filter": map[string]any{
				"type":        "string",
				"description": "Filter memories by type: user, feedback, project, or reference (optional)",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of memories to return (default: 50)",
			},
		},
		Required: []string{},
	}
}

func (t *MemoryReadTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Read persistent memory entries with freshness timestamps. " +
		"Memories are point-in-time observations — verify stale claims against current code. " +
		"Supports scanning the entire memory directory, filtering by type, or reading specific files."
}

func (t *MemoryReadTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	memoryDir, _ := input["memory_dir"].(string)
	if memoryDir == "" {
		// Default to .dogclaw/memory/ under project root
		memoryDir = filepath.Join(toolCtx.Cwd, ".dogclaw", "memory")
	} else if !filepath.IsAbs(memoryDir) {
		memoryDir = filepath.Join(toolCtx.Cwd, memoryDir)
	}

	// Check if reading a specific file
	filePath, hasPath := input["file_path"].(string)
	if hasPath && filePath != "" {
		return t.readSingleFile(filePath, memoryDir)
	}

	// Type filter
	typeFilter := ""
	if tf, ok := input["type_filter"].(string); ok {
		typeFilter = strings.ToLower(tf)
	}

	// Limit
	limit := 50
	if l, ok := input["limit"].(int); ok && l > 0 {
		limit = l
	}

	// Scan memory directory
	headers, err := memory.ScanMemoryFiles(memoryDir)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Failed to scan memory directory '%s': %v", memoryDir, err),
			IsError: true,
		}, nil
	}

	if len(headers) == 0 {
		return &types.ToolResult{
			Data: fmt.Sprintf("No memory files found in '%s'.\n\n"+
				"Memory directory: %s\n\n"+
				"Create memories using MemoryWrite tool with format:\n"+
				"---\n"+
				"name: {{memory name}}\n"+
				"description: {{one-line description}}\n"+
				"type: {{user|feedback|project|reference}}\n"+
				"---\n\n{{memory content}}",
				memoryDir, memoryDir,
			),
			IsError: false,
		}, nil
	}

	// Apply type filter
	if typeFilter != "" {
		var filtered []memory.MemoryHeader
		for _, h := range headers {
			if string(h.Type) == typeFilter {
				filtered = append(filtered, h)
			}
		}
		headers = filtered
	}

	if len(headers) == 0 {
		return &types.ToolResult{
			Data:    fmt.Sprintf("No memories found matching type filter '%s'.", typeFilter),
			IsError: false,
		}, nil
	}

	// Cap at limit
	if len(headers) > limit {
		headers = headers[:limit]
	}

	// Ingest file contents
	memories, err := core.Ingest(headers)
	if err != nil {
		// Don't fail entirely — use headers with empty content for unreadable files
	}

	// Build output with freshness annotations
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d memories in '%s':\n\n", len(headers), memoryDir))

	for _, m := range memories {
		age := memory.MemoryFreshnessNote(m.MtimeMs)
		sb.WriteString(fmt.Sprintf("## path: %s | age: %s\n\n",
			m.Path, memory.MemoryAge(m.MtimeMs)))
		if age != "" {
			sb.WriteString(age)
		}
		content := strings.TrimSpace(m.Content)
		if content != "" {
			sb.WriteString(content)
		} else {
			sb.WriteString("(memory file is empty)")
		}
		sb.WriteString("\n\n---\n\n")
	}

	return &types.ToolResult{
		Data:    strings.TrimSuffix(sb.String(), "\n\n---\n\n"),
		IsError: false,
	}, nil
}

// readSingleFile reads a specific memory file with freshness annotation.
func (t *MemoryReadTool) readSingleFile(filePath, memoryDir string) (*types.ToolResult, error) {
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(memoryDir, filePath)
	}
	filePath = filepath.Clean(filePath)

	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &types.ToolResult{
				Data:    fmt.Sprintf("Error: Memory file not found: '%s'", filePath),
				IsError: true,
			}, nil
		}
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Cannot access '%s': %v", filePath, err),
			IsError: true,
		}, nil
	}

	content, err := core.ReadContentWithoutFrontmatter(filePath)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Failed to read '%s': %v", filePath, err),
			IsError: true,
		}, nil
	}

	mtimeMs := info.ModTime().UnixMilli()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Memory: %s (modified: %s)\n\n", filePath, memory.MemoryAge(mtimeMs)))

	if freshness := memory.MemoryFreshnessNote(mtimeMs); freshness != "" {
		sb.WriteString(freshness)
	}

	if strings.TrimSpace(content) == "" {
		sb.WriteString("(memory file has only frontmatter, no content)")
	} else {
		sb.WriteString(content)
	}

	return &types.ToolResult{
		Data:    sb.String(),
		IsError: false,
	}, nil
}

func (t *MemoryReadTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *MemoryReadTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *MemoryReadTool) IsDestructive(input map[string]any) bool     { return false }
func (t *MemoryReadTool) IsEnabled() bool                             { return true }
func (t *MemoryReadTool) SearchHint() string {
	return "read memory files persistent context recall"
}
