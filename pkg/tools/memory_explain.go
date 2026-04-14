package tools

import (
	"context"
	"dogclaw/pkg/memory"
	"dogclaw/pkg/types"
	"os"
)

type MemoryExplainTool struct{}

func NewMemoryExplainTool() *MemoryExplainTool {
	return &MemoryExplainTool{}
}

func (t *MemoryExplainTool) Name() string      { return "MemoryExplain" }
func (t *MemoryExplainTool) Aliases() []string { return []string{"memory_explain", "explain_memory"} }

func (t *MemoryExplainTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
	}
}

func (t *MemoryExplainTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Get the memory system documentation and usage instructions. " +
		"Returns the complete memory system prompt explaining how to use the file-based memory system, " +
		"including memory types (user, feedback, project, reference), how to save memories, " +
		"and best practices for memory management."
}

func (t *MemoryExplainTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	cwd, _ := os.Getwd()
	prompt := memory.BuildMemoryPrompt(memory.PromptConfig{
		DisplayName: "Auto Memory",
		MemoryDir:   memory.GetAutoMemPath(cwd),
	})

	return &types.ToolResult{
		Data:    prompt,
		IsError: false,
	}, nil
}

func (t *MemoryExplainTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *MemoryExplainTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *MemoryExplainTool) IsDestructive(input map[string]any) bool     { return false }
func (t *MemoryExplainTool) IsEnabled() bool                             { return true }
func (t *MemoryExplainTool) SearchHint() string {
	return "memory system documentation explain usage instructions"
}
