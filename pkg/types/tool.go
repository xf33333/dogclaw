package types

import "context"

// ToolInputSchema represents the JSON schema for tool input
type ToolInputSchema struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	Required   []string       `json:"required,omitempty"`
}

// ToolResult is the result of a tool execution
type ToolResult struct {
	Data        any       `json:"data"`
	NewMessages []Message `json:"newMessages,omitempty"`
	Error       string    `json:"error,omitempty"`
	IsError     bool      `json:"isError"`
}

// ToolCallProgress is a callback for reporting tool progress
type ToolCallProgress func(progress ToolProgress)

// ToolProgress represents progress information
type ToolProgress struct {
	ToolUseID string `json:"toolUseID"`
	Data      any    `json:"data"`
}

// Tool defines the interface all tools must implement
type Tool interface {
	// Name returns the tool's primary name
	Name() string

	// Aliases returns alternative names for the tool
	Aliases() []string

	// InputSchema returns the JSON schema for tool input
	InputSchema() ToolInputSchema

	// Description returns a description of what the tool does
	Description(input map[string]any, opts ToolDescriptionOptions) string

	// Call executes the tool with the given input
	Call(ctx context.Context, input map[string]any, useCtx ToolUseContext, onProgress ToolCallProgress) (*ToolResult, error)

	// IsConcurrencySafe returns true if the tool can run concurrently
	IsConcurrencySafe(input map[string]any) bool

	// IsReadOnly returns true if the tool only reads data
	IsReadOnly(input map[string]any) bool

	// IsDestructive returns true if the tool performs irreversible operations
	IsDestructive(input map[string]any) bool

	// IsEnabled returns true if the tool is currently enabled
	IsEnabled() bool

	// SearchHint returns a one-line capability phrase for tool search
	SearchHint() string
}

// ToolDescriptionOptions contains options for generating tool descriptions
type ToolDescriptionOptions struct {
	IsNonInteractiveSession bool
	Tools                   []Tool
}

// ToolUseContext provides context for tool execution
type ToolUseContext struct {
	Cwd                     string
	AbortController         context.Context
	Tools                   []Tool
	IsNonInteractiveSession bool
}

// Tools is a collection of tools
type Tools []Tool

// FindToolByName finds a tool by name or alias
func (tools Tools) FindToolByName(name string) Tool {
	for _, tool := range tools {
		if tool.Name() == name {
			return tool
		}
		for _, alias := range tool.Aliases() {
			if alias == name {
				return tool
			}
		}
	}
	return nil
}
