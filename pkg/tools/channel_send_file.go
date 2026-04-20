package tools

import (
	"context"
	"fmt"
	"log"
	"strings"

	"dogclaw/pkg/channel"
	"dogclaw/pkg/types"
)

type ChannelSendFileTool struct {
	registry *channel.Registry
}

func NewChannelSendFileTool(registry *channel.Registry) *ChannelSendFileTool {
	return &ChannelSendFileTool{
		registry: registry,
	}
}

func (t *ChannelSendFileTool) Name() string { return "channel_send_file" }
func (t *ChannelSendFileTool) Aliases() []string {
	return []string{"send_file", "csf"}
}

func (t *ChannelSendFileTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"channel": map[string]any{
				"type":        "string",
				"description": "Target channel name: 'qq', 'weixin', etc.",
				"enum":        []string{"qq", "weixin"},
			},
			"chat_id": map[string]any{
				"type":        "string",
				"description": "Target chat ID. For private chat, use the user's openid. For group chat, use 'group:group_openid'. If omitted, sends to the current active chat.",
			},
			"file_type": map[string]any{
				"type":        "integer",
				"description": "File type: 1=image, 2=video, 3=voice, 4=file",
				"enum":        []int{1, 2, 3, 4},
			},
			"file_url": map[string]any{
				"type":        "string",
				"description": "HTTP/HTTPS URL or local file path of the file to send. Supports both remote URLs and local files.",
			},
			"file_name": map[string]any{
				"type":        "string",
				"description": "Optional file name to display. Required for file type 4 (file) when using local files.",
			},
		},
		Required: []string{"channel", "file_type", "file_url"},
	}
}

func (t *ChannelSendFileTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	desc := "Send a file to a user or group via a specific channel (QQ, WeChat, etc.). " +
		"Supports both remote HTTP/HTTPS URLs and local file paths. " +
		"File types: 1=image, 2=video, 3=voice, 4=file. " +
		"IMPORTANT: Some channels may have restrictions on file types. " +
		"For local files, the file will be uploaded via base64 encoding."

	if ch, ok := input["channel"].(string); ok && ch != "" {
		activeIDs := t.registry.ActiveChatIDs(ch)
		if len(activeIDs) > 0 {
			desc += fmt.Sprintf(" Current active chat IDs for %s: %s.", ch, strings.Join(activeIDs, ", "))
		}
	} else {
		for _, ch := range []string{"qq", "weixin"} {
			activeIDs := t.registry.ActiveChatIDs(ch)
			if len(activeIDs) > 0 {
				desc += fmt.Sprintf(" Active %s chats: %s.", ch, strings.Join(activeIDs, ", "))
			}
		}
	}

	return desc
}

func (t *ChannelSendFileTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	log.Printf("[ChannelSendFileTool] Call invoked with input: %+v", input)

	if t.registry == nil {
		return &types.ToolResult{
			Data:    "Error: Channel registry not initialized",
			IsError: true,
		}, nil
	}

	channelName, _ := input["channel"].(string)
	chatID, _ := input["chat_id"].(string)
	fileURL, _ := input["file_url"].(string)
	fileName, _ := input["file_name"].(string)

	var fileType int
	switch v := input["file_type"].(type) {
	case int:
		fileType = v
	case float64:
		fileType = int(v)
	case int64:
		fileType = int(v)
	default:
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: 'file_type' must be an integer, got %T", input["file_type"]),
			IsError: true,
		}, nil
	}

	if channelName == "" {
		return &types.ToolResult{
			Data:    "Error: 'channel' parameter is required",
			IsError: true,
		}, nil
	}

	availableChannels := t.registry.GetChannels()
	found := false
	for _, ch := range availableChannels {
		if ch == channelName {
			found = true
			break
		}
	}
	if !found {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: channel '%s' not found. Available channels: %v", channelName, availableChannels),
			IsError: true,
		}, nil
	}

	if chatID == "" {
		activeIDs := t.registry.ActiveChatIDs(channelName)
		if len(activeIDs) == 0 {
			return &types.ToolResult{
				Data:    fmt.Sprintf("Error: no active %s chat session found. Please provide chat_id.", channelName),
				IsError: true,
			}, nil
		}
		if len(activeIDs) == 1 {
			chatID = activeIDs[0]
			log.Printf("[ChannelSendFileTool] Auto-detected chatID: %s", chatID)
		} else {
			return &types.ToolResult{
				Data:    fmt.Sprintf("Error: multiple active %s chats (%s). Please specify chat_id.", channelName, strings.Join(activeIDs, ", ")),
				IsError: true,
			}, nil
		}
	}

	if fileURL == "" {
		return &types.ToolResult{
			Data:    "Error: 'file_url' parameter is required",
			IsError: true,
		}, nil
	}

	if fileType < 1 || fileType > 4 {
		return &types.ToolResult{
			Data:    "Error: 'file_type' must be 1 (image), 2 (video), 3 (voice), or 4 (file)",
			IsError: true,
		}, nil
	}

	fileTypeNames := map[int]string{
		1: "image",
		2: "video",
		3: "voice",
		4: "file",
	}

	log.Printf("[ChannelSendFileTool] Calling registry.SendFile: channel=%s, chatID=%s, fileType=%d, fileURL=%s, fileName=%s", channelName, chatID, fileType, fileURL, fileName)

	err := t.registry.SendFile(ctx, channelName, chatID, fileType, fileURL, fileName)
	if err != nil {
		log.Printf("[ChannelSendFileTool] SendFile failed: %v", err)
		return &types.ToolResult{
			Data:    fmt.Sprintf("Failed to send file: %v", err),
			IsError: true,
		}, nil
	}

	log.Printf("[ChannelSendFileTool] SendFile succeeded")
	return &types.ToolResult{
		Data:    fmt.Sprintf("Successfully sent %s to %s chat %s", fileTypeNames[fileType], channelName, chatID),
		IsError: false,
	}, nil
}

func (t *ChannelSendFileTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *ChannelSendFileTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *ChannelSendFileTool) IsDestructive(input map[string]any) bool     { return false }
func (t *ChannelSendFileTool) IsEnabled() bool                             { return true }
func (t *ChannelSendFileTool) SearchHint() string {
	return "send file image video document qq weixin channel"
}
