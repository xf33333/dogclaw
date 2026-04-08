package cron

import (
	"context"
	"fmt"
	"strconv"

	"dogclaw/pkg/types"
)

// CronTool allows LLM to manage scheduled tasks
type CronTool struct{}

func NewCronTool() *CronTool {
	return &CronTool{}
}

func (t *CronTool) Name() string { return "Cron" }
func (t *CronTool) Aliases() []string {
	return []string{"schedule_task", "manage_cron", "cron_jobs"}
}

func (t *CronTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Action to perform: 'list', 'add', 'delete', 'update'",
				"enum":        []string{"list", "add", "delete", "update"},
			},
			"schedule": map[string]any{
				"type":        "string",
				"description": "Cron expression (e.g., '*/5 * * * *') for 'add' or 'update' actions",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Natural language description of the task for 'add' or 'update' actions,具体的定时任务工作内容，不包含给自己定时",
			},
			"index": map[string]any{
				"type":        "integer",
				"description": "Index of the cron job to delete or update (0-based)",
			},
		},
		Required: []string{"action"},
	}
}

func (t *CronTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Manage scheduled tasks (cron jobs). You can list existing jobs, add new ones with a cron expression and description, " +
		"delete jobs by index, or update existing jobs. Jobs run in background every minute check."
}

func (t *CronTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	action, _ := input["action"].(string)

	config, err := LoadConfig()
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error loading cron config: %v", err),
			IsError: true,
		}, nil
	}

	switch action {
	case "list":
		if len(config.Tasks) == 0 {
			return &types.ToolResult{Data: "No scheduled tasks found.", IsError: false}, nil
		}
		res := "Scheduled tasks:\n"
		for i, job := range config.Tasks {
			res += fmt.Sprintf("[%d] Schedule: %s, Description: %s\n", i, job.Schedule, job.Description)
		}
		return &types.ToolResult{Data: res, IsError: false}, nil

	case "add":
		schedule, _ := input["schedule"].(string)
		description, _ := input["description"].(string)
		if schedule == "" || description == "" {
			return &types.ToolResult{
				Data:    "Error: 'schedule' and 'description' are required for 'add' action",
				IsError: true,
			}, nil
		}
		// Basic validation of schedule could be added here later
		config.Tasks = append(config.Tasks, CronJob{
			Schedule:    schedule,
			Description: description,
		})
		if err := SaveConfig(config); err != nil {
			return &types.ToolResult{
				Data:    fmt.Sprintf("Error saving cron config: %v", err),
				IsError: true,
			}, nil
		}
		return &types.ToolResult{
			Data:    fmt.Sprintf("Successfully added task: %s (%s)", description, schedule),
			IsError: false,
		}, nil

	case "delete":
		idxVal, ok := input["index"]
		if !ok {
			return &types.ToolResult{
				Data:    "Error: 'index' is required for 'delete' action",
				IsError: true,
			}, nil
		}
		var idx int
		switch v := idxVal.(type) {
		case int:
			idx = v
		case float64:
			idx = int(v)
		case string:
			idx, _ = strconv.Atoi(v)
		}

		if idx < 0 || idx >= len(config.Tasks) {
			return &types.ToolResult{
				Data:    fmt.Sprintf("Error: Invalid index %d (total tasks: %d)", idx, len(config.Tasks)),
				IsError: true,
			}, nil
		}
		job := config.Tasks[idx]
		config.Tasks = append(config.Tasks[:idx], config.Tasks[idx+1:]...)
		if err := SaveConfig(config); err != nil {
			return &types.ToolResult{
				Data:    fmt.Sprintf("Error saving cron config: %v", err),
				IsError: true,
			}, nil
		}
		return &types.ToolResult{
			Data:    fmt.Sprintf("Successfully deleted task: %s (%s)", job.Description, job.Schedule),
			IsError: false,
		}, nil

	case "update":
		idxVal, ok := input["index"]
		if !ok {
			return &types.ToolResult{
				Data:    "Error: 'index' is required for 'update' action",
				IsError: true,
			}, nil
		}
		var idx int
		switch v := idxVal.(type) {
		case int:
			idx = v
		case float64:
			idx = int(v)
		case string:
			idx, _ = strconv.Atoi(v)
		}

		if idx < 0 || idx >= len(config.Tasks) {
			return &types.ToolResult{
				Data:    fmt.Sprintf("Error: Invalid index %d", idx),
				IsError: true,
			}, nil
		}

		if schedule, ok := input["schedule"].(string); ok && schedule != "" {
			config.Tasks[idx].Schedule = schedule
		}
		if description, ok := input["description"].(string); ok && description != "" {
			config.Tasks[idx].Description = description
		}

		if err := SaveConfig(config); err != nil {
			return &types.ToolResult{
				Data:    fmt.Sprintf("Error saving cron config: %v", err),
				IsError: true,
			}, nil
		}
		return &types.ToolResult{
			Data:    fmt.Sprintf("Successfully updated task at index %d", idx),
			IsError: false,
		}, nil

	default:
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: Unknown action '%s'", action),
			IsError: true,
		}, nil
	}
}

func (t *CronTool) IsConcurrencySafe(input map[string]any) bool { return false } // Config access not thread-safe here
func (t *CronTool) IsReadOnly(input map[string]any) bool {
	action, _ := input["action"].(string)
	return action == "list"
}
func (t *CronTool) IsDestructive(input map[string]any) bool {
	action, _ := input["action"].(string)
	return action == "delete"
}
func (t *CronTool) IsEnabled() bool    { return true }
func (t *CronTool) SearchHint() string { return "cron schedule background tasks" }
