package tools

import (
	"context"
	"fmt"

	"dogclaw/pkg/types"
)

// TaskCreateTool implements task creation for tracking work items
type TaskCreateTool struct{}

func NewTaskCreateTool() *TaskCreateTool {
	return &TaskCreateTool{}
}

func (t *TaskCreateTool) Name() string      { return "TaskCreate" }
func (t *TaskCreateTool) Aliases() []string { return []string{"create_task", "new_task"} }

func (t *TaskCreateTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "Task title",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Task description",
			},
			"status": map[string]any{
				"type":        "string",
				"enum":        []string{"pending", "in_progress", "completed", "cancelled"},
				"description": "Task status",
			},
		},
		Required: []string{"title"},
	}
}

func (t *TaskCreateTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Create a new task for tracking work items. Tasks can be updated later with TaskUpdate."
}

func (t *TaskCreateTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	title, ok := input["title"].(string)
	if !ok || title == "" {
		return &types.ToolResult{
			Data:    "Error: 'title' parameter is required",
			IsError: true,
		}, nil
	}

	description, _ := input["description"].(string)
	status, _ := input["status"].(string)
	if status == "" {
		status = "pending"
	}

	return &types.ToolResult{
		Data:    fmt.Sprintf("Task created: %s\nDescription: %s\nStatus: %s", title, description, status),
		IsError: false,
	}, nil
}

func (t *TaskCreateTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *TaskCreateTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *TaskCreateTool) IsDestructive(input map[string]any) bool     { return false }
func (t *TaskCreateTool) IsEnabled() bool                             { return true }
func (t *TaskCreateTool) SearchHint() string                          { return "task create new work item track" }

// TaskUpdateTool implements task status updates
type TaskUpdateTool struct{}

func NewTaskUpdateTool() *TaskUpdateTool {
	return &TaskUpdateTool{}
}

func (t *TaskUpdateTool) Name() string      { return "TaskUpdate" }
func (t *TaskUpdateTool) Aliases() []string { return []string{"update_task", "modify_task"} }

func (t *TaskUpdateTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "Task identifier",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "Updated task title",
			},
			"status": map[string]any{
				"type":        "string",
				"enum":        []string{"pending", "in_progress", "completed", "cancelled"},
				"description": "Updated task status",
			},
		},
		Required: []string{"task_id"},
	}
}

func (t *TaskUpdateTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Update an existing task's status or details. Use to track progress on work items."
}

func (t *TaskUpdateTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	taskID, ok := input["task_id"].(string)
	if !ok || taskID == "" {
		return &types.ToolResult{
			Data:    "Error: 'task_id' parameter is required",
			IsError: true,
		}, nil
	}

	title, _ := input["title"].(string)
	status, _ := input["status"].(string)

	return &types.ToolResult{
		Data:    fmt.Sprintf("Task %s updated: title=%s, status=%s", taskID, title, status),
		IsError: false,
	}, nil
}

func (t *TaskUpdateTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *TaskUpdateTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *TaskUpdateTool) IsDestructive(input map[string]any) bool     { return false }
func (t *TaskUpdateTool) IsEnabled() bool                             { return true }
func (t *TaskUpdateTool) SearchHint() string                          { return "task update modify status change" }

// EnterPlanModeTool implements entering plan mode
type EnterPlanModeTool struct{}

func NewEnterPlanModeTool() *EnterPlanModeTool {
	return &EnterPlanModeTool{}
}

func (t *EnterPlanModeTool) Name() string      { return "EnterPlanMode" }
func (t *EnterPlanModeTool) Aliases() []string { return []string{"plan_mode", "enter_plan"} }

func (t *EnterPlanModeTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
	}
}

func (t *EnterPlanModeTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Enter plan mode to discuss approach before implementing. No file edits will be made in plan mode."
}

func (t *EnterPlanModeTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	return &types.ToolResult{
		Data:    "Entered plan mode. I'll discuss the approach before making any changes.",
		IsError: false,
	}, nil
}

func (t *EnterPlanModeTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *EnterPlanModeTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *EnterPlanModeTool) IsDestructive(input map[string]any) bool     { return false }
func (t *EnterPlanModeTool) IsEnabled() bool                             { return true }
func (t *EnterPlanModeTool) SearchHint() string                          { return "plan mode discuss approach before implement" }

// ExitPlanModeTool implements exiting plan mode
type ExitPlanModeTool struct{}

func NewExitPlanModeTool() *ExitPlanModeTool {
	return &ExitPlanModeTool{}
}

func (t *ExitPlanModeTool) Name() string      { return "ExitPlanMode" }
func (t *ExitPlanModeTool) Aliases() []string { return []string{"exit_plan", "leave_plan"} }

func (t *ExitPlanModeTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
	}
}

func (t *ExitPlanModeTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Exit plan mode and return to normal mode where file edits are allowed."
}

func (t *ExitPlanModeTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	return &types.ToolResult{
		Data:    "Exited plan mode. Ready to implement changes.",
		IsError: false,
	}, nil
}

func (t *ExitPlanModeTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *ExitPlanModeTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *ExitPlanModeTool) IsDestructive(input map[string]any) bool     { return false }
func (t *ExitPlanModeTool) IsEnabled() bool                             { return true }
func (t *ExitPlanModeTool) SearchHint() string                          { return "exit plan mode normal implement" }

// ExitTool implements the exit command
type ExitTool struct{}

func NewExitTool() *ExitTool {
	return &ExitTool{}
}

func (t *ExitTool) Name() string      { return "Exit" }
func (t *ExitTool) Aliases() []string { return []string{"quit", "terminate"} }

func (t *ExitTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
	}
}

func (t *ExitTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Exit the current session. Use when the conversation is complete."
}

func (t *ExitTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	return &types.ToolResult{
		Data:    "Exiting session.",
		IsError: false,
	}, nil
}

func (t *ExitTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *ExitTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *ExitTool) IsDestructive(input map[string]any) bool     { return false }
func (t *ExitTool) IsEnabled() bool                             { return true }
func (t *ExitTool) SearchHint() string                          { return "exit quit terminate end session" }
