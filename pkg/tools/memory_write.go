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

// MemoryWriteTool writes or updates memory files.
// It supports create, update, delete, and dedup operations.
type MemoryWriteTool struct{}

func NewMemoryWriteTool() *MemoryWriteTool {
	return &MemoryWriteTool{}
}

func (t *MemoryWriteTool) Name() string { return "MemoryWrite" }
func (t *MemoryWriteTool) Aliases() []string {
	return []string{"memory_write", "write_memory", "save_memory"}
}

func (t *MemoryWriteTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Action to perform: 'create', 'update', 'delete', or 'dedup'",
				"enum":        []string{"create", "update", "delete", "dedup"},
			},
			"memory_dir": map[string]any{
				"type":        "string",
				"description": "Path to the memory directory (optional)",
			},
			"file_path": map[string]any{
				"type":        "string",
				"description": "Target memory file path (required for update/delete)",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Name of the memory (required for create)",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "One-line description of the memory",
			},
			"memory_type": map[string]any{
				"type":        "string",
				"description": "Type of memory: user, feedback, project, or reference",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Memory body content",
			},
		},
		Required: []string{"action"},
	}
}

func (t *MemoryWriteTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Write persistent memories to file-based storage. " +
		"Supports create, update, delete, and dedup operations. " +
		"Memories use YAML frontmatter (name, description, type) and markdown content. " +
		"Always check for existing memories before creating new ones."
}

func (t *MemoryWriteTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	action, ok := input["action"].(string)
	if !ok || action == "" {
		return &types.ToolResult{
			Data:    "Error: 'action' parameter is required. Must be 'create', 'update', 'delete', or 'dedup'.",
			IsError: true,
		}, nil
	}

	memoryDir, _ := input["memory_dir"].(string)
	if memoryDir == "" {
		memoryDir = filepath.Join(toolCtx.Cwd, ".dogclaw", "memory")
	} else if !filepath.IsAbs(memoryDir) {
		memoryDir = filepath.Join(toolCtx.Cwd, memoryDir)
	}

	switch action {
	case "create":
		return t.createMemory(input, toolCtx, memoryDir)
	case "update":
		return t.updateMemory(input, toolCtx, memoryDir)
	case "delete":
		return t.deleteMemory(input, toolCtx, memoryDir)
	case "dedup":
		return t.dedupMemories(input, toolCtx, memoryDir)
	default:
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Unknown action '%s'. Must be 'create', 'update', 'delete', or 'dedup'.", action),
			IsError: true,
		}, nil
	}
}

// createMemory writes a new memory file with frontmatter.
func (t *MemoryWriteTool) createMemory(input map[string]any, toolCtx types.ToolUseContext, memoryDir string) (*types.ToolResult, error) {
	name, _ := input["name"].(string)
	if name == "" {
		return &types.ToolResult{
			Data:    "Error: 'name' parameter is required for 'create' action.",
			IsError: true,
		}, nil
	}

	description, _ := input["description"].(string)
	if description == "" {
		description = name
	}

	memType, _ := input["memory_type"].(string)
	mt := memory.ParseMemoryType(memType)
	if mt == "" {
		mt = memory.MemoryTypeProject // default type
	}

	content, _ := input["content"].(string)

	// Generate filename from name
	filename := sanitizeFilename(name) + ".md"
	filePath := filepath.Join(memoryDir, filename)

	// Check for duplicates
	headers, err := memory.ScanMemoryFiles(memoryDir)
	if err == nil {
		for _, h := range headers {
			if strings.EqualFold(h.Filename, filename) {
				return &types.ToolResult{
					Data: fmt.Sprintf("Error: Memory file already exists: '%s'.\n"+
						"Description: %s\nType: %s\n\nUse 'update' action to modify existing memories.",
						h.Filename, h.Description, h.Type),
					IsError: true,
				}, nil
			}
		}
	}

	// Create frontmatter + content
	fileContent := buildFrontmatterContent(name, description, mt, content)

	// Ensure memory directory exists
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Failed to create memory directory '%s': %v", memoryDir, err),
			IsError: true,
		}, nil
	}

	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Failed to write memory file '%s': %v", filePath, err),
			IsError: true,
		}, nil
	}

	return &types.ToolResult{
		Data: fmt.Sprintf("Created memory:\n"+
			"File: %s\n"+
			"Type: %s\n"+
			"Description: %s\n\n"+
			"Content written (%d bytes).",
			filename, string(mt), description, len(fileContent)),
		IsError: false,
	}, nil
}

