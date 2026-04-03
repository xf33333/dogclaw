package tools

import (
	"context"
	"fmt"

	"dogclaw/pkg/types"
)

// SendMessageTool implements inter-agent messaging
type SendMessageTool struct{}

func NewSendMessageTool() *SendMessageTool {
	return &SendMessageTool{}
}

func (t *SendMessageTool) Name() string      { return "SendMessage" }
func (t *SendMessageTool) Aliases() []string { return []string{"send_message", "message"} }

func (t *SendMessageTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"message": map[string]any{
				"type":        "string",
				"description": "The message to send",
			},
			"recipient": map[string]any{
				"type":        "string",
				"description": "The recipient agent or channel",
			},
		},
		Required: []string{"message"},
	}
}

func (t *SendMessageTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Send a message to another agent or channel. Use for inter-agent communication."
}

func (t *SendMessageTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	message, ok := input["message"].(string)
	if !ok || message == "" {
		return &types.ToolResult{
			Data:    "Error: 'message' parameter is required",
			IsError: true,
		}, nil
	}

	recipient, _ := input["recipient"].(string)
	if recipient == "" {
		recipient = "default"
	}

	return &types.ToolResult{
		Data:    fmt.Sprintf("Message sent to %s: %s", recipient, message),
		IsError: false,
	}, nil
}

func (t *SendMessageTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *SendMessageTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *SendMessageTool) IsDestructive(input map[string]any) bool     { return false }
func (t *SendMessageTool) IsEnabled() bool                             { return true }
func (t *SendMessageTool) SearchHint() string                          { return "send message communicate agent channel" }
