package query

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"dogclaw/internal/api"
	"dogclaw/pkg/claudemd"
	"dogclaw/pkg/compact"
	ctxpkg "dogclaw/pkg/context"
	"dogclaw/pkg/fastmode"
	"dogclaw/pkg/history"
	"dogclaw/pkg/slash"
	"dogclaw/pkg/thinking"
	"dogclaw/pkg/types"
	"dogclaw/pkg/usage"
)

// QueryEngine manages the conversation loop with the LLM
type QueryEngine struct {
	client         *api.Client
	tools          []types.Tool
	messages       []api.MessageParam
	systemPrompt   string
	maxTurns       int
	currentTurn    int
	verbose        bool
	compactConfig  *compact.AutoCompactConfig
	compactTracker *compact.AutoCompactTracker
	snipConfig     *compact.SnipConfig
	cwd            string
	sessionID      string
	historyMgr     *history.HistoryManager

	// Slash command support
	cmdRegistry *slash.CommandRegistry

	// Skill registry
	skillRegistry *slash.SkillRegistry

	// Usage tracking
	usageTracker *usage.AccumulatedUsage
	modelName    string

	// Budget control
	maxBudgetUSD float64
	currentCost  float64

	// Thinking config
	thinkingConfig *thinking.Config

	// Fast mode manager
	fastModeManager *fastmode.Manager

	// Structured output
	jsonSchema                 map[string]any
	structuredOutputRetries    int
	maxStructuredOutputRetries int

	// Output writer (defaults to os.Stdout, can be set to terminal.Manager for readline-safe output)
	out io.Writer

	// Display settings
	showToolUsageInReply bool // Whether to show tool usage explanation in replies
	showThinkingInLog    bool // Whether to log LLM thinking content

	// ToolCallCallback is called each time a tool is invoked during SubmitMessage.
	// It receives the tool name and a human-readable summary of the input.
	ToolCallCallback func(toolName string, summary string)

	// LastTurnToolCalls records the last turn's tool use blocks (for channels to consume after SubmitMessage)
	LastTurnToolCalls []ToolCallInfo
}

// ToolCallInfo describes a single tool call for external consumers (e.g. QQ channel).
type ToolCallInfo struct {
	Name    string `json:"name"`
	Input   string `json:"input"`   // JSON-marshaled input
	Summary string `json:"summary"` // Human-readable summary
}

// NewQueryEngine creates a new query engine with context and compaction support
func NewQueryEngine(client *api.Client, tools []types.Tool, systemPrompt string, maxTurns int) *QueryEngine {
	// Get current working directory
	cwd, _ := os.Getwd()

	// Initialize history manager
	hm := history.GetHistoryManager()
	hm.Init(cwd, "default-session")

	// Initialize command registry
	cmdRegistry := slash.NewCommandRegistry()
	slash.RegisterBuiltinCommands(cmdRegistry)

	// Initialize skill registry
	skillRegistry := slash.NewSkillRegistry()
	skillRegistry.DiscoverAll(cwd)

	// Initialize usage tracker
	usageTracker := &usage.AccumulatedUsage{}

	return &QueryEngine{
		client:         client,
		tools:          tools,
		messages:       make([]api.MessageParam, 0),
		systemPrompt:   systemPrompt,
		maxTurns:       maxTurns,
		currentTurn:    0,
		compactConfig:  compact.DefaultAutoCompactConfig(),
		compactTracker: &compact.AutoCompactTracker{},
		snipConfig:     compact.DefaultSnipConfig(),
		cwd:            cwd,
		historyMgr:     hm,
		cmdRegistry:    cmdRegistry,
		skillRegistry:  skillRegistry,
		usageTracker:   usageTracker,
		modelName:      client.Model,
		maxBudgetUSD:   0, // unlimited
		thinkingConfig: thinking.DefaultConfig(),
		fastModeManager: func() *fastmode.Manager {
			m := fastmode.NewManager(true)
			m.SetModel(client.Model)
			return m
		}(),
		maxStructuredOutputRetries: 5,
	}
}

// SetVerbose enables/disables verbose mode
func (qe *QueryEngine) SetVerbose(verbose bool) {
	qe.verbose = verbose
}

// SetOutput sets the output writer for engine messages.
// Pass the terminal.Manager for readline-safe output that won't corrupt the prompt.
func (qe *QueryEngine) SetOutput(out io.Writer) {
	qe.out = out
}