// updateMemory updates an existing memory file.
func (t *MemoryWriteTool) updateMemory(input map[string]any, toolCtx types.ToolUseContext, memoryDir string) (*types.ToolResult, error) {
	filePath, ok := input["file_path"].(string)
	if !ok || filePath == "" {
		return &types.ToolResult{
			Data:    "Error: 'file_path' parameter is required for 'update' action.",
			IsError: true,
		}, nil
	}

	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(memoryDir, filePath)
	}
	filePath = filepath.Clean(filePath)

	// Read existing frontmatter
	existingDesc, existingType := readExistingFrontmatter(filePath)

	// Update fields
	name := filepath.Base(filePath)
	name = strings.TrimSuffix(name, ".md")
	description, _ := input["description"].(string)
	if description == "" {
		description = existingDesc
	}

	memType, _ := input["memory_type"].(string)
	mt := memory.ParseMemoryType(memType)
	if mt == "" {
		mt = existingType
	}

	content, _ := input["content"].(string)

	fileContent := buildFrontmatterContent(name, description, mt, content)

	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Failed to update memory file '%s': %v", filePath, err),
			IsError: true,
		}, nil
	}

	return &types.ToolResult{
		Data: fmt.Sprintf("Updated memory: %s\n"+
			"Description: %s\n"+
			"Type: %s\n\n"+
			"Content written (%d bytes).",
			filePath, description, string(mt), len(fileContent)),
		IsError: false,
	}, nil
}

// deleteMemory removes a memory file.
func (t *MemoryWriteTool) deleteMemory(input map[string]any, toolCtx types.ToolUseContext, memoryDir string) (*types.ToolResult, error) {
	filePath, ok := input["file_path"].(string)
	if !ok || filePath == "" {
		return &types.ToolResult{
			Data:    "Error: 'file_path' parameter is required for 'delete' action.",
			IsError: true,
		}, nil
	}

	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(memoryDir, filePath)
	}
	filePath = filepath.Clean(filePath)

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return &types.ToolResult{
				Data:    fmt.Sprintf("Error: Memory file not found: '%s'", filePath),
				IsError: true,
			}, nil
		}
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Failed to delete '%s': %v", filePath, err),
			IsError: true,
		}, nil
	}

	return &types.ToolResult{
		Data:    fmt.Sprintf("Deleted memory file: %s", filePath),
		IsError: false,
	}, nil
}

// dedupMemories scans for memories with similar content and suggests merges.
// This implements Phase 4 automatic deduplication.
func (t *MemoryWriteTool) dedupMemories(input map[string]any, toolCtx types.ToolUseContext, memoryDir string) (*types.ToolResult, error) {
	headers, err := memory.ScanMemoryFiles(memoryDir)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Failed to scan memory directory: %v", err),
			IsError: true,
		}, nil
	}

	if len(headers) < 2 {
		return &types.ToolResult{
			Data:    "Not enough memories to compare for duplicates (need at least 2).",
			IsError: false,
		}, nil
	}

	// Ingest all memories for content comparison
	memories, err := core.Ingest(headers)
	if err != nil {
		memories = []memory.RelevantMemory{}
	}

	// Find potential duplicates using content similarity
	type pair struct {
		A, B memory.RelevantMemory
	}
	var duplicates []pair

	for i := 0; i < len(memories); i++ {
		for j := i + 1; j < len(memories); j++ {
			if areSimilar(memories[i], memories[j]) {
				duplicates = append(duplicates, pair{memories[i], memories[j]})
			}
		}
	}

	if len(duplicates) == 0 {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Scanned %d memories — no potential duplicates found.", len(memories)),
			IsError: false,
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d potential duplicate pairs:\n\n", len(duplicates)))
	for _, d := range duplicates {
		sb.WriteString(fmt.Sprintf("Pair 1: %s\n", d.A.Path))
		if d.A.Content != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", truncate(d.A.Content, 100)))
		}
		sb.WriteString(fmt.Sprintf("Pair 2: %s\n", d.B.Path))
		if d.B.Content != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", truncate(d.B.Content, 100)))
		}
		sb.WriteString("\n---\n\n")
	}

	sb.WriteString("\nTo merge these, use the 'update' action to combine content, then 'delete' to remove the duplicate.")

	return &types.ToolResult{
		Data:    strings.TrimSpace(sb.String()),
		IsError: false,
	}, nil
}

