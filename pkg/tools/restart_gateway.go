package tools

import (
	"context"
	"os"

	"dogclaw/pkg/types"
)

// RestartGatewayTool 用于重启 gateway
type RestartGatewayTool struct{}

// NewRestartGatewayTool 创建新的 RestartGatewayTool
func NewRestartGatewayTool() *RestartGatewayTool {
	return &RestartGatewayTool{}
}

func (t *RestartGatewayTool) Name() string { return "RestartGateway" }
func (t *RestartGatewayTool) Aliases() []string {
	return []string{"restart", "reboot", "restart-gateway"}
}

func (t *RestartGatewayTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type:       "object",
		Properties: map[string]any{},
	}
}

func (t *RestartGatewayTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Restart the gateway process by exiting, allowing the daemon script to restart it."
}

func (t *RestartGatewayTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	// 首先发送进度提示
	if onProgress != nil {
		onProgress(types.ToolProgress{
			Data: "Initiating gateway restart...",
		})
	}

	// 发送重启信号（跨平台实现）
	err := signalRestartProcess()
	if err != nil {
		// 如果信号发送失败，尝试直接退出
		go func() {
			os.Exit(0)
		}()
	}

	return &types.ToolResult{
		Data:    "Gateway restart initiated. The process will restart shortly.",
		IsError: false,
	}, nil
}

func (t *RestartGatewayTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *RestartGatewayTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *RestartGatewayTool) IsDestructive(input map[string]any) bool     { return false }
func (t *RestartGatewayTool) IsEnabled() bool                             { return false }
func (t *RestartGatewayTool) SearchHint() string                          { return "restart reboot gateway" }
