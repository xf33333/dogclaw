package thinking

import (
	"fmt"
	"regexp"
	"strings"
)

// Config represents the thinking configuration
type Config struct {
	Enabled      bool   // Whether thinking is enabled
	BudgetTokens int    // Maximum thinking tokens
	Type         string // "enabled", "disabled", or "adaptive"
}

// DefaultConfig returns the default thinking config
func DefaultConfig() *Config {
	return &Config{
		Enabled:      false,
		BudgetTokens: 32000, // Default thinking budget
		Type:         "enabled",
	}
}

// AdaptiveConfig returns adaptive thinking config
func AdaptiveConfig() *Config {
	return &Config{
		Enabled:      true,
		BudgetTokens: 0, // 0 means auto/adaptive
		Type:         "adaptive",
	}
}

// DisabledConfig returns disabled thinking config
func DisabledConfig() *Config {
	return &Config{
		Enabled:      false,
		BudgetTokens: 0,
		Type:         "disabled",
	}
}

// ParseThinkingType parses a thinking type string
func ParseThinkingType(s string) (string, error) {
	switch strings.ToLower(s) {
	case "enabled", "on", "true":
		return "enabled", nil
	case "disabled", "off", "false":
		return "disabled", nil
	case "adaptive", "auto":
		return "adaptive", nil
	default:
		return "", fmt.Errorf("unknown thinking type: %s", s)
	}
}

// ModelSupportsThinking checks if a model supports thinking
func ModelSupportsThinking(model string) bool {
	model = strings.ToLower(model)
	// Claude 4+ models support thinking
	return strings.Contains(model, "claude-4") ||
		strings.Contains(model, "opus-4") ||
		strings.Contains(model, "sonnet-4") ||
		strings.Contains(model, "opus-4-6") ||
		strings.Contains(model, "sonnet-4-6")
}

// ModelSupportsAdaptiveThinking checks if a model supports adaptive thinking
func ModelSupportsAdaptiveThinking(model string) bool {
	model = strings.ToLower(model)
	// Only specific models support adaptive thinking
	return strings.Contains(model, "opus-4-6") ||
		strings.Contains(model, "sonnet-4-6")
}

// HasUltrathinkKeyword checks if text contains the "ultrathink" keyword
var ultrathinkRegex = regexp.MustCompile(`(?i)\bultrathink\b`)

func HasUltrathinkKeyword(text string) bool {
	return ultrathinkRegex.MatchString(text)
}
