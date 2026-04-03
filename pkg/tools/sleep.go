package tools

import (
	"context"
	"fmt"

	"dogclaw/pkg/types"
)

// SleepTool implements the sleep tool for proactive mode
type SleepTool struct{}

func NewSleepTool() *SleepTool {
	return &SleepTool{}
}

func (t *SleepTool) Name() string      { return "Sleep" }
func (t *SleepTool) Aliases() []string { return []string{"wait", "pause"} }

func (t *SleepTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"reason": map[string]any{
				"type":        "string",
				"description": "Reason for waiting",
			},
		},
		Required: []string{"reason"},
	}
}

func (t *SleepTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Wait for user to provide more input. Use when you need the user to take action first."
}

func (t *SleepTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	reason, ok := input["reason"].(string)
	if !ok {
		reason = "Waiting for user input"
	}

	return &types.ToolResult{
		Data:    fmt.Sprintf("Waiting: %s", reason),
		IsError: false,
	}, nil
}

func (t *SleepTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *SleepTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *SleepTool) IsDestructive(input map[string]any) bool     { return false }
func (t *SleepTool) IsEnabled() bool                             { return true }
func (t *SleepTool) SearchHint() string                          { return "wait pause stop sleep" }
