package compact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dogclaw/internal/api"
)

func makeTimestamp() int64 {
	return time.Now().UnixMilli()
}

// CompactedSession holds the compacted session state for persistence
type CompactedSession struct {
	Version           string                 `json:"version"`
	Timestamp         int64                  `json:"timestamp"`
	OriginalMessages  int                    `json:"original_messages"`
	CompactedMessages int                    `json:"compacted_messages"`
	PreTokens         int                    `json:"pre_tokens"`
	PostTokens        int                    `json:"post_tokens"`
	Messages          []api.MessageParam     `json:"messages"`
}

const compactedSessionVersion = "1.0"

// SerializeCompactedSession serializes a compact result and messages to JSON
func SerializeCompactedSession(result *CompactResult, messages []api.MessageParam) (string, error) {
	if result == nil {
		return "", fmt.Errorf("compact result is nil")
	}

	session := CompactedSession{
		Version:           compactedSessionVersion,
		Timestamp:         nowUnixMilli(),
		OriginalMessages:  result.OriginalMessageCount,
		CompactedMessages: result.CompactedMessageCount,
		PreTokens:         result.PreCompactTokenCount,
		PostTokens:        result.PostCompactTokenCount,
		Messages:          messages,
	}

	data, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("marshal compacted session: %w", err)
	}

	return string(data), nil
}

// DeserializeCompactedSession deserializes JSON back to CompactedSession
func DeserializeCompactedSession(data string) (*CompactedSession, error) {
	var session CompactedSession
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, fmt.Errorf("unmarshal compacted session: %w", err)
	}
	return &session, nil
}

func nowUnixMilli() int64 {
	return makeTimestamp()
}

// AutoCompactConfig holds configuration for automatic context compaction
type AutoCompactConfig struct {
	Enabled            bool
	ThresholdRatio     float64 // Trigger compaction when context exceeds this ratio of model's context window
	WarningRatio       float64 // Show warning when context exceeds this ratio
	MaxContextTokens   int     // Maximum context tokens before blocking
	ModelContextWindow int     // Model's max context window (e.g., 200000)
}

// DefaultAutoCompactConfig returns sensible defaults
func DefaultAutoCompactConfig() *AutoCompactConfig {
	return &AutoCompactConfig{
		Enabled:            true,
		ThresholdRatio:     0.75, // Compact at 75% of context window
		WarningRatio:       0.65, // Warn at 65%
		MaxContextTokens:   190000,
		ModelContextWindow: 200000,
	}
}

// CompactResult holds the result of a compaction operation
type CompactResult struct {
	OriginalMessageCount  int
	CompactedMessageCount int
	PreCompactTokenCount  int
	PostCompactTokenCount int
	SummaryMessages       []api.MessageParam
	Attachments           []api.MessageParam
}

// AutoCompactTracker tracks compaction state across turns
type AutoCompactTracker struct {
	Compacted           bool
	TurnID              string
	TurnCounter         int
	ConsecutiveFailures int
}

// EstimateTokenCount estimates token count for messages (rough approximation)
// 1 token ≈ 4 chars for English text
func EstimateTokenCount(text string) int {
	return len(text) / 4
}

// EstimateMessagesTokenCount estimates total token count for a message list
func EstimateMessagesTokenCount(messages []api.MessageParam) int {
	total := 0
	for _, msg := range messages {
		switch content := msg.Content.(type) {
		case string:
			total += EstimateTokenCount(content)
		case []api.ContentBlockParam:
			for _, block := range content {
				if block.Type == "text" {
					total += EstimateTokenCount(block.Text)
				} else if block.Type == "tool_use" {
					data, _ := json.Marshal(block.Input)
					total += EstimateTokenCount(string(data))
				} else if block.Type == "tool_result" {
					if blocks, ok := block.Content.([]api.ContentBlockParam); ok {
						for _, sub := range blocks {
							if sub.Type == "text" {
								total += EstimateTokenCount(sub.Text)
							}
						}
					}
				}
			}
		}
	}
	return total
}

// CheckAutoCompact checks if compaction should be triggered
func CheckAutoCompact(messages []api.MessageParam, config *AutoCompactConfig, tracker *AutoCompactTracker) (bool, int, int) {
	tokenCount := EstimateMessagesTokenCount(messages)
	threshold := int(float64(config.ModelContextWindow) * config.ThresholdRatio)

	return tokenCount >= threshold, tokenCount, threshold
}

// GetWarningState returns warning information based on current context usage
func GetWarningState(tokenCount int, config *AutoCompactConfig) (warning string, isAtBlockingLimit bool) {
	warningRatio := int(float64(config.ModelContextWindow) * config.WarningRatio)

	if tokenCount >= config.MaxContextTokens {
		return "", true // At blocking limit
	}

	if tokenCount >= warningRatio {
		pct := float64(tokenCount) / float64(config.ModelContextWindow) * 100
		warning = fmt.Sprintf("⚠️ Context window usage: %.0f%% (%d/%d tokens)", pct, tokenCount, config.ModelContextWindow)
	}

	return warning, false
}

