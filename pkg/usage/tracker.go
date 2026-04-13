package usage

import (
	"fmt"
)

// TokenUsage represents token usage from a single API response
type TokenUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	// Reasoning tokens from extended thinking
	ReasoningInputTokens  int `json:"reasoning_input_tokens,omitempty"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens,omitempty"`
}

// AccumulatedUsage tracks total usage across multiple turns
type AccumulatedUsage struct {
	TotalInput          int
	TotalOutput         int
	TotalCacheRead      int
	TotalCacheCreation  int
	TotalReasoningInput int
	TotalReasoningOutput int
	Turns               int
}

// Add accumulates usage from a single response
func (a *AccumulatedUsage) Add(u TokenUsage) {
	a.TotalInput += u.InputTokens
	a.TotalOutput += u.OutputTokens
	a.TotalCacheRead += u.CacheReadInputTokens
	a.TotalCacheCreation += u.CacheCreationInputTokens
	a.TotalReasoningInput += u.ReasoningInputTokens
	a.TotalReasoningOutput += u.ReasoningOutputTokens
	a.Turns++
}

// TotalTokens returns the sum of all tokens
func (a *AccumulatedUsage) TotalTokens() int {
	return a.TotalInput + a.TotalOutput + a.TotalCacheRead + a.TotalCacheCreation + a.TotalReasoningInput + a.TotalReasoningOutput
}

// FormatTokens formats a token count with K/M/B suffixes
func FormatTokens(tokens int) string {
	switch {
	case tokens >= 1_000_000_000:
		return fmt.Sprintf("%.2fB", float64(tokens)/1_000_000_000.0)
	case tokens >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(tokens)/1_000_000.0)
	case tokens >= 1_000:
		return fmt.Sprintf("%.2fK", float64(tokens)/1_000.0)
	default:
		return fmt.Sprintf("%d", tokens)
	}
}