// println writes output through the configured writer (or os.Stdout as fallback).
func (qe *QueryEngine) println(a ...any) {
	if qe.out != nil {
		fmt.Fprintln(qe.out, a...)
	} else {
		fmt.Println(a...)
	}
}

// printf writes formatted output through the configured writer (or os.Stdout as fallback).
func (qe *QueryEngine) printf(format string, a ...any) {
	if qe.out != nil {
		fmt.Fprintf(qe.out, format, a...)
	} else {
		fmt.Printf(format, a...)
	}
}

// SetSessionID sets the session ID for history tracking
func (qe *QueryEngine) SetSessionID(sessionID string) {
	qe.sessionID = sessionID
	qe.historyMgr.Init(qe.cwd, sessionID)
}

// GetMessages returns the current message list
func (qe *QueryEngine) GetMessages() []api.MessageParam {
	return qe.messages
}

// SubmitMessage processes a user message and runs the tool call loop
func (qe *QueryEngine) SubmitMessage(ctx context.Context, prompt string) error {
	// Check if this is a slash command
	if slash.IsSlashCommand(prompt) {
		return qe.handleSlashCommand(ctx, prompt)
	}

	// Add to history
	qe.historyMgr.AddSimpleHistory(prompt)

	// Add user message
	userMsg := api.MessageParam{
		Role:    "user",
		Content: prompt,
	}
	qe.messages = append(qe.messages, userMsg)

	// Main query loop
	for qe.currentTurn < qe.maxTurns {
		qe.currentTurn++

		if qe.verbose {
			qe.printf("[Turn %d/%d]\n", qe.currentTurn, qe.maxTurns)
		}

		// Check if auto-compact is needed
		if qe.compactConfig.Enabled {
			shouldCompact, tokenCount, threshold := compact.CheckAutoCompact(qe.messages, qe.compactConfig, qe.compactTracker)
			if shouldCompact {
				if qe.verbose {
					qe.printf("[Auto-compact triggered: %d tokens >= threshold %d]\n", tokenCount, threshold)
				}
				result, err := compact.CompactMessages(ctx, qe.client, qe.messages, qe.systemPrompt, qe.compactConfig)
				if err != nil {
					qe.printf("[Auto-compact error: %v]\n", err)
				} else if result != nil {
					qe.messages = compact.ApplyCompactResult(qe.messages, result)
					qe.compactTracker.Compacted = true
					qe.compactTracker.TurnCounter++
					if qe.verbose {
						qe.printf("[Auto-compact complete: %d -> %d messages, %d -> %d tokens]\n",
							result.OriginalMessageCount, result.CompactedMessageCount,
							result.PreCompactTokenCount, result.PostCompactTokenCount)
					}
				}
			} else {
				// Check for warning state
				warning, isBlocking := compact.GetWarningState(tokenCount, qe.compactConfig)
				if warning != "" {
					qe.println(warning)
				}
				if isBlocking {
					return fmt.Errorf("context window is full (blocking limit reached). Please start a new conversation.")
				}
			}
		}

		// Check if snip is needed (aggressive message count reduction)
		if qe.snipConfig.Enabled {
			snipResult := compact.SnipHistory(qe.messages, qe.snipConfig)
			if snipResult != nil {
				if qe.verbose {
					qe.printf("[Snip: removed %d messages, %d remaining]\n",
						snipResult.SnippedCount, len(snipResult.Remaining))
				}
				qe.messages = snipResult.Remaining
			}
		}

		// Build full system prompt with context
		fullSystemPrompt := qe.buildFullSystemPrompt()

		// Build API request
		req := &api.MessageRequest{
			Model:     qe.client.Model,
			MaxTokens: 8192,
			System:    fullSystemPrompt,
			Messages:  qe.messages,
			Tools:     qe.toAPITools(),
		}

		// Configure thinking based on current settings
		if qe.thinkingConfig.Enabled {
			if qe.thinkingConfig.Type == "adaptive" {
				req.Thinking = &api.ThinkingConfig{
					Type: "enabled",
				}
			} else {
				req.Thinking = &api.ThinkingConfig{
					Type:         "enabled",
					BudgetTokens: qe.thinkingConfig.BudgetTokens,
				}
			}
		} else {
			req.Thinking = &api.ThinkingConfig{
				Type: "disabled",
			}
		}

		// Use fast mode model if active
		if qe.fastModeManager.IsActive() {
			req.Model = qe.fastModeManager.GetModel()
		}

		// Call API
		resp, err := qe.client.SendMessage(ctx, req)
		if err != nil {
			return fmt.Errorf("API error: %w", err)
		}

		// Track usage from response
		if resp.Usage.InputTokens > 0 || resp.Usage.OutputTokens > 0 {
			tokenUsage := usage.TokenUsage{
				InputTokens:              resp.Usage.InputTokens,
				OutputTokens:             resp.Usage.OutputTokens,
				CacheReadInputTokens:     resp.Usage.CacheReadInputTokens,
				CacheCreationInputTokens: resp.Usage.CacheCreationInputTokens,
			}
			qe.usageTracker.Add(tokenUsage)

			// Update cost
			pricing := usage.GetPricingForModel(qe.modelName)
			qe.currentCost = qe.usageTracker.CalculateCost(pricing)

			// Check budget
			if qe.maxBudgetUSD > 0 && qe.currentCost >= qe.maxBudgetUSD {
				return fmt.Errorf("reached maximum budget ($%.2f)", qe.maxBudgetUSD)
			}
		}

		// Build assistant message content blocks
		var assistantContent []api.ContentBlockParam
		var toolUseDetails []string

		// Process response - add text content and capture thinking
		for _, block := range resp.Content {
			if block.Type == "text" && block.Text != "" {
				assistantContent = append(assistantContent, api.ContentBlockParam{
					Type: "text",
					Text: block.Text,
				})
			}
			if block.Type == "thinking" && block.Text != "" {
				if qe.showThinkingInLog {
					qe.printf("[🧠 Thinking (%d chars)]\n%s\n[End Thinking]\n", len(block.Text), block.Text)
				}
			}
		}

		// Check for tool calls
		var toolUseBlocks []api.ContentBlock
		for _, block := range resp.Content {
			if block.Type == "tool_use" {
				toolUseBlocks = append(toolUseBlocks, block)
			}
		}

		if len(toolUseBlocks) > 0 {
			// Add tool_use blocks to assistant message
			for _, block := range toolUseBlocks {
				assistantContent = append(assistantContent, api.ContentBlockParam{
					Type:  "tool_use",
					ID:    block.ID,
					Name:  block.Name,
					Input: block.Input,
				})

				// Collect tool use details for logging
				inputJSON, _ := json.Marshal(block.Input)
				detail := fmt.Sprintf("  📦 %s (id=%s): %s", block.Name, block.ID, string(inputJSON))
				toolUseDetails = append(toolUseDetails, detail)
			}

			// Log tool calls
			if qe.verbose {
				for _, detail := range toolUseDetails {
					qe.println(detail)
				}
			}
		}

		// Add assistant message to history
		if len(assistantContent) > 0 {
			qe.messages = append(qe.messages, api.MessageParam{
				Role:    "assistant",
				Content: assistantContent,
			})
		}

		if len(toolUseBlocks) == 0 {
			// No tool calls, just text response - we're done
			if qe.verbose {
				qe.println("[Response complete]")
			}
			return nil
		}

		// Reset per-turn tool call tracking
		qe.LastTurnToolCalls = nil

		// Execute tool calls and add results
		var toolResults []string
		for _, block := range toolUseBlocks {
			toolName := block.Name
			toolInput := block.Input
			toolUseID := block.ID

			if qe.verbose {
				qe.printf("[Tool call: %s (id=%s)]\n", toolName, toolUseID)
			}

			// Build human-readable summary + JSON summary for external consumers
			summary := buildToolCallSummary(toolName, toolInput)
			inputJSON, _ := json.Marshal(toolInput)

			// Record for channel consumption
			qe.LastTurnToolCalls = append(qe.LastTurnToolCalls, ToolCallInfo{
				Name:    toolName,
				Input:   string(inputJSON),
				Summary: summary,
			})

			// Invoke callback for real-time notification (e.g. QQ channel)
			if qe.ToolCallCallback != nil {
				qe.ToolCallCallback(toolName, summary)
			}

			// Find tool
			tool := qe.findTool(toolName)
			if tool == nil {
				qe.addToolResult(toolUseID, fmt.Sprintf("Error: Unknown tool '%s'", toolName), true)
				toolResults = append(toolResults, fmt.Sprintf("- **%s**: ❌ Unknown tool", toolName))
				continue
			}

			// Convert input to map
			inputMap, ok := toolInput.(map[string]any)
			if !ok {
				qe.addToolResult(toolUseID, "Error: Invalid tool input", true)
				toolResults = append(toolResults, fmt.Sprintf("- **%s**: ❌ Invalid input", toolName))
				continue
			}

			// Execute tool
			toolCtx := types.ToolUseContext{
				Cwd:             qe.cwd,
				AbortController: ctx,
				Tools:           qe.tools,
			}

			result, err := tool.Call(ctx, inputMap, toolCtx, nil)
			if err != nil {
				qe.addToolResult(toolUseID, fmt.Sprintf("Error: %v", err), true)
				toolResults = append(toolResults, fmt.Sprintf("- **%s**: ❌ Error: %v", toolName, err))
				continue
			}

			// Log tool result summary
			resultStr, _ := json.Marshal(result.Data)
			if qe.verbose {
				preview := string(resultStr)
				if len(preview) > 200 {
					preview = preview[:200] + "..."
				}
				qe.printf("  ✅ Result: %s\n", preview)
			}

			// Collect tool result for optional reply
			status := "✅"
			if result.IsError {
				status = "❌"
			}
			toolResults = append(toolResults, fmt.Sprintf("- **%s**: %s", toolName, status))

			// Add tool result
			qe.addToolResult(toolUseID, string(resultStr), result.IsError)
		}

		// If showToolUsageInReply is enabled, append tool usage summary to the assistant's text
		if qe.showToolUsageInReply && len(toolUseBlocks) > 0 {
			// Find the last text block to append the summary
			foundText := false
			for i := len(assistantContent) - 1; i >= 0; i-- {
				if assistantContent[i].Type == "text" {
					summary := "\n\n---\n**🔧 Tool Usage:**\n" + strings.Join(toolResults, "\n")
					assistantContent[i].Text += summary
					foundText = true
					break
				}
			}
			// If no text block exists, create one
			if !foundText {
				summary := "**🔧 Tool Usage:**\n" + strings.Join(toolResults, "\n")
				assistantContent = append(assistantContent, api.ContentBlockParam{
					Type: "text",
					Text: summary,
				})
			}
		}
	}

	return fmt.Errorf("reached maximum turns (%d)", qe.maxTurns)
}