// buildFrontmatterContent creates the frontmatter + content string.
func buildFrontmatterContent(name, description string, memType memory.MemoryType, content string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", sanitizeFrontmatterValue(name)))
	sb.WriteString(fmt.Sprintf("description: %s\n", sanitizeFrontmatterValue(description)))
	sb.WriteString(fmt.Sprintf("type: %s\n", string(memType)))
	sb.WriteString("---\n")
	if strings.TrimSpace(content) != "" {
		sb.WriteString("\n")
		sb.WriteString(content)
		sb.WriteString("\n")
	}
	return sb.String()
}

// sanitizeFrontmatterValue escapes values for YAML frontmatter.
func sanitizeFrontmatterValue(s string) string {
	s = strings.TrimSpace(s)
	// Quote if contains special characters
	if strings.ContainsAny(s, ":#'") || strings.HasPrefix(s, "-") {
		s = `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

// sanitizeFilename creates a safe filename from a name.
func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		" ", "-",
		"/", "_",
		"\\", "_",
		"?", "_",
		"*", "_",
		":", "-",
		"<", "_",
		">", "_",
		"|", "_",
	)
	name = replacer.Replace(name)
	// Collapse multiple hyphens/underscores
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	name = strings.Trim(name, "-_")
	if name == "" {
		name = "unnamed"
	}
	return strings.ToLower(name)
}

// readExistingFrontmatter reads frontmatter from an existing file.
func readExistingFrontmatter(filePath string) (description string, memType memory.MemoryType) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", ""
	}
	defer f.Close()

	inFM := false
	for i := 0; i < memory.FrontmatterMaxLines; i++ {
		var line string
		_, err := fmt.Fscanln(f, &line)
		if err != nil {
			break
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			if !inFM {
				inFM = true
			} else {
				return description, memType
			}
			continue
		}
		if inFM {
			if idx := strings.Index(trimmed, ":"); idx > 0 {
				key := strings.TrimSpace(trimmed[:idx])
				value := strings.TrimSpace(trimmed[idx+1:])
				if len(value) >= 2 {
					if (value[0] == '"' && value[len(value)-1] == '"') ||
						(value[0] == '\'' && value[len(value)-1] == '\'') {
						value = value[1 : len(value)-1]
					}
				}
				switch key {
				case "description":
					description = value
				case "type":
					memType = memory.ParseMemoryType(value)
				}
			}
		}
	}
	return description, memType
}

// areSimilar checks if two memories have similar content using simple heuristics.
func areSimilar(a, b memory.RelevantMemory) bool {
	if a.Content == "" || b.Content == "" {
		return false
	}

	// Normalize content for comparison
	contentA := normalizeForCompare(a.Content)
	contentB := normalizeForCompare(b.Content)

	// If content is very short, skip
	if len(contentA) < 20 || len(contentB) < 20 {
		return false
	}

	// Check significant overlap using shared word ratio
	wordsA := strings.Fields(contentA)
	wordsB := strings.Fields(contentB)

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return false
	}

	setA := make(map[string]bool)
	for _, w := range wordsA {
		setA[w] = true
	}

	common := 0
	for _, w := range wordsB {
		if setA[w] {
			common++
		}
	}

	minLen := len(wordsA)
	if len(wordsB) < minLen {
		minLen = len(wordsB)
	}

	if minLen == 0 {
		return false
	}

	ratio := float64(common) / float64(minLen)
	return ratio > 0.6 // 60% shared words threshold
}

// normalizeForCompare normalizes content for similarity comparison.
func normalizeForCompare(content string) string {
	content = strings.ToLower(content)
	// Remove common formatting
	content = strings.NewReplacer("##", "", "**", "", "*", "", "-", "", "\n", " ").Replace(content)
	return strings.Join(strings.Fields(content), " ")
}

// truncate truncates a string to maxLen.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (t *MemoryWriteTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *MemoryWriteTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *MemoryWriteTool) IsDestructive(input map[string]any) bool     { return false } // only delete is destructive, but we track it
func (t *MemoryWriteTool) IsEnabled() bool                             { return true }
func (t *MemoryWriteTool) SearchHint() string {
	return "write save memory create update persistent context"
}
