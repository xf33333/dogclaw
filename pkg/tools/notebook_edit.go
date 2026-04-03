package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"dogclaw/pkg/types"
)

// NotebookEditTool implements Jupyter notebook editing
type NotebookEditTool struct{}

func NewNotebookEditTool() *NotebookEditTool {
	return &NotebookEditTool{}
}

func (t *NotebookEditTool) Name() string      { return "NotebookEdit" }
func (t *NotebookEditTool) Aliases() []string { return []string{"notebook", "jupyter"} }

func (t *NotebookEditTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"notebook_path": map[string]any{
				"type":        "string",
				"description": "Path to the Jupyter notebook file (.ipynb)",
			},
			"cell_id": map[string]any{
				"type":        "string",
				"description": "ID of the cell to edit",
			},
			"new_source": map[string]any{
				"type":        "string",
				"description": "New source code for the cell",
			},
		},
		Required: []string{"notebook_path", "cell_id", "new_source"},
	}
}

func (t *NotebookEditTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Edit a cell in a Jupyter notebook (.ipynb file). " +
		"Modify the source code of a specific cell identified by its cell_id."
}

func (t *NotebookEditTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	notebookPath, ok := input["notebook_path"].(string)
	if !ok || notebookPath == "" {
		return &types.ToolResult{
			Data:    "Error: 'notebook_path' parameter is required",
			IsError: true,
		}, nil
	}

	cellID, ok := input["cell_id"].(string)
	if !ok || cellID == "" {
		return &types.ToolResult{
			Data:    "Error: 'cell_id' parameter is required",
			IsError: true,
		}, nil
	}

	_, ok = input["new_source"].(string)
	if !ok {
		return &types.ToolResult{
			Data:    "Error: 'new_source' parameter is required",
			IsError: true,
		}, nil
	}

	if !filepath.IsAbs(notebookPath) {
		notebookPath = filepath.Join(toolCtx.Cwd, notebookPath)
	}
	notebookPath = filepath.Clean(notebookPath)

	info, err := os.Stat(notebookPath)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Cannot access notebook '%s': %v", notebookPath, err),
			IsError: true,
		}, nil
	}

	if info.IsDir() {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: '%s' is a directory", notebookPath),
			IsError: true,
		}, nil
	}

	// In a full implementation, we would parse the JSON, find the cell, and update it
	// For now, we'll just report success with a note
	return &types.ToolResult{
		Data:    fmt.Sprintf("Notebook edit requested for cell %s in %s (full JSON parsing not yet implemented)", cellID, notebookPath),
		IsError: false,
	}, nil
}

func (t *NotebookEditTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *NotebookEditTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *NotebookEditTool) IsDestructive(input map[string]any) bool     { return false }
func (t *NotebookEditTool) IsEnabled() bool                             { return true }
func (t *NotebookEditTool) SearchHint() string                          { return "jupyter notebook cell edit ipynb" }
