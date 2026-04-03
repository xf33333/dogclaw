package config

import (
	"dogclaw/pkg/types"
)

// Config holds the main configuration for DogClaw
type Config struct {
	APIKey               string
	Model                string
	BaseURL              string
	MaxTurns             int
	MaxTokens            int
	MaxBudgetUSD         float64
	PermissionMode       types.PermissionMode
	Verbose              bool
	Cwd                  string
	ShowToolUsageInReply bool
	ShowThinkingInLog    bool
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		//Model:          "openrouter/anthropic/claude-sonnet-4.5",
		Model:          "qwen/qwen3.6-plus:free",
		BaseURL:        "https://",
		MaxTurns:       1000,
		MaxTokens:      8192,
		PermissionMode: types.PermissionModeDefault,
		Cwd:            ".",
	}
}

// ConfigFromSettings creates a Config from the provided Settings and active provider model
func ConfigFromSettings(s *Settings) (*Config, error) {
	cfg := DefaultConfig()

	active, err := s.GetActive()
	if err != nil {
		return nil, err
	}

	cfg.Model = active.Model
	cfg.BaseURL = active.URL
	cfg.MaxTurns = s.MaxTurns
	cfg.MaxTokens = s.MaxTokens
	cfg.MaxBudgetUSD = s.MaxBudgetUSD
	cfg.PermissionMode = types.PermissionMode(s.PermissionMode)
	cfg.Verbose = s.Verbose
	cfg.ShowToolUsageInReply = s.ShowToolUsageInReply
	cfg.ShowThinkingInLog = s.ShowThinkingInLog

	return cfg, nil
}
