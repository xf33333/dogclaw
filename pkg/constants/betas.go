package constants

// Beta headers for Anthropic API requests.

const (
	ClaudeCode20250219BetaHeader  = "claude-code-20250219"
	InterleavedThinkingBetaHeader = "interleaved-thinking-2025-05-14"
	Context1MBetaHeader           = "context-1m-2025-08-07"
	ContextManagementBetaHeader   = "context-management-2025-06-27"
	StructuredOutputsBetaHeader   = "structured-outputs-2025-12-15"
	WebSearchBetaHeader           = "web-search-2025-03-05"

	// Tool search beta headers differ by provider:
	// - Claude API / Foundry: advanced-tool-use-2025-11-20
	// - Vertex AI / Bedrock: tool-search-tool-2025-10-19
	ToolSearchBetaHeader1P = "advanced-tool-use-2025-11-20"
	ToolSearchBetaHeader3P = "tool-search-tool-2025-10-19"

	EffortBetaHeader              = "effort-2025-11-24"
	TaskBudgetsBetaHeader         = "task-budgets-2026-03-13"
	PromptCachingScopeBetaHeader  = "prompt-caching-scope-2026-01-05"
	FastModeBetaHeader            = "fast-mode-2026-02-01"
	RedactThinkingBetaHeader      = "redact-thinking-2026-02-12"
	TokenEfficientToolsBetaHeader = "token-efficient-tools-2026-03-28"

	// Conditional betas — in Go these are always empty unless build tags enable them.
	SummarizeConnectorTextBetaHeader = "" // feature('CONNECTOR_TEXT') ? 'summarize-connector-text-2026-03-13' : ''
	AfkModeBetaHeader                = "" // feature('TRANSCRIPT_CLASSIFIER') ? 'afk-mode-2026-01-31' : ''
	CliInternalBetaHeader            = "" // USER_TYPE === 'ant' ? 'cli-internal-2026-02-09' : ''
	AdvisorBetaHeader                = "advisor-tool-2026-03-01"
)

// BedrockExtraParamsHeaders maintains the beta strings that should be in
// Bedrock extraBodyParams and not in Bedrock headers.
var BedrockExtraParamsHeaders = map[string]struct{}{
	InterleavedThinkingBetaHeader: {},
	Context1MBetaHeader:           {},
	ToolSearchBetaHeader3P:        {},
}

// VertexCountTokensAllowedBetas are betas allowed on Vertex countTokens API.
// Other betas will cause 400 errors.
var VertexCountTokensAllowedBetas = map[string]struct{}{
	ClaudeCode20250219BetaHeader:  {},
	InterleavedThinkingBetaHeader: {},
	ContextManagementBetaHeader:   {},
}
