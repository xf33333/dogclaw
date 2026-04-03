// Package compactmem provides LLM-assisted memory compaction.
//
// It identifies stale or redundant memories and uses the LLM to
// merge, summarize, or discard them.
package compactmem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dogclaw/internal/api"
	"dogclaw/pkg/core"
	"dogclaw/pkg/memory"
)

// CompactionConfig holds configuration for memory compaction.
type CompactionConfig struct {
	// Enabled turns compaction on/off.
	Enabled bool
	// StaleThreshold is the age after which a memory is considered stale.
	StaleThreshold time.Duration
	// MaxMemories is the max number of memory files before triggering compaction.
	MaxMemories int
	// CompactRatio is the target fraction of memories to keep after compaction.
	CompactRatio float64 // e.g., 0.6 means keep 60%, compress 40%
	// MinMemories is the minimum number of memories — compaction won't go below this.
	MinMemories int
}

// DefaultCompactionConfig returns sensible defaults.
func DefaultCompactionConfig() *CompactionConfig {
	return &CompactionConfig{
		Enabled:        true,
		StaleThreshold: 30 * 24 * time.Hour, // 30 days
		MaxMemories:    100,
		CompactRatio:   0.7,
		MinMemories:    5,
	}
}

// CompactionResult holds the result of a compaction operation.
type CompactionResult struct {
	OriginalCount int
	NewCount      int
	Compacted     []string // Paths of compacted files
	Removed       []string // Paths of removed files
	Errors        []string
}

// ShouldCompact checks if the memory directory needs compaction.
func ShouldCompact(memoryDir string, config *CompactionConfig) bool {
	if !config.Enabled {
		return false
	}

	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		return false
	}

	mdCount := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(e.Name()), ".md") &&
			e.Name() != memory.EntrypointName {
			mdCount++
		}
	}

	if mdCount > config.MaxMemories {
		return true
	}

	// Check for stale memories
	return hasStaleMemories(memoryDir, entries, config)
}

// hasStaleMemories checks if enough memories are stale to warrant compaction.
func hasStaleMemories(memoryDir string, entries []os.DirEntry, config *CompactionConfig) bool {
	staleCount := 0
	for _, e := range entries {
		if e.IsDir() || e.Name() == memory.EntrypointName {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		if time.Since(info.ModTime()) > config.StaleThreshold {
			staleCount++
		}
	}

	// Compact if >40% of memories are stale
	total := len(entries) - 1 // exclude entrypoint
	return total > 0 && float64(staleCount)/float64(total) > 0.4
}

// Compact performs LLM-assisted memory compaction.
func Compact(ctx context.Context, client *api.Client, memoryDir string, config *CompactionConfig) (*CompactionResult, error) {
	result := &CompactionResult{}

	// Scan memories
	headers, err := memory.ScanMemoryFiles(memoryDir)
	if err != nil {
		return nil, fmt.Errorf("scan memory files: %w", err)
	}

	if len(headers) == 0 {
		return result, nil
	}

	result.OriginalCount = len(headers)

	// Read full contents
	memories, err := core.Ingest(headers)
	if err != nil {
		return nil, fmt.Errorf("ingest memories: %w", err)
	}

	// Classify memories: stale vs fresh
	var staleMemories, freshMemories []memory.RelevantMemory
	for _, m := range memories {
		if memory.ShouldCompactMemory(time.UnixMilli(m.MtimeMs)) {
			staleMemories = append(staleMemories, m)
		} else {
			freshMemories = append(freshMemories, m)
		}
	}

	// Group stale memories by type for batch compaction
	staleByType := make(map[memory.MemoryType][]memory.RelevantMemory)
	for _, m := range staleMemories {
		mType := detectType(m)
		staleByType[mType] = append(staleByType[mType], m)
	}

	// Compact each type group
	for memType, group := range staleByType {
		if len(group) <= 1 {
			continue
		}

		compacted, err := compactGroup(ctx, client, group, memType)
		if err != nil {
			result.Errors = append(result.Errors,
				fmt.Sprintf("failed to compact %s memories: %v", memType, err))
			continue
		}

		for _, old := range group {
			result.Compacted = append(result.Compacted, old.Path)
		}
		result.NewCount += len(compacted)
	}

	// Count fresh memories that were untouched
	for _, m := range freshMemories {
		// Check if this was part of a compacted group
		wasCompacted := false
		for _, c := range result.Compacted {
			if c == m.Path {
				wasCompacted = true
				break
			}
		}
		if !wasCompacted {
			result.NewCount++
		}
	}

	return result, nil
}

// compactGroup uses LLM to merge multiple stale memories into fewer entries.
func compactGroup(ctx context.Context, client *api.Client, memories []memory.RelevantMemory, memType memory.MemoryType) ([]memory.RelevantMemory, error) {
	// Build prompt with all stale memories
	var sb strings.Builder
	sb.WriteString("The following memory files are stale and should be consolidated.\n\n")
	sb.WriteString(fmt.Sprintf("Memory type: %s\n\n", memType))

	for i, m := range memories {
		content, _ := core.ReadContentWithoutFrontmatter(m.Path)
		sb.WriteString(fmt.Sprintf("=== Memory %d (path: %s) ===\n%s\n\n", i+1, m.Path, content))
	}

	sb.WriteString("Instructions:\n")
	sb.WriteString("1. Analyze all the memories above and identify overlapping or outdated content.\n")
	sb.WriteString("2. Create a consolidated summary that preserves the important information.\n")
	sb.WriteString("3. Output a JSON array of new memory entries.\n")
	sb.WriteString("4. Each entry should have: name, description, content.\n")
	sb.WriteString("5. Merge similar memories, discard clearly outdated ones.\n")
	sb.WriteString("6. Keep the output concise.\n\n")
	sb.WriteString("Output ONLY valid JSON array:\n")

	req := &api.MessageRequest{
		Model:     client.Model,
		MaxTokens: 4096,
		Messages: []api.MessageParam{
			{Role: "user", Content: sb.String()},
		},
		Temperature: 0.3,
	}

	resp, err := client.SendMessage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM compaction request failed: %w", err)
	}

	var textContent string
	for _, block := range resp.Content {
		if block.Type == "text" {
			textContent += block.Text
		}
	}

	return parseCompactedOutput(textContent, memType, memories)
}

