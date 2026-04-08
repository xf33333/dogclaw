package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const AgentMdFileName = "AGENT.md"

const DefaultAgentPrompt = `You are DogClaw, a helpful AI coding assistant implemented in Go. You can help with software engineering tasks including writing code, debugging, file manipulation, and web research.
## Guidelines

- Use tools when needed to accomplish tasks
- Be concise and accurate
- Show code and command output when relevant
- Think step by step before acting
`

// EnsureAgentMarkdownExists checks if ~/.dogclaw/AGENT.md exists, creates if not.
func EnsureAgentMarkdownExists() error {
	dir, err := GetSettingsDir()
	if err != nil {
		return err
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	path := filepath.Join(dir, AgentMdFileName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// File doesn't exist, create it with default content
		if err := os.WriteFile(path, []byte(DefaultAgentPrompt), 0644); err != nil {
			return fmt.Errorf("failed to create default AGENT.md: %w", err)
		}
	}

	return nil
}

// GetAgentMarkdown reads the content of AGENT.md.
// Priority: ./AGENT.md > ~/.dogclaw/AGENT.md
func GetAgentMarkdown() string {
	// Try current directory first
	if data, err := os.ReadFile(AgentMdFileName); err == nil {
		return string(data)
	}

	// Try global directory
	dir, err := GetSettingsDir()
	if err != nil {
		return DefaultAgentPrompt
	}

	path := filepath.Join(dir, AgentMdFileName)
	if data, err := os.ReadFile(path); err == nil {
		return string(data)
	}

	return DefaultAgentPrompt
}