// buildFullSystemPrompt combines base system prompt with context (git status, AGENT.md, etc.)
func (qe *QueryEngine) buildFullSystemPrompt() string {
	var parts []string

	// Base system prompt with tool descriptions
	if qe.systemPrompt != "" {
		parts = append(parts, qe.systemPrompt)
	}

	// Add skills section
	skillsSection := qe.skillRegistry.FormatSkillsForPrompt()
	if skillsSection != "" {
		parts = append(parts, skillsSection)
	}

	// Load AGENT.md context
	memoryFiles, err := claudemd.GetMemoryFiles(qe.cwd)
	if err == nil {
		agentMdContext := claudemd.BuildAgentMdContext(memoryFiles)
		if agentMdContext != "" {
			parts = append(parts, agentMdContext)
		}
	}

	// System context (git status, current date)
	sysCtx := ctxpkg.GetSystemContext()
	if sysCtx.GitStatus != "" {
		parts = append(parts, "## Git Status\n\n"+sysCtx.GitStatus)
	}
	parts = append(parts, sysCtx.CurrentDate)

	return strings.Join(parts, "\n\n")
}

// toAPITools converts tools to API tool definitions
func (qe *QueryEngine) toAPITools() []api.ToolParam {
	var apiTools []api.ToolParam
	for _, tool := range qe.tools {
		if !tool.IsEnabled() {
			continue
		}
		apiTools = append(apiTools, api.ToolParam{
			Name:        tool.Name(),
			Description: tool.Description(nil, types.ToolDescriptionOptions{}),
			InputSchema: tool.InputSchema(),
		})
	}
	return apiTools
}

