package mcp

import "context"

// MCPServer represents a configured MCP server
type MCPServer struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"` // "stdio", "http", "oauth"
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	OAuth   OAuthConfig       `json:"oauth,omitempty"`
}

// OAuthConfig represents OAuth configuration for MCP servers
type OAuthConfig struct {
	TokenURL     string `json:"tokenUrl"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	Scope        string `json:"scope,omitempty"`
	Token        string `json:"token,omitempty"`
}

// MCPTool represents a tool exposed by an MCP server
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	ServerName  string                 `json:"serverName"`
}

// MCPToolCallResult represents the result of calling an MCP tool
type MCPToolCallResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// MCPContent represents a piece of content from an MCP tool call
type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Client defines the interface for MCP clients
type Client interface {
	// Connect connects to the MCP server
	Connect(ctx context.Context) error

	// Disconnect disconnects from the MCP server
	Disconnect() error

	// ListTools lists all tools available from the server
	ListTools(ctx context.Context) ([]MCPTool, error)

	// CallTool calls a tool on the server
	CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (*MCPToolCallResult, error)

	// ServerName returns the name of the server this client is connected to
	ServerName() string
}