// parseCompactedOutput extracts the compacted memories from LLM output.
func parseCompactedOutput(output string, memType memory.MemoryType, originals []memory.RelevantMemory) ([]memory.RelevantMemory, error) {
	// Find JSON array
	start := strings.Index(output, "[")
	end := strings.LastIndex(output, "]")
	if start == -1 || end == -1 {
		return nil, fmt.Errorf("no JSON array in compaction output")
	}

	var entries []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}

	if err := json.Unmarshal([]byte(output[start:end+1]), &entries); err != nil {
		return nil, fmt.Errorf("failed to parse compacted memories: %w", err)
	}

	var result []memory.RelevantMemory
	for _, e := range entries {
		if len(result) >= len(originals) {
			break // Don't create more than original
		}

		content := formatMemoryMarkdown(e.Name, e.Description, memType, e.Content)

		result = append(result, memory.RelevantMemory{
			Path:    "", // Will be set when writing
			MtimeMs: time.Now().UnixMilli(),
			Content: content,
		})
	}

	return result, nil
}

// formatMemoryMarkdown creates a properly formatted memory file.
func formatMemoryMarkdown(name, description string, memType memory.MemoryType, content string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", name))
	sb.WriteString(fmt.Sprintf("description: %s\n", description))
	sb.WriteString(fmt.Sprintf("type: %s\n", memType))
	sb.WriteString("---\n\n")
	sb.WriteString(content)
	return sb.String()
}

// WriteCompactedMemories writes the compacted memories to files.
func WriteCompactedMemories(memoryDir string, compacted []memory.RelevantMemory) error {
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		return err
	}

	for i, m := range compacted {
		if m.Path == "" {
			m.Path = filepath.Join(memoryDir, fmt.Sprintf("compacted_%d.md", i))
		}

		if err := os.WriteFile(m.Path, []byte(m.Content), 0644); err != nil {
			return fmt.Errorf("write compacted memory %s: %w", m.Path, err)
		}
	}

	return nil
}

// RemoveOldFiles removes the original files after compaction.
func RemoveOldFiles(paths []string) error {
	var firstErr error
	for _, p := range paths {
		if err := os.Remove(p); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("remove %s: %w", p, err)
		}
	}
	return firstErr
}

// detectType attempts to determine the memory type from a memory path or content.
func detectType(m memory.RelevantMemory) memory.MemoryType {
	// Try from path
	lower := strings.ToLower(m.Path)
	for _, mt := range memory.ValidMemoryTypes() {
		if strings.Contains(lower, string(mt)+"/") || strings.Contains(lower, string(mt)+"_") {
			return mt
		}
	}

	// Try from content
	for _, mt := range memory.ValidMemoryTypes() {
		if strings.Contains(lower, string(mt)) {
			return mt
		}
	}

	return memory.MemoryTypeProject // fallback
}

// CompactIfNeeded checks and performs compaction if needed.
// This is the main entry point for automatic compaction.
func CompactIfNeeded(ctx context.Context, client *api.Client, memoryDir string, config *CompactionConfig) (*CompactionResult, error) {
	if config == nil {
		config = DefaultCompactionConfig()
	}

	if !ShouldCompact(memoryDir, config) {
		return nil, nil
	}

	result, err := Compact(ctx, client, memoryDir, config)
	if err != nil {
		return nil, err
	}

	// Update the MEMORY.md index
	if err := rebuildIndex(memoryDir); err != nil {
		return result, fmt.Errorf("compaction succeeded but index rebuild failed: %w", err)
	}

	return result, nil
}

// rebuildIndex regenerates the MEMORY.md index file.
func rebuildIndex(memoryDir string) error {
	headers, err := memory.ScanMemoryFiles(memoryDir)
	if err != nil {
		return err
	}

	var indexLines []string
	indexLines = append(indexLines, "# Memory Index")
	indexLines = append(indexLines, "")
	indexLines = append(indexLines, "Each memory is stored in its own file. This file is an index of pointers.")
	indexLines = append(indexLines, "Format: - [title](file.md) — one-line summary")
	indexLines = append(indexLines, "")

	for _, h := range headers {
		name := h.Filename
		if !strings.HasSuffix(name, ".md") {
			name += ".md"
		}
		title := strings.TrimSuffix(filepath.Base(h.Filename), ".md")
		title = strings.ReplaceAll(title, "_", " ")
		title = strings.Title(title)

		desc := h.Description
		if desc == "" {
			desc = "No description"
		}

		indexLines = append(indexLines, fmt.Sprintf("- [%s](%s) — %s", title, h.Filename, desc))
	}

	entryPath := filepath.Join(memoryDir, memory.EntrypointName)
	return os.WriteFile(entryPath, []byte(strings.Join(indexLines, "\n")+"\n"), 0644)
}