// findTool finds a tool by name
func (qe *QueryEngine) findTool(name string) types.Tool {
	for _, tool := range qe.tools {
		if tool.Name() == name {
			return tool
		}
	}
	return nil
}

// addToolResult adds a tool result message
func (qe *QueryEngine) addToolResult(toolUseID string, content string, isError bool) {
	qe.messages = append(qe.messages, api.MessageParam{
		Role: "user",
		Content: []api.ContentBlockParam{
			{
				Type:      "tool_result",
				ToolUseID: toolUseID,
				Content: []api.ContentBlockParam{
					{
						Type: "text",
						Text: content,
					},
				},
				IsError: isError,
			},
		},
	})
}

// GetLastAssistantText returns the last assistant's text response
func (qe *QueryEngine) GetLastAssistantText() string {
	for i := len(qe.messages) - 1; i >= 0; i-- {
		msg := qe.messages[i]
		if msg.Role == "assistant" {
			if blocks, ok := msg.Content.([]api.ContentBlockParam); ok {
				for _, block := range blocks {
					if block.Type == "text" && block.Text != "" {
						return block.Text
					}
				}
			}
			if text, ok := msg.Content.(string); ok {
				return text
			}
		}
	}
	return ""
}

// handleSlashCommand processes a slash command and returns the result
func (qe *QueryEngine) handleSlashCommand(ctx context.Context, input string) error {
	result, err := qe.cmdRegistry.Execute(ctx, input)
	if err != nil {
		return fmt.Errorf("command error: %w", err)
	}

	if result == nil {
		return nil // Not a recognized command
	}

	if result.IsError {
		return fmt.Errorf("%s", result.ErrorMsg)
	}

	// Handle specific commands that modify engine state
	name, _ := slash.ParseCommand(input)
	switch strings.ToLower(name) {
	case "clear":
		qe.messages = make([]api.MessageParam, 0)
		qe.currentTurn = 0
		qe.usageTracker = &usage.AccumulatedUsage{}
		qe.historyMgr.Init(qe.cwd, qe.sessionID)
		qe.println(result.Output)

	case "model":
		// Extract model name from args
		_, args := slash.ParseCommand(input)
		if args != "" {
			qe.modelName = strings.ToLower(args)
			qe.client.Model = qe.modelName
		}
		qe.println(result.Output)

	case "verbose":
		qe.verbose = !qe.verbose
		qe.printf("Verbose mode: %v\n", qe.verbose)

	case "max-turns":
		_, args := slash.ParseCommand(input)
		if args != "" {
			var maxTurns int
			fmt.Sscanf(args, "%d", &maxTurns)
			if maxTurns > 0 {
				qe.maxTurns = maxTurns
			}
		}
		qe.println(result.Output)

	case "usage":
		// Use the skill-aware handler
		cmdResult, err := slash.HandleUsageCommand(ctx, "", qe.usageTracker)
		if err != nil {
			return err
		}
		qe.println(cmdResult.Output)

	case "skills":
		_, args := slash.ParseCommand(input)
		cmdResult, err := slash.HandleSkillsCommand(ctx, args, qe.skillRegistry)
		if err != nil {
			return err
		}
		qe.println(cmdResult.Output)

	case "thinking":
		_, args := slash.ParseCommand(input)
		args = strings.TrimSpace(args)
		if args == "" {
			// Show current thinking state
			state := "enabled"
			if !qe.thinkingConfig.Enabled {
				state = "disabled"
			}
			qe.printf("Thinking: %s (budget: %d tokens)\n", state, qe.thinkingConfig.BudgetTokens)
		} else {
			thinkType, err := thinking.ParseThinkingType(args)
			if err != nil {
				qe.printf("Error: %v\n", err)
				return nil
			}
			qe.thinkingConfig.Type = thinkType
			switch thinkType {
			case "enabled":
				qe.thinkingConfig.Enabled = true
				qe.thinkingConfig.BudgetTokens = 32000
			case "adaptive":
				qe.thinkingConfig.Enabled = true
				qe.thinkingConfig.BudgetTokens = 0
			case "disabled":
				qe.thinkingConfig.Enabled = false
				qe.thinkingConfig.BudgetTokens = 0
			}
			qe.printf("Thinking set to: %s\n", thinkType)
		}

	case "fast":
		_, args := slash.ParseCommand(input)
		args = strings.TrimSpace(args)
		if args == "" {
			// Show current fast mode state
			state := qe.fastModeManager.GetState()
			qe.printf("Fast Mode: %s\n", state)
			if state == fastmode.StateCooldown {
				remaining := qe.fastModeManager.TimeUntilCooldownEnd()
				qe.printf("Cooldown remaining: %v\n", remaining)
			}
		} else {
			switch strings.ToLower(args) {
			case "on", "enable":
				qe.fastModeManager = fastmode.NewManager(true)
				qe.fastModeManager.SetModel(qe.modelName)
				qe.println("Fast mode enabled")
			case "off", "disable":
				qe.fastModeManager.Disable()
				qe.println("Fast mode disabled")
			case "status":
				state := qe.fastModeManager.GetState()
				qe.printf("Fast Mode: %s (model: %s)\n", state, qe.fastModeManager.GetModel())
			default:
				qe.printf("Unknown fast mode argument: %s. Use: on/off/status\n", args)
			}
		}

	case "snip":
		_, args := slash.ParseCommand(input)
		args = strings.TrimSpace(args)
		if args == "" {
			// Force snip now
			snipResult := compact.SnipHistory(qe.messages, qe.snipConfig)
			if snipResult != nil {
				qe.messages = snipResult.Remaining
				qe.printf("Snipped %d messages, %d remaining\n", snipResult.SnippedCount, len(snipResult.Remaining))
			} else {
				qe.println("No snip needed")
			}
		} else {
			switch strings.ToLower(args) {
			case "on", "enable":
				qe.snipConfig.Enabled = true
				qe.println("Snip enabled")
			case "off", "disable":
				qe.snipConfig.Enabled = false
				qe.println("Snip disabled")
			case "status":
				qe.printf("Snip: enabled=%v, max_messages=%d, preserve=%d\n",
					qe.snipConfig.Enabled, qe.snipConfig.MaxMessages, qe.snipConfig.PreserveCount)
			}
		}

	case "showtools":
		_, args := slash.ParseCommand(input)
		args = strings.TrimSpace(args)
		if args == "" {
			state := "disabled"
			if qe.showToolUsageInReply {
				state = "enabled"
			}
			qe.printf("Show tool usage in reply: %s\n", state)
		} else {
			switch strings.ToLower(args) {
			case "on", "enable":
				qe.showToolUsageInReply = true
				qe.println("Tool usage will be shown in replies ✅")
			case "off", "disable":
				qe.showToolUsageInReply = false
				qe.println("Tool usage hidden from replies")
			default:
				qe.printf("Unknown argument: %s. Use: on/off\n", args)
			}
		}

	case "showthinking":
		_, args := slash.ParseCommand(input)
		args = strings.TrimSpace(args)
		if args == "" {
			state := "disabled"
			if qe.showThinkingInLog {
				state = "enabled"
			}
			qe.printf("Show thinking in log: %s\n", state)
		} else {
			switch strings.ToLower(args) {
			case "on", "enable":
				qe.showThinkingInLog = true
				qe.println("Thinking will be logged ✅")
			case "off", "disable":
				qe.showThinkingInLog = false
				qe.println("Thinking hidden from logs")
			default:
				qe.printf("Unknown argument: %s. Use: on/off\n", args)
			}
		}

	default:
		qe.println(result.Output)
	}

	return nil
}

