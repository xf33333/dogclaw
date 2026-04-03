package constants

import (
	"fmt"
	"os"
	"strings"
)

const (
	ClaudeAIInferenceScope = "user:inference"
	ClaudeAIProfileScope   = "user:profile"
	OAuthBetaHeader        = "oauth-2025-04-20"
)

// Console OAuth scopes - for API key creation via Console
var ConsoleOAuthScopes = []string{
	"org:create_api_key",
	ClaudeAIProfileScope,
}

// Claude.ai OAuth scopes - for Claude.ai subscribers (Pro/Max/Team/Enterprise)
var ClaudeAIOAuthScopes = []string{
	ClaudeAIProfileScope,
	ClaudeAIInferenceScope,
	"user:sessions:claude_code",
	"user:mcp_servers",
	"user:file_upload",
}

// AllOAuthScopes is the union of all scopes used in Claude CLI.
var AllOAuthScopes = func() []string {
	scopeMap := make(map[string]struct{})
	for _, s := range ConsoleOAuthScopes {
		scopeMap[s] = struct{}{}
	}
	for _, s := range ClaudeAIOAuthScopes {
		scopeMap[s] = struct{}{}
	}
	result := make([]string, 0, len(scopeMap))
	for s := range scopeMap {
		result = append(result, s)
	}
	return result
}()

// OAuthConfig holds OAuth configuration
type OAuthConfig struct {
	BaseAPIURL           string
	ConsoleAuthorizeURL  string
	ClaudeAIAuthorizeURL string
	ClaudeAIOrigin       string
	TokenURL             string
	APIKeyURL            string
	RolesURL             string
	ConsoleSuccessURL    string
	ClaudeAISuccessURL   string
	ManualRedirectURL    string
	ClientID             string
	OAuthFileSuffix      string
	MCPProxyURL          string
	MCPProxyPath         string
}

// MCPClientMetadataURL is the Client ID Metadata Document URL for MCP OAuth (CIMD / SEP-991).
const MCPClientMetadataURL = "https://claude.ai/oauth/claude-code-client-metadata"

// Production OAuth configuration
var prodOAuthConfig = OAuthConfig{
	BaseAPIURL:           "https://api.anthropic.com",
	ConsoleAuthorizeURL:  "https://platform.claude.com/oauth/authorize",
	ClaudeAIAuthorizeURL: "https://claude.com/cai/oauth/authorize",
	ClaudeAIOrigin:       "https://claude.ai",
	TokenURL:             "https://platform.claude.com/v1/oauth/token",
	APIKeyURL:            "https://api.anthropic.com/api/oauth/claude_cli/create_api_key",
	RolesURL:             "https://api.anthropic.com/api/oauth/claude_cli/roles",
	ConsoleSuccessURL:    "https://platform.claude.com/buy_credits?returnUrl=/oauth/code/success%3Fapp%3Dclaude-code",
	ClaudeAISuccessURL:   "https://platform.claude.com/oauth/code/success?app=claude-code",
	ManualRedirectURL:    "https://platform.claude.com/oauth/code/callback",
	ClientID:             "9d1c250a-e61b-44d9-88ed-5944d1962f5e",
	OAuthFileSuffix:      "",
	MCPProxyURL:          "https://mcp-proxy.anthropic.com",
	MCPProxyPath:         "/v1/mcp/{server_id}",
}

// Allowed base URLs for CLAUDE_CODE_CUSTOM_OAUTH_URL override.
var allowedOAuthBaseURLs = []string{
	"https://beacon.claude-ai.staging.ant.dev",
	"https://claude.fedstart.com",
	"https://claude-staging.fedstart.com",
}

// FileSuffixForOAuthConfig returns the file suffix for the current OAuth config.
func FileSuffixForOAuthConfig() string {
	if os.Getenv("CLAUDE_CODE_CUSTOM_OAUTH_URL") != "" {
		return "-custom-oauth"
	}
	switch getOAuthConfigType() {
	case "local":
		return "-local-oauth"
	case "staging":
		return "-staging-oauth"
	case "prod":
		return ""
	}
	return ""
}