// CompactMessages performs context compaction by summarizing older messages
// Uses the LLM to generate a summary of earlier conversation turns
func CompactMessages(
	ctx context.Context,
	client *api.Client,
	messages []api.MessageParam,
	systemPrompt string,
	config *AutoCompactConfig,
) (*CompactResult, error) {
	if len(messages) < 4 {
		return nil, nil // Too few messages to compact
	}

	tokenCount := EstimateMessagesTokenCount(messages)
	threshold := int(float64(config.ModelContextWindow) * config.ThresholdRatio)

	if tokenCount < threshold {
		return nil, nil // Under threshold, no compaction needed
	}

	// Determine split point: preserve recent messages, compact older ones
	// Keep last 3-4 messages (recent context), compact everything before
	preserveCount := 4
	if len(messages) <= preserveCount {
		return nil, nil // Not enough messages to preserve
	}

	messagesToCompact := messages[:len(messages)-preserveCount]
	messagesToPreserve := messages[len(messages)-preserveCount:]

	// Build compact request
	compactPrompt := buildCompactPrompt(messagesToCompact)
	compactMessages := []api.MessageParam{
		{Role: "user", Content: compactPrompt},
	}

	compactReq := &api.MessageRequest{
		Model:     client.Model,
		MaxTokens: 4096,
		System:    buildCompactSystemPrompt(systemPrompt),
		Messages:  compactMessages,
	}

	resp, err := client.SendMessage(ctx, compactReq)
	if err != nil {
		return nil, fmt.Errorf("compact API error: %w", err)
	}

	// Extract summary text
	var summaryText string
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != "" {
			summaryText += block.Text
		}
	}

	if summaryText == "" {
		return nil, fmt.Errorf("compact returned empty summary")
	}

	// Build post-compact messages
	postCompactMessages := []api.MessageParam{
		{
			Role: "user",
			Content: []api.ContentBlockParam{
				{
					Type: "text",
					Text: fmt.Sprintf("[Previous conversation summary]\n\n%s\n\n[End of summary]\n\nContinue from where the conversation left off.", summaryText),
				},
			},
		},
	}

	// Append preserved messages
	postCompactMessages = append(postCompactMessages, messagesToPreserve...)

	result := &CompactResult{
		OriginalMessageCount:  len(messages),
		CompactedMessageCount: len(postCompactMessages),
		PreCompactTokenCount:  tokenCount,
		PostCompactTokenCount: EstimateMessagesTokenCount(postCompactMessages),
		SummaryMessages:       postCompactMessages[:1],
	}

	return result, nil
}

// buildCompactPrompt creates the prompt for generating conversation summary
func buildCompactPrompt(messages []api.MessageParam) string {
	var sb strings.Builder

	sb.WriteString("Please provide a concise summary of the following conversation.\n\n")
	sb.WriteString("Include:\n")
	sb.WriteString("- Key decisions made\n")
	sb.WriteString("- Important code changes or file modifications\n")
	sb.WriteString("- Current task status and what remains to be done\n")
	sb.WriteString("- Any relevant tool use results\n\n")
	sb.WriteString("Exclude:\n")
	sb.WriteString("- Repetitive tool calls and their outputs\n")
	sb.WriteString("- Minor corrections or typos\n")
	sb.WriteString("- Intermediate thinking steps\n\n")
	sb.WriteString("Keep the summary under 2000 characters.\n\n")
	sb.WriteString("--- Conversation History ---\n\n")

	for _, msg := range messages {
		switch msg.Role {
		case "user":
			switch content := msg.Content.(type) {
			case string:
				sb.WriteString(fmt.Sprintf("User: %s\n\n", content))
			case []api.ContentBlockParam:
				for _, block := range content {
					if block.Type == "text" {
						sb.WriteString(fmt.Sprintf("User: %s\n\n", block.Text))
					}
				}
			}
		case "assistant":
			if blocks, ok := msg.Content.([]api.ContentBlockParam); ok {
				for _, block := range blocks {
					if block.Type == "text" && block.Text != "" {
						sb.WriteString(fmt.Sprintf("Assistant: %s\n\n", block.Text))
					}
				}
			}
		}
	}

	sb.WriteString("--- End of Conversation ---\n\n")
	sb.WriteString("Provide your summary:")

	return sb.String()
}

// buildCompactSystemPrompt creates the system prompt for the compact operation
func buildCompactSystemPrompt(originalSystemPrompt string) string {
	return fmt.Sprintf(`You are a conversation summarization assistant.

Your task is to summarize a conversation between a user and an AI coding assistant.
The summary should capture all important context so the conversation can continue seamlessly.

%s

Be concise but thorough. Focus on actionable information and current state.`, originalSystemPrompt)
}

// ApplyCompactResult applies a compaction result to the message list
func ApplyCompactResult(messages []api.MessageParam, result *CompactResult) []api.MessageParam {
	if result == nil {
		return messages
	}
	return result.SummaryMessages
}
