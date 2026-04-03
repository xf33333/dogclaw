package sessionmemory

import (
	"fmt"
	"strings"

	"dogclaw/pkg/constants"
)

// BuildExtractionPrompt builds the prompt for the extraction subagent
func BuildExtractionPrompt(currentMemory, conversationSummary string, sections []Section) string {
	var sb strings.Builder

	// System instructions
	sb.WriteString(constants.SessionMemoryExtractionInstructions)
	sb.WriteString("\n\n")

	// Current memory content
	if currentMemory != "" {
		sb.WriteString("<current_memory>\n")
		sb.WriteString(currentMemory)
		sb.WriteString("\n</current_memory>\n\n")
	}

	// Conversation content to extract from
	sb.WriteString("<conversation>\n")
	sb.WriteString(conversationSummary)
	sb.WriteString("\n</conversation>\n\n")

	// Section instructions
	sb.WriteString("<sections>\n")
	for _, section := range sections {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", section.Name, section.Description))
	}
	sb.WriteString("</sections>\n\n")

	// Update instructions
	sb.WriteString("## Instructions\n\n")
	sb.WriteString("1. Review the current memory file and the conversation content.\n")
	sb.WriteString("2. Extract new and relevant information from the conversation.\n")
	sb.WriteString("3. Update the sections above with the extracted information.\n")
	sb.WriteString("4. Keep information concise and actionable.\n")
	sb.WriteString("5. Use the Edit tool to update the SESSION.md file.\n")
	sb.WriteString("6. Only update sections that have new information.\n")
	sb.WriteString("7. Preserve existing information that is still relevant.\n")
	sb.WriteString("8. Remove outdated information if superseded by new content.\n")

	return sb.String()
}

// BuildConversationSummaryForExtraction builds a summary of the conversation
// suitable for the extraction agent to process.
// It includes user messages and assistant responses with tool results.
func BuildConversationSummaryForExtraction(messages []ConversationMessage) string {
	var sb strings.Builder

	for _, msg := range messages {
		switch msg.Role {
		case "user":
			sb.WriteString(fmt.Sprintf("### User: %s\n\n", msg.Content))
		case "assistant":
			content := msg.Content
			if len(msg.ToolCalls) > 0 {
				var toolCalls []string
				for _, tc := range msg.ToolCalls {
					toolCalls = append(toolCalls, fmt.Sprintf("Called %s(%s)", tc.Name, truncate(tc.Input, 200)))
				}
				content = content + "\n\n" + strings.Join(toolCalls, "\n")
			}
			sb.WriteString(fmt.Sprintf("### Assistant: %s\n\n", content))
		case "tool_result":
			sb.WriteString(fmt.Sprintf("### Tool %s Result: %s\n\n", msg.ToolName, truncate(msg.Content, 500)))
		}
	}

	return sb.String()
}

// ConversationMessage represents a message in the conversation
type ConversationMessage struct {
	Role      string
	Content   string
	ToolCalls []ToolCallInfo
	ToolName  string
}

// ToolCallInfo represents information about a tool call
type ToolCallInfo struct {
	Name  string
	Input string
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