// GetUsageTracker returns the current usage tracker
func (qe *QueryEngine) GetUsageTracker() *usage.AccumulatedUsage {
	return qe.usageTracker
}

// GetSkillRegistry returns the skill registry
func (qe *QueryEngine) GetSkillRegistry() *slash.SkillRegistry {
	return qe.skillRegistry
}

// SetMaxBudget sets the maximum budget in USD
func (qe *QueryEngine) SetMaxBudget(usd float64) {
	qe.maxBudgetUSD = usd
}

// GetCurrentCost returns the current session cost
func (qe *QueryEngine) GetCurrentCost() float64 {
	return qe.currentCost
}

// SetModel switches the current model
func (qe *QueryEngine) SetModel(model string) {
	qe.modelName = model
	qe.client.Model = model
}

// SetThinkingConfig sets the thinking configuration
func (qe *QueryEngine) SetThinkingConfig(config *thinking.Config) {
	qe.thinkingConfig = config
}

// SetShowToolUsageInReply sets whether to show tool usage in replies
func (qe *QueryEngine) SetShowToolUsageInReply(enabled bool) {
	qe.showToolUsageInReply = enabled
}

// SetShowThinkingInLog sets whether to log thinking content
func (qe *QueryEngine) SetShowThinkingInLog(enabled bool) {
	qe.showThinkingInLog = enabled
}

