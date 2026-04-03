package tools

import (
	"context"
	"fmt"

	"dogclaw/pkg/types"
)

// TodoWriteTool implements todo list management
type TodoWriteTool struct{}

func NewTodoWriteTool() *TodoWriteTool {
	return &TodoWriteTool{}
}

func (t *TodoWriteTool) Name() string      { return "TodoWrite" }
func (t *TodoWriteTool) Aliases() []string { return []string{"todo", "todos"} }

func (t *TodoWriteTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"todos": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content": map[string]any{
							"type":        "string",
							"description": "The todo item content",
						},
						"status": map[string]any{
							"type":        "string",
							"enum":        []string{"pending", "in_progress", "completed"},
							"description": "The status of the todo item",
						},
					},
					"required": []string{"content", "status"},
				},
				"description": "The updated todo list (all items, not just changes)",
			},
		},
		Required: []string{"todos"},
	}
}

func (t *TodoWriteTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Create and manage a todo list for tracking task progress. " +
		"Always provide the complete list of all todos with their current status."
}

func (t *TodoWriteTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	todos, ok := input["todos"]
	if !ok {
		return &types.ToolResult{
			Data:    "Error: 'todos' parameter is required",
			IsError: true,
		}, nil
	}

	return &types.ToolResult{
		Data:    fmt.Sprintf("Todo list updated successfully with %d items", len(todos.([]any))),
		IsError: false,
	}, nil
}

func (t *TodoWriteTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *TodoWriteTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *TodoWriteTool) IsDestructive(input map[string]any) bool     { return false }
func (t *TodoWriteTool) IsEnabled() bool                             { return true }
func (t *TodoWriteTool) SearchHint() string                          { return "todo list tasks manage track" }