// GetOAuthConfig returns the OAuth configuration.
func GetOAuthConfig() OAuthConfig {
	config := func() OAuthConfig {
		switch getOAuthConfigType() {
		case "local":
			return getLocalOAuthConfig()
		case "staging":
			return prodOAuthConfig
		case "prod":
			return prodOAuthConfig
		}
		return prodOAuthConfig
	}()

	if oauthBaseURL := os.Getenv("CLAUDE_CODE_CUSTOM_OAUTH_URL"); oauthBaseURL != "" {
		base := strings.TrimRight(oauthBaseURL, "/")
		allowed := false
		for _, u := range allowedOAuthBaseURLs {
			if u == base {
				allowed = true
				break
			}
		}
		if !allowed {
			panic("CLAUDE_CODE_CUSTOM_OAUTH_URL is not an approved endpoint.")
		}
		config = OAuthConfig{
			BaseAPIURL:           base,
			ConsoleAuthorizeURL:  base + "/oauth/authorize",
			ClaudeAIAuthorizeURL: base + "/oauth/authorize",
			ClaudeAIOrigin:       base,
			TokenURL:             base + "/v1/oauth/token",
			APIKeyURL:            base + "/api/oauth/claude_cli/create_api_key",
			RolesURL:             base + "/api/oauth/claude_cli/roles",
			ConsoleSuccessURL:    base + "/oauth/code/success?app=claude-code",
			ClaudeAISuccessURL:   base + "/oauth/code/success?app=claude-code",
			ManualRedirectURL:    base + "/oauth/code/callback",
			OAuthFileSuffix:      "-custom-oauth",
			MCPProxyURL:          config.MCPProxyURL,
			MCPProxyPath:         config.MCPProxyPath,
			ClientID:             config.ClientID,
		}
	}

	if clientID := os.Getenv("CLAUDE_CODE_OAUTH_CLIENT_ID"); clientID != "" {
		config.ClientID = clientID
	}

	return config
}

func getOAuthConfigType() string {
	userType := os.Getenv("USER_TYPE")
	if userType == "ant" {
		if isEnvTruthy(os.Getenv("USE_LOCAL_OAUTH")) {
			return "local"
		}
		if isEnvTruthy(os.Getenv("USE_STAGING_OAUTH")) {
			return "staging"
		}
	}
	return "prod"
}

func isEnvTruthy(v string) bool {
	return v == "1" || v == "true" || v == "yes"
}

func getLocalOAuthConfig() OAuthConfig {
	api := strings.TrimRight(os.Getenv("CLAUDE_LOCAL_OAUTH_API_BASE"), "/")
	if api == "" {
		api = "http://localhost:8000"
	}
	apps := strings.TrimRight(os.Getenv("CLAUDE_LOCAL_OAUTH_APPS_BASE"), "/")
	if apps == "" {
		apps = "http://localhost:4000"
	}
	consoleBase := strings.TrimRight(os.Getenv("CLAUDE_LOCAL_OAUTH_CONSOLE_BASE"), "/")
	if consoleBase == "" {
		consoleBase = "http://localhost:3000"
	}

	return OAuthConfig{
		BaseAPIURL:           api,
		ConsoleAuthorizeURL:  fmt.Sprintf("%s/oauth/authorize", consoleBase),
		ClaudeAIAuthorizeURL: fmt.Sprintf("%s/oauth/authorize", apps),
		ClaudeAIOrigin:       apps,
		TokenURL:             fmt.Sprintf("%s/v1/oauth/token", api),
		APIKeyURL:            fmt.Sprintf("%s/api/oauth/claude_cli/create_api_key", api),
		RolesURL:             fmt.Sprintf("%s/api/oauth/claude_cli/roles", api),
		ConsoleSuccessURL:    fmt.Sprintf("%s/buy_credits?returnUrl=/oauth/code/success%%3Fapp%%3Dclaude-code", consoleBase),
		ClaudeAISuccessURL:   fmt.Sprintf("%s/oauth/code/success?app=claude-code", consoleBase),
		ManualRedirectURL:    fmt.Sprintf("%s/oauth/code/callback", consoleBase),
		ClientID:             "22422756-60c9-4084-8eb7-27705fd5cf9a",
		OAuthFileSuffix:      "-local-oauth",
		MCPProxyURL:          "http://localhost:8205",
		MCPProxyPath:         "/v1/toolbox/shttp/mcp/{server_id}",
	}
}
