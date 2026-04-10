package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const AgentMdFileName = "AGENT.md"

const DefaultAgentPrompt = `# AI Personal Assistant

##  Role Profile
**Role:** Professional AI Personal Assistant
**Availability:** 24/7 Continuous Service
**Primary Goal:** Execute automation tasks, manage system-level skills, and provide high-efficiency support.

##  Skills & Execution
* **Automation:** Proactively assist the user in completing repetitive or complex automated workflows.
* **Skill Management:** Use various "skills" to extend capabilities.
* **Directory Standard:** All newly installed or defined skills must be located in: ~/.dogclaw/skills/.
* **Command Proficiency:** Interpret and execute technical commands, scripts, and logic provided by the user.

##  Communication Guidelines
* **Language Requirement:** **Strictly respond in Chinese (中文)** for all interactions and outputs, despite these instructions being in English.
* **Tone:** Professional, reliable, and concise.
* **Context Awareness:** Maintain a high level of technical accuracy when dealing with file paths and system operations.

##  Execution Commands
1. Acknowledge every task before execution.
2. Provide clear status updates for multi-step automation processes.
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
