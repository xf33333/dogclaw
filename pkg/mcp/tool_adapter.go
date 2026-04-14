package mcp

import (
	"context"
	"fmt"

	"dogclaw/pkg/types"
)

// MCPToolAdapter adapts an MCP tool to the types.Tool interface
type MCPToolAdapter struct {
	tool   MCPTool
	client Client
}

// NewMCPToolAdapter creates a new MCP tool adapter
func NewMCPToolAdapter(tool MCPTool, client Client) *MCPToolAdapter {
	return &MCPToolAdapter{
		tool:   tool,
		client: client,
	}
}

// Name returns the tool's name
func (a *MCPToolAdapter) Name() string {
	return fmt.Sprintf("mcp_%s_%s", a.tool.ServerName, a.tool.Name)
}

// Aliases returns alternative names for the tool
func (a *MCPToolAdapter) Aliases() []string {
	return []string{
		fmt.Sprintf("%s_%s", a.tool.ServerName, a.tool.Name),
		a.tool.Name,
	}
}

// InputSchema returns the JSON schema for tool input
func (a *MCPToolAdapter) InputSchema() types.ToolInputSchema {
	schema := types.ToolInputSchema{
		Type:       "object",
		Properties: make(map[string]any),
		Required:   []string{},
	}

	if a.tool.InputSchema != nil {
		if props, ok := a.tool.InputSchema["properties"].(map[string]interface{}); ok {
			for k, v := range props {
				schema.Properties[k] = v
			}
		}
		if required, ok := a.tool.InputSchema["required"].([]interface{}); ok {
			for _, r := range required {
				if str, ok := r.(string); ok {
					schema.Required = append(schema.Required, str)
				}
			}
		}
	}

	return schema
}

// Description returns a description of what the tool does
func (a *MCPToolAdapter) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	desc := fmt.Sprintf("[MCP/%s] %s", a.tool.ServerName, a.tool.Description)
	if len(desc) > 512 {
		desc = desc[:512] + "… [truncated]"
	}
	return desc
}

// Call executes the tool with the given input
func (a *MCPToolAdapter) Call(ctx context.Context, input map[string]any, useCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	result, err := a.client.CallTool(ctx, a.tool.Name, input)
	if err != nil {
		return &types.ToolResult{
			Error:   err.Error(),
			IsError: true,
		}, nil
	}

	var textContent string
	for _, content := range result.Content {
		if content.Type == "text" {
			textContent += content.Text + "\n"
		}
	}

	return &types.ToolResult{
		Data:    textContent,
		IsError: result.IsError,
	}, nil
}

// IsConcurrencySafe returns true if the tool can run concurrently
func (a *MCPToolAdapter) IsConcurrencySafe(input map[string]any) bool {
	return true
}

// IsReadOnly returns true if the tool only reads data
func (a *MCPToolAdapter) IsReadOnly(input map[string]any) bool {
	return false // Default to false since we don't know
}

// IsDestructive returns true if the tool performs irreversible operations
func (a *MCPToolAdapter) IsDestructive(input map[string]any) bool {
	return false // Default to false since we don't know
}

// IsEnabled returns true if the tool is currently enabled
func (a *MCPToolAdapter) IsEnabled() bool {
	return true
}

// SearchHint returns a one-line capability phrase for tool search
func (a *MCPToolAdapter) SearchHint() string {
	return fmt.Sprintf("MCP tool from %s server: %s", a.tool.ServerName, a.tool.Name)
}
