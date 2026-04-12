package compact

import (
	"dogclaw/internal/api"
)

// SnipConfig holds configuration for aggressive history snipping
type SnipConfig struct {
	Enabled       bool
	MaxMessages   int // Maximum messages before snipping triggers
	PreserveCount int // Number of recent messages to always preserve
}

// DefaultSnipConfig returns sensible defaults
func DefaultSnipConfig() *SnipConfig {
	return &SnipConfig{
		Enabled:       false, // 默认禁用 snip，优先使用更智能的 LLM 压缩
		MaxMessages:   50,    // Trigger snip if we have more than 50 messages
		PreserveCount: 6,     // Always keep the last 6 messages
	}
}

// SnipResult holds the result of a snip operation
type SnipResult struct {
	OriginalCount int
	SnippedCount  int
	Remaining     []api.MessageParam
}

// SnipHistory aggressively removes old messages to save context window
func SnipHistory(messages []api.MessageParam, config *SnipConfig) *SnipResult {
	if !config.Enabled || len(messages) <= config.MaxMessages {
		return nil // No snip needed
	}

	// Determine split point
	preserveStart := len(messages) - config.PreserveCount
	if preserveStart < 0 {
		preserveStart = 0
	}

	// Keep only the recent messages
	remaining := messages[preserveStart:]

	return &SnipResult{
		OriginalCount: len(messages),
		SnippedCount:  len(messages) - len(remaining),
		Remaining:     remaining,
	}
}

// IsSnipBoundaryMessage checks if a message is a safe boundary for snipping
// (e.g., we shouldn't snip in the middle of a tool call sequence)
func IsSnipBoundaryMessage(msg api.MessageParam) bool {
	// Safe boundaries: User text messages, or completed tool results
	switch content := msg.Content.(type) {
	case string:
		return msg.Role == "user"
	case []api.ContentBlockParam:
		// Check if it contains text or completed tool_result
		for _, block := range content {
			if block.Type == "tool_result" {
				return true // Completed tool result is a safe boundary
			}
			if block.Type == "text" && msg.Role == "user" {
				return true
			}
		}
	}
	return false
}
