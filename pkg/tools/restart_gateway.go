package tools

import (
	"context"
	"os"
	"syscall"

	"dogclaw/pkg/types"
)

// RestartGatewayTool 用于重启 gateway
type RestartGatewayTool struct{}

// NewRestartGatewayTool 创建新的 RestartGatewayTool
func NewRestartGatewayTool() *RestartGatewayTool {
	return &RestartGatewayTool{}
}

func (t *RestartGatewayTool) Name() string      { return "RestartGateway" }
func (t *RestartGatewayTool) Aliases() []string { return []string{"restart", "reboot", "restart-gateway"} }

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

	// 向自己发送 SIGUSR2 信号让 daemon 重启
	pid := os.Getpid()
	process, err := os.FindProcess(pid)
	if err != nil {
		return &types.ToolResult{
			Data:    "Failed to find current process",
			IsError: true,
		}, nil
	}

	// 使用 SIGUSR2 来触发重启（之前约定的信号）
	err = process.Signal(syscall.SIGUSR2)
	if err != nil {
		// 如果 SIGUSR2 失败，尝试直接退出
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
func (t *RestartGatewayTool) IsEnabled() bool                             { return true }
func (t *RestartGatewayTool) SearchHint() string                          { return "restart reboot gateway" }