// GetThinkingConfig returns the current thinking config
func (qe *QueryEngine) GetThinkingConfig() *thinking.Config {
	return qe.thinkingConfig
}

// SetFastMode enables or disables fast mode
func (qe *QueryEngine) SetFastMode(enabled bool) {
	if enabled {
		qe.fastModeManager = fastmode.NewManager(true)
		qe.fastModeManager.SetModel(qe.modelName)
	} else {
		qe.fastModeManager.Disable()
	}
}

// IsFastModeActive checks if fast mode is currently active
func (qe *QueryEngine) IsFastModeActive() bool {
	return qe.fastModeManager.IsActive()
}

// GetFastModeModel returns the model to use if fast mode is active
func (qe *QueryEngine) GetFastModeModel() string {
	if qe.fastModeManager.IsActive() {
		return qe.fastModeManager.GetModel()
	}
	return qe.modelName
}

// ForceSnip triggers an immediate snip operation
func (qe *QueryEngine) ForceSnip() *compact.SnipResult {
	result := compact.SnipHistory(qe.messages, qe.snipConfig)
	if result != nil {
		qe.messages = result.Remaining
	}
	return result
}

// buildToolCallSummary creates a human-readable summary of a tool call.
func buildToolCallSummary(toolName string, input any) string {
	inputMap, ok := input.(map[string]any)
	if !ok {
		return toolName
	}

	switch toolName {
	case "read_file":
		if path, ok := inputMap["file_path"].(string); ok {
			return fmt.Sprintf("读取 %s", path)
		}
	case "write_file":
		if path, ok := inputMap["file_path"].(string); ok {
			return fmt.Sprintf("写入 %s", path)
		}
	case "edit_file":
		if path, ok := inputMap["file_path"].(string); ok {
			return fmt.Sprintf("编辑 %s", path)
		}
	case "bash":
		if cmd, ok := inputMap["command"].(string); ok {
			if len(cmd) > 80 {
				cmd = cmd[:80] + "…"
			}
			return fmt.Sprintf("执行命令: %s", cmd)
		}
	case "grep":
		if pattern, ok := inputMap["pattern"].(string); ok {
			if path, ok := inputMap["path"].(string); ok {
				return fmt.Sprintf("搜索 %s (路径: %s)", pattern, path)
			}
			return fmt.Sprintf("搜索 %s", pattern)
		}
	case "glob":
		if pattern, ok := inputMap["pattern"].(string); ok {
			return fmt.Sprintf("查找文件 %s", pattern)
		}
	case "web_search":
		if query, ok := inputMap["query"].(string); ok {
			return fmt.Sprintf("搜索网络: %s", query)
		}
	case "web_fetch":
		if url, ok := inputMap["url"].(string); ok {
			if len(url) > 60 {
				url = url[:60] + "…"
			}
			return fmt.Sprintf("获取网页: %s", url)
		}
	}

	// Generic: show first meaningful value
	for k, v := range inputMap {
		if s, ok := v.(string); ok && len(s) > 0 {
			if len(s) > 60 {
				s = s[:60] + "…"
			}
			return fmt.Sprintf("%s(%s=%s)", toolName, k, s)
		}
	}
	return toolName
}

// BuildSystemPrompt builds the system prompt with tool descriptions
func BuildSystemPrompt(tools []types.Tool, customPrompt string) string {
	var sb strings.Builder

	if customPrompt != "" {
		sb.WriteString(customPrompt)
		sb.WriteString("\n\n")
	}

	sb.WriteString("You are DogClaw, a helpful AI coding assistant implemented in Go. " +
		"You can help with software engineering tasks including writing code, debugging, " +
		"file manipulation, and web research.\n\n")

	sb.WriteString("## Available Tools\n\n")
	for _, tool := range tools {
		if !tool.IsEnabled() {
			continue
		}
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", tool.Name(),
			tool.Description(nil, types.ToolDescriptionOptions{})))
	}

	sb.WriteString("\n## Guidelines\n\n")
	sb.WriteString("- Use tools when needed to accomplish tasks\n")
	sb.WriteString("- Be concise and accurate\n")
	sb.WriteString("- Show code and command output when relevant\n")
	sb.WriteString("- Think step by step before acting\n")

	return sb.String()
}
