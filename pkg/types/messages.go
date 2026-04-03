package types

// Message represents a message in the conversation
type Message struct {
	Type    string `json:"type"` // "user", "assistant", "system", "tool_result", "tool_use"
	Content any    `json:"content"`
	Role    string `json:"role,omitempty"` // "user", "assistant", "system"
	UUID    string `json:"uuid,omitempty"`
}

// UserMessage represents a message from the user
type UserMessage struct {
	Message Message `json:"message"`
	UUID    string  `json:"uuid"`
}

// AssistantMessage represents a message from the assistant
type AssistantMessage struct {
	Message Message `json:"message"`
	UUID    string  `json:"uuid"`
}

// SystemMessage represents a system message
type SystemMessage struct {
	Content string `json:"content"`
	UUID    string `json:"uuid"`
}

// ToolUseMessage represents a tool use request
type ToolUseMessage struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input any    `json:"input"`
}

// ToolResultMessage represents a tool result
type ToolResultMessage struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error"`
}
