package compact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dogclaw/pkg/types"
)

const (
	DefaultMaxTurns        = 50
	DefaultMaxMessageBytes = 500000
	defaultCompactDirName  = "compacts"
)

// SessionManager manages session compacts in memory (not persisted).
type SessionManager struct {
	mu           sync.RWMutex
	compactCache map[types.SessionID]string
	createdAt    map[types.SessionID]time.Time
}

// NewSessionManager creates a new session compact manager.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		compactCache: make(map[types.SessionID]string),
		createdAt:    make(map[types.SessionID]time.Time),
	}
}

// SetCompactForSession sets the compact string for a session.
func (s *SessionManager) SetCompactForSession(sessionID types.SessionID, compact string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.compactCache[sessionID] = compact
	s.createdAt[sessionID] = time.Now()
}

// GetCompactForSession retrieves the compact for a session.
func (s *SessionManager) GetCompactForSession(sessionID types.SessionID) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.compactCache[sessionID]
	return val, ok
}

// GetOrCreateCompact returns existing compact or builds a new one.
func (s *SessionManager) GetOrCreateCompact(sessionID types.SessionID, messages []types.Message, maxTurns int, maxMessageBytes int) (string, bool, error) {
	s.mu.RLock()
	cached, exists := s.compactCache[sessionID]
	s.mu.RUnlock()

	if exists && cached != "" {
		return cached, true, nil
	}

	if len(messages) == 0 {
		return "", false, nil
	}

	compact, err := BuildCompact(messages, maxTurns, maxMessageBytes)
	if err != nil {
		return "", false, fmt.Errorf("build compact: %w", err)
	}

	if compact == "" {
		return "", false, nil
	}

	s.mu.Lock()
	s.compactCache[sessionID] = compact
	s.createdAt[sessionID] = time.Now()
	s.mu.Unlock()

	return compact, false, nil
}

// RemoveCompact removes the compact for a session.
func (s *SessionManager) RemoveCompact(sessionID types.SessionID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.compactCache, sessionID)
	delete(s.createdAt, sessionID)
}

// HasCompact checks if a session has a compact.
func (s *SessionManager) HasCompact(sessionID types.SessionID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.compactCache[sessionID]
	return ok
}

// GetCompactDir returns the directory path for compacts (not persisted).
func GetCompactDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "goclaude", defaultCompactDirName)
	}
	return filepath.Join(home, ".dogclaw", defaultCompactDirName)
}

// BuildCompact creates a compressed summary of conversation messages.
// It keeps recent messages and summarizes older ones to stay within limits.
func BuildCompact(messages []types.Message, maxTurns int, maxMessageBytes int) (string, error) {
	if len(messages) == 0 {
		return "", nil
	}

	// Limit to maxTurns message pairs (user + assistant = 1 turn)
	totalMessages := len(messages)
	if maxTurns > 0 && totalMessages > maxTurns*2 {
		messages = messages[totalMessages-maxTurns*2:]
	}

	var sb strings.Builder
	sb.WriteString("# Session Compact\n\n")
	sb.WriteString(fmt.Sprintf("<!-- Compacted at %s, %d messages -->\n\n",
		time.Now().Format(time.RFC3339), len(messages)))

	totalBytes := 0
	for i, msg := range messages {
		content := formatMessage(msg)
		if totalBytes+len(content) > maxMessageBytes && maxMessageBytes > 0 {
			sb.WriteString("<!-- Truncated: remaining messages omitted due to size limit -->\n")
			break
		}
		sb.WriteString(content)
		sb.WriteString("\n")
		totalBytes += len(content)

		_ = i // unused for now
	}

	return sb.String(), nil
}

// formatMessage formats a message for compact representation
func formatMessage(msg types.Message) string {
	role := msg.Role
	if role == "" {
		role = msg.Type
	}

	contentStr := ""
	switch c := msg.Content.(type) {
	case string:
		contentStr = c
	case []interface{}:
		// Handle array content (e.g., mixed text/tool_use blocks)
		var parts []string
		for _, item := range c {
			if text, ok := item.(string); ok {
				parts = append(parts, text)
			} else {
				// Try to serialize structured content
				b, err := json.Marshal(item)
				if err == nil {
					parts = append(parts, string(b))
				}
			}
		}
		contentStr = strings.Join(parts, "\n")
	default:
		// Fall back to JSON serialization
		b, err := json.Marshal(msg.Content)
		if err == nil {
			contentStr = string(b)
		}
	}

	// Truncate very long content
	if len(contentStr) > 4000 {
		contentStr = contentStr[:4000] + "\n<!-- truncated -->"
	}

	return fmt.Sprintf("## %s\n\n%s", role, contentStr)
}
