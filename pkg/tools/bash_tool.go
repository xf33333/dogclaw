package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"dogclaw/pkg/types"
)

// BashTool implements the Bash tool for command execution
type BashTool struct{}

func NewBashTool() *BashTool {
	return &BashTool{}
}

func (t *BashTool) Name() string {
	return "Bash"
}

func (t *BashTool) Aliases() []string {
	return []string{}
}

func (t *BashTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The bash command to execute",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Optional timeout in milliseconds (max 600000)",
			},
		},
		Required: []string{"command"},
	}
}

func (t *BashTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Execute a bash command in the current working directory. " +
		"Use this to run shell commands, scripts, and programs. " +
		"Commands run in a non-interactive shell with a timeout."
}

func (t *BashTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	command, ok := input["command"].(string)
	if !ok || command == "" {
		return &types.ToolResult{
			Data:    "Error: 'command' parameter is required",
			IsError: true,
		}, nil
	}

	// Execute command via shell
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = toolCtx.Cwd

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &types.ToolResult{
			Data: fmt.Sprintf("**Bash**\n\ncommand: `%s`\n\noutput:\n%s\n\nerror: %v", command, strings.TrimSpace(string(output)), err),
			IsError: true,
		}, nil
	}

	return &types.ToolResult{
		Data: fmt.Sprintf("**Bash**\n\ncommand: `%s`\n\noutput:\n%s", command, strings.TrimSpace(string(output))),
		IsError: false,
	}, nil
}

func (t *BashTool) IsConcurrencySafe(input map[string]any) bool {
	return false
}

func (t *BashTool) IsReadOnly(input map[string]any) bool {
	command, _ := input["command"].(string)
	// Simple heuristic: read-only commands typically don't modify files
	readOnlyPrefixes := []string{"cat ", "ls ", "grep ", "find ", "head ", "tail ", "wc ", "echo "}
	for _, prefix := range readOnlyPrefixes {
		if strings.HasPrefix(command, prefix) {
			return true
		}
	}
	return false
}

func (t *BashTool) IsDestructive(input map[string]any) bool {
	command, _ := input["command"].(string)
	destructivePrefixes := []string{"rm ", "del ", "rmdir ", ">"}
	for _, prefix := range destructivePrefixes {
		if strings.HasPrefix(command, prefix) {
			return true
		}
	}
	return false
}

func (t *BashTool) IsEnabled() bool {
	return true
}

func (t *BashTool) SearchHint() string {
	return "execute shell commands run scripts programs"
}
