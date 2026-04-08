package tools

import (
	"context"
	"fmt"

	"dogclaw/pkg/channel"
	"dogclaw/pkg/types"
)

// NotifyChannelTool allows LLM to send messages to various channels
type NotifyChannelTool struct {
	registry *channel.Registry
}

func NewNotifyChannelTool(registry *channel.Registry) *NotifyChannelTool {
	return &NotifyChannelTool{
		registry: registry,
	}
}

func (t *NotifyChannelTool) Name() string { return "notify_channel" }
func (t *NotifyChannelTool) Aliases() []string {
	return []string{"send_channel_message", "push_notification"}
}

func (t *NotifyChannelTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"channel": map[string]any{
				"type":        "string",
				"description": "Target channel name: 'qq', 'weixin', or 'cli'",
				"enum":        []string{"qq", "weixin", "cli"},
			},
			"chat_id": map[string]any{
				"type":        "string",
				"description": "Optional target chat/user ID. If empty, sends to all active sessions in that channel.",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "The message content to send",
			},
		},
		Required: []string{"channel", "message"},
	}
}

func (t *NotifyChannelTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Send a message to a specific channel (QQ, WeChat, or CLI). Use this to notify users or groups, especially during background tasks."
}

func (t *NotifyChannelTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	if t.registry == nil {
		return &types.ToolResult{
			Data:    "Error: Channel registry not initialized",
			IsError: true,
		}, nil
	}

	channelName, _ := input["channel"].(string)
	chatID, _ := input["chat_id"].(string)
	message, _ := input["message"].(string)

	if message == "" {
		return &types.ToolResult{
			Data:    "Error: 'message' parameter is required",
			IsError: true,
		}, nil
	}

	err := t.registry.Send(ctx, channelName, chatID, message)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Failed to send message: %v", err),
			IsError: true,
		}, nil
	}

	target := chatID
	if target == "" {
		target = "all active sessions"
	}
	return &types.ToolResult{
		Data:    fmt.Sprintf("Successfully sent message to %s (%s)", channelName, target),
		IsError: false,
	}, nil
}

func (t *NotifyChannelTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *NotifyChannelTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *NotifyChannelTool) IsDestructive(input map[string]any) bool     { return false }
func (t *NotifyChannelTool) IsEnabled() bool                             { return true }
func (t *NotifyChannelTool) SearchHint() string                          { return "notify channel qq wechat send message" }
