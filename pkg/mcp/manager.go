package mcp

import (
	"context"
	"dogclaw/internal/logger"
	"fmt"
	"sync"

	"dogclaw/pkg/types"
)

// Manager manages MCP servers and tools
type Manager struct {
	config  *Config
	clients map[string]Client
	tools   []types.Tool
	mu      sync.RWMutex
}

// NewManager creates a new MCP manager
func NewManager(config *Config) *Manager {
	return &Manager{
		config:  config,
		clients: make(map[string]Client),
		tools:   []types.Tool{},
	}
}

// Initialize initializes all configured MCP servers
func (m *Manager) Initialize(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, server := range m.config.Servers {
		// Set server name from map key
		server.Name = name

		// Automatically determine server type
		if server.Command != "" {
			server.Type = "stdio"
		} else if server.OAuth.TokenURL != "" || server.OAuth.ClientID != "" || server.OAuth.ClientSecret != "" {
			server.Type = "oauth"
		} else {
			server.Type = "http"
		}

		var client Client

		// Create appropriate client based on server type
		switch server.Type {
		case "http":
			client = NewHTTPClient(server)
		case "oauth":
			client = NewOAuthClient(server)
		case "stdio":
			// Default to mock client for stdio (not implemented yet)
			client = &mockClient{
				server: server,
			}
		default:
			return fmt.Errorf("unknown MCP server type: %s", server.Type)
		}

		if err := client.Connect(ctx); err != nil {
			logger.GetGlobalLogger().Errorf("failed to connect to MCP server %s: %w", server.Name, err)
			continue
		}

		m.clients[server.Name] = client

		// Load tools from the server
		tools, err := client.ListTools(ctx)
		if err != nil {
			logger.GetGlobalLogger().Errorf("failed to list tools from MCP server %s: %w", server.Name, err)
			continue
		}

		for _, tool := range tools {
			adapter := NewMCPToolAdapter(tool, client)
			m.tools = append(m.tools, adapter)
		}
	}

	return nil
}

// GetTools returns all MCP tools adapted to the types.Tool interface
func (m *Manager) GetTools() []types.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return append([]types.Tool{}, m.tools...)
}

// Shutdown disconnects all MCP servers
func (m *Manager) Shutdown() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for name, client := range m.clients {
		if err := client.Disconnect(); err != nil {
			errs = append(errs, fmt.Errorf("failed to disconnect from %s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errs)
	}

	return nil
}

// mockClient is a placeholder MCP client for demonstration
type mockClient struct {
	server MCPServer
}

func (m *mockClient) Connect(ctx context.Context) error {
	// In a real implementation, this would connect to the actual MCP server
	return nil
}

func (m *mockClient) Disconnect() error {
	// In a real implementation, this would disconnect from the MCP server
	return nil
}

func (m *mockClient) ListTools(ctx context.Context) ([]MCPTool, error) {
	// In a real implementation, this would list tools from the actual MCP server
	return []MCPTool{}, nil
}

func (m *mockClient) CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (*MCPToolCallResult, error) {
	// In a real implementation, this would call the tool on the actual MCP server
	return nil, fmt.Errorf("not implemented")
}

func (m *mockClient) ServerName() string {
	return m.server.Name
}
