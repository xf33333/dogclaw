package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds MCP configuration
type Config struct {
	Servers map[string]MCPServer `json:"servers"`
}

// DefaultConfig returns a default MCP configuration
func DefaultConfig() *Config {
	return &Config{
		Servers: make(map[string]MCPServer),
	}
}

// LoadConfig loads MCP configuration from a file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read MCP config: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse MCP config: %w", err)
	}

	return &config, nil
}

// SaveConfig saves MCP configuration to a file
func (c *Config) SaveConfig(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal MCP config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write MCP config: %w", err)
	}

	return nil
}

// GetConfigPath returns the path to the MCP configuration file
func GetConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".dogclaw", "mcp.json"), nil
}
