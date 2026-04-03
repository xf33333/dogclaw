package constants

// CLISyspromptPrefix represents possible CLI sysprompt prefix values.
type CLISyspromptPrefix string

const (
	DefaultPrefix                  CLISyspromptPrefix = "You are Claude Code, Anthropic's official CLI for Claude."
	AgentSDKClaudeCodePresetPrefix CLISyspromptPrefix = "You are Claude Code, Anthropic's official CLI for Claude, running within the Claude Agent SDK."
	AgentSDKPrefix                 CLISyspromptPrefix = "You are a Claude agent, built on Anthropic's Claude Agent SDK."
)

// CLISyspromptPrefixes is the set of all possible CLI sysprompt prefix values.
var CLISyspromptPrefixes = map[string]struct{}{
	string(DefaultPrefix):                  {},
	string(AgentSDKClaudeCodePresetPrefix): {},
	string(AgentSDKPrefix):                 {},
}

// GetCLISyspromptPrefix returns the appropriate sysprompt prefix based on options.
// In Go, we don't have the same runtime feature detection as the TS version,
// so this is a simplified version.
func GetCLISyspromptPrefix(isNonInteractive, hasAppendSystemPrompt bool) CLISyspromptPrefix {
	if isNonInteractive {
		if hasAppendSystemPrompt {
			return AgentSDKClaudeCodePresetPrefix
		}
		return AgentSDKPrefix
	}
	return DefaultPrefix
}
