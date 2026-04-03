package constants

import (
	"strings"
)

const (
	ProductURL = "https://claude.com/claude-code"

	// Claude Code Remote session URLs
	ClaudeAIBaseURL        = "https://claude.ai"
	ClaudeAIStagingBaseURL = "https://claude-ai.staging.ant.dev"
	ClaudeAILocalBaseURL   = "http://localhost:4000"
)

// IsRemoteSessionStaging determines if we're in a staging environment for remote sessions.
func IsRemoteSessionStaging(sessionID, ingressURL string) bool {
	return strings.Contains(sessionID, "_staging_") ||
		strings.Contains(ingressURL, "staging")
}

// IsRemoteSessionLocal determines if we're in a local-dev environment for remote sessions.
func IsRemoteSessionLocal(sessionID, ingressURL string) bool {
	return strings.Contains(sessionID, "_local_") ||
		strings.Contains(ingressURL, "localhost")
}

// GetClaudeAiBaseUrl returns the base URL for Claude AI based on environment.
func GetClaudeAiBaseUrl(sessionID, ingressURL string) string {
	if IsRemoteSessionLocal(sessionID, ingressURL) {
		return ClaudeAILocalBaseURL
	}
	if IsRemoteSessionStaging(sessionID, ingressURL) {
		return ClaudeAIStagingBaseURL
	}
	return ClaudeAIBaseURL
}

// GetRemoteSessionUrl returns the full session URL for a remote session.
func GetRemoteSessionUrl(sessionID, ingressURL string) string {
	compatID := ToCompatSessionID(sessionID)
	baseURL := GetClaudeAiBaseUrl(compatID, ingressURL)
	return baseURL + "/code/" + compatID
}

// ToCompatSessionID converts session IDs for compatibility.
func ToCompatSessionID(sessionID string) string {
	return sessionID
}
