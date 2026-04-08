package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"dogclaw/internal/api"
	"dogclaw/internal/logger"
	"dogclaw/pkg/claudemd"
	"dogclaw/pkg/compact"
	ctxpkg "dogclaw/pkg/context"
	"dogclaw/pkg/memory"
	"dogclaw/pkg/slash"
	"dogclaw/pkg/transcript"
	"dogclaw/pkg/types"
	"dogclaw/pkg/usage"
)

// verboseLogLimit is the maximum length of a single content block in verbose output.
const verboseLogLimit = 2000

// formatTruncated returns s truncated to n characters, with "..." appended if truncated.
func formatTruncated(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// parsedToolCall represents a tool call extracted from text
type parsedToolCall struct {
	ID    string
	Name  string
	Input map[string]any
}

// parseToolUseFromText extracts <function=...> tags from text and returns tool calls.
// Supports formats like: <function=Bash command="..."> or <function=Bash><parameter=command>...</parameter></function>
func parseToolUseFromText(text string) []parsedToolCall {
	var calls []parsedToolCall

	// Regex matches either <function=Name>...</function> or <function=Name ...>
	reFunc := regexp.MustCompile(`(?s)<function=([a-zA-Z0-9_\-]+)([^>]*)>(.*?)</function>|<function=([a-zA-Z0-9_\-]+)([^>]*)>`)
	matches := reFunc.FindAllStringSubmatch(text, -1)

	for _, m := range matches {
		var toolName, attrs, inner string
		if m[1] != "" {
			// Matched <function=Name>...</function>
			toolName = m[1]
			attrs = m[2]
			inner = m[3]
		} else if m[4] != "" {
			// Matched <function=Name ...>
			toolName = m[4]
			attrs = m[5]
		} else {
			continue
		}

		inputArgs := make(map[string]any)

		// Parse inner parameters <parameter=name>value</parameter>
		if inner != "" {
			paramRe := regexp.MustCompile(`(?s)<parameter=([a-zA-Z0-9_\-]+)>(.*?)</parameter>`)
			paramMatches := paramRe.FindAllStringSubmatch(inner, -1)
			for _, pm := range paramMatches {
				name := pm[1]
				// Get the content, trim trailing/leading spaces without removing indentations
				value := regexp.MustCompile(`^\s*|\s*$`).ReplaceAllString(pm[2], "")

				// Try to unmarshal JSON (handles ints, bools, etc.). If it fails, keep as string.
				var jsonVal any
				if err := json.Unmarshal([]byte(value), &jsonVal); err == nil {
					inputArgs[name] = jsonVal
				} else {
					inputArgs[name] = value
				}
			}
		}

		// Look in attrs (e.g. command="...")
		if attrs != "" {
			attrRe := regexp.MustCompile(`([a-zA-Z0-9_\-]+)=(?:"([^"]*)"|'([^']*)')`)
			attrMatches := attrRe.FindAllStringSubmatch(attrs, -1)
			for _, am := range attrMatches {
				name := am[1]
				value := am[2]
				if value == "" {
					value = am[3]
				}
				if _, exists := inputArgs[name]; !exists {
					var jsonVal any
					if err := json.Unmarshal([]byte(value), &jsonVal); err == nil {
						inputArgs[name] = jsonVal
					} else {
						inputArgs[name] = value
					}
				}
			}
		}

		calls = append(calls, parsedToolCall{
			ID:    fmt.Sprintf("toolspec-%d", time.Now().UnixNano()),
			Name:  toolName,
			Input: inputArgs,
		})
	}
	return calls
}

// removeToolCallTags removes all <function=...>...</function>, <function=...>, <tool_call>...</tool_call> tags
// and returns the natural text parts (before, between, and after the tags).
func removeToolCallTags(text string) string {
	// Remove paired tags: <function...>...</function> or <tool_call>...</tool_call>
	rePaired := regexp.MustCompile(`(?s)<(function|tool_call)(?:\s+[^>]*)?>.*?</\1>`)
	cleaned := rePaired.ReplaceAllString(text, "")
	// Remove self-closing tags: <function...> or <tool_call...>
	reSelfClosing := regexp.MustCompile(`(?s)<(function|tool_call)(?:\s+[^>]*)?/>`)
	cleaned = reSelfClosing.ReplaceAllString(cleaned, "")
	// Remove opening tags without closing: <function=...> or <tool_call>
	reOpenTag := regexp.MustCompile(`(?s)<(function|tool_call)(?:\s+[^>]*)?>`)
	cleaned = reOpenTag.ReplaceAllString(cleaned, "")
	// Trim leading/trailing whitespace
	cleaned = strings.TrimSpace(cleaned)
	return cleaned
}

// extractTextBeforeToolUse removes any <function=...> or <tool_call> tags and returns only the natural text part.
// This is a wrapper for backward compatibility - it now preserves text before, between, and after tool call tags.
func extractTextBeforeToolUse(text string) string {
	return removeToolCallTags(text)
}

// extractToolCallsFromContent scans assistantContent for text blocks containing
// <function=...> tags, converts them to tool_use blocks, and cleans the text.
// It modifies assistantContent in-place and returns the extracted tool_use blocks.
// If cleaning leaves an empty text block, it replaces it with a placeholder so that
// the assistant message always contains some user-facing text.
func (qe *QueryEngine) extractToolCallsFromContent(assistantContent *[]api.ContentBlockParam) []api.ContentBlock {
	var toolUseBlocks []api.ContentBlock
	for i := 0; i < len(*assistantContent); i++ {
		block := (*assistantContent)[i]
		if block.Type == "text" {
			extracted := parseToolUseFromText(block.Text)
			if len(extracted) > 0 {
				// Convert to tool_use blocks
				for _, bc := range extracted {
					toolUseBlocks = append(toolUseBlocks, api.ContentBlock{
						Type:  "tool_use",
						ID:    bc.ID,
						Name:  bc.Name,
						Input: bc.Input,
					})
				}
				// Clean the text block
				cleaned := extractTextBeforeToolUse(block.Text)
				if cleaned != "" {
					(*assistantContent)[i].Text = cleaned
				} else {
					// Replace empty text with a placeholder so user sees something
					(*assistantContent)[i].Text = "(正在执行工具操作…)"
				}
			}
		}
	}
	return toolUseBlocks
}

// dumpMessageRequest prints the request details in verbose mode.
func (qe *QueryEngine) dumpMessageRequest(req *api.MessageRequest) {
	qe.logger.Infoln("═══════════════════════ LLM Request ═══════════════════════")
	qe.logger.Infof("Model:     %s", req.Model)
	qe.logger.Infof("MaxTokens: %d", req.MaxTokens)
	if req.Thinking != nil {
		qe.logger.Infof("Thinking:  %s (budget=%d)", req.Thinking.Type, req.Thinking.BudgetTokens)
	}
	qe.logger.Infof("Messages:  %d", len(req.Messages))

	// Print system prompt (truncated)
	switch sys := req.System.(type) {
	case string:
		qe.logger.Infof("\n--- System Prompt (%d chars) ---\n%s\n--- End System Prompt ---\n\n",
			len(sys), formatTruncated(sys, verboseLogLimit))
	case []api.SystemBlock:
		for i, block := range sys {
			qe.logger.Infof("\n--- System Block [%d] (%d chars) ---\n%s\n--- End System Block ---\n\n",
				i, len(block.Text), formatTruncated(block.Text, verboseLogLimit))
		}
	}

	// Print each message
	for i, msg := range req.Messages {
		contentStr := messageContentToString(msg.Content)
		qe.logger.Infof("[%d] role=%s (%d chars): %s",
			i, msg.Role, len(contentStr), formatTruncated(contentStr, verboseLogLimit))
	}

	// Print tool names
	if len(req.Tools) > 0 {
		var toolNames []string
		for _, t := range req.Tools {
			toolNames = append(toolNames, t.Name)
		}
		qe.logger.Infof("Tools: [%s]", strings.Join(toolNames, ", "))
	}

	// Print memory summary
	if ms := req.MemorySummary; ms != nil {
		flag := ""
		if ms.AutoMemPrompt {
			flag = " +auto-mem-prompt"
		}
		qe.logger.Infof("Memory:         %d files%s", ms.TotalFiles, flag)
		if len(ms.ClaudeMDFiles) > 0 {
			qe.logger.Infof("  ClaudeMDFiles:  [%s]", strings.Join(ms.ClaudeMDFiles, ", "))
		}
		if len(ms.SemanticHits) > 0 {
			qe.logger.Infof("  SemanticHits:   [%s]", strings.Join(ms.SemanticHits, ", "))
		}
	}

	qe.logger.Infoln("═══════════════════════ End Request ═══════════════════════")
}

// dumpMessageResponse prints the response details in verbose mode.
func (qe *QueryEngine) dumpMessageResponse(resp *api.MessageResponse) {
	qe.logger.Infoln("═══════════════════════ LLM Response ══════════════════════")
	qe.logger.Infof("ID:         %s", resp.ID)
	qe.logger.Infof("Model:      %s", resp.Model)
	qe.logger.Infof("StopReason: %s", resp.StopReason)
	qe.logger.Infof("Usage:      input=%d output=%d cache_create=%d cache_read=%d",
		resp.Usage.InputTokens, resp.Usage.OutputTokens,
		resp.Usage.CacheCreationInputTokens, resp.Usage.CacheReadInputTokens)

	for i, block := range resp.Content {
		switch block.Type {
		case "text":
			qe.logger.Infof("\n--- Content [%d] type=text (%d chars) ---\n%s\n--- End Content ---\n\n",
				i, len(block.Text), formatTruncated(block.Text, verboseLogLimit))
		case "tool_use":
			inputJSON, _ := json.Marshal(block.Input)
			qe.logger.Infof("[%d] type=tool_use name=%s id=%s input=%s",
				i, block.Name, block.ID, formatTruncated(string(inputJSON), verboseLogLimit))
		case "thinking":
			if qe.showThinkingInLog {
				qe.logger.Infof("\n--- Content [%d] type=thinking (%d chars) ---\n%s\n--- End Content ---\n\n",
					i, len(block.Text), formatTruncated(block.Text, verboseLogLimit))
			} else {
				qe.logger.Infof("[%d] type=thinking (%d chars, hidden)", i, len(block.Text))
			}
		default:
			qe.logger.Infof("[%d] type=%s", i, block.Type)
		}
	}
	qe.logger.Infoln("═══════════════════════ End Response ══════════════════════")
}

// messageContentToString converts a message content field to a string for display.
func messageContentToString(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []api.ContentBlockParam:
		var parts []string
		for _, b := range v {
			switch b.Type {
			case "text":
				parts = append(parts, b.Text)
			case "tool_use":
				inputJSON, _ := json.Marshal(b.Input)
				parts = append(parts, fmt.Sprintf("[tool_use:%s]", b.Name))
				_ = inputJSON
			case "tool_result":
				if textBlocks, ok := b.Content.([]api.ContentBlockParam); ok {
					for _, tb := range textBlocks {
						if tb.Type == "text" {
							parts = append(parts, fmt.Sprintf("[tool_result:%s]", formatTruncated(tb.Text, 200)))
						}
					}
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprintf("%v", content)
	}
}

// SubmitMessage processes a user message and runs the tool call loop
func (qe *QueryEngine) SubmitMessage(ctx context.Context, prompt string) error {
	qe.SetProcessing(true)
	defer qe.SetProcessing(false)

	// Clear previous assistant text for this new turn
	qe.lastAssistantText = ""

	// Check if this is a slash command BEFORE triggering any LLM operations (like memory initialization)
	if slash.IsSlashCommand(prompt) {
		err := qe.handleSlashCommand(ctx, prompt)
		// Fire TextCallback so channels (Weixin, QQ, etc.) receive slash command output
		if qe.TextCallback != nil && qe.lastAssistantText != "" {
			qe.TextCallback(qe.lastAssistantText)
		}
		return err
	}

	// Start heartbeat monitoring if enabled
	if qe.IsHeartbeatEnabled() {
		qe.StartHeartbeat(ctx)
		defer qe.StopHeartbeat()
	}

	// One-time memory initialization (semantic index + compaction)
	qe.initMemoryIndex(ctx)
	qe.tryCompactMemory(ctx)

	// Add to history
	qe.historyMgr.AddSimpleHistory(prompt)

	// Add user message
	userMsg := api.MessageParam{
		Role:    "user",
		Content: prompt,
	}
	qe.messages = append(qe.messages, userMsg)

	// Record to transcript
	qe.RecordMessageToTranscript(transcript.MessageTypeUser, "user", []byte(prompt))

	// Update heartbeat after user message
	qe.UpdateHeartbeat()

	// Reset turn counter for per-query budget
	qe.resetForNewQuery()

	// Main query loop
	timeoutRecoveryCount := 0
	for qe.currentTurn < qe.effectiveMaxTurns() {
		qe.currentTurn++

		// Query limit grace mode: if we've exceeded the per-query budget
		// (currentTurn > queryMaxTurns but < effectiveMaxTurns which includes +1),
		// inject a system-directing user message asking for a summary.
		if qe.queryMaxTurns > 0 && qe.currentTurn > qe.queryMaxTurns && !qe.queryLimitGraceMode {
			qe.queryLimitGraceMode = true
			qe.logger.Infof("[⏱️ Query reached max turns (%d) — requesting summary turn]", qe.queryMaxTurns)
			qe.messages = append(qe.messages, api.MessageParam{
				Role:    "user",
				Content: "You have reached the maximum number of turns for this query. Please do NOT make any more tool calls. Instead, provide a concise summary of your findings and conclusions based on the information gathered so far.",
			})
		}

		// After grace turn, if the model still calls tools, stop immediately
		if qe.queryMaxTurns > 0 && qe.queryLimitGraceMode && qe.currentTurn > qe.queryMaxTurns+1 {
			if qe.verbose {
				qe.logger.Debug("[Grace turn exceeded — forcing stop]")
			}
			return fmt.Errorf("reached maximum turns for query (%d)", qe.queryMaxTurns)
		}

		maxTurnLabel := qe.effectiveMaxTurns()
		if qe.verbose {
			qe.logger.Debugf("[Turn %d/%d]", qe.currentTurn, maxTurnLabel)
		}

		// Check if auto-compact is needed
		if qe.compactConfig.Enabled {
			shouldCompact, tokenCount, threshold := compact.CheckAutoCompact(qe.messages, qe.compactConfig, qe.compactTracker)
			if shouldCompact {
				if qe.verbose {
					qe.logger.Debugf("[Auto-compact triggered: %d tokens >= threshold %d]", tokenCount, threshold)
				}
				result, err := compact.CompactMessages(ctx, qe.client, qe.messages, qe.systemPrompt, qe.compactConfig)
				if err != nil {
					qe.logger.Errorf("[Auto-compact error: %v]", err)
				} else if result != nil {
					qe.messages = compact.ApplyCompactResult(qe.messages, result)
					qe.compactTracker.Compacted = true
					qe.compactTracker.TurnCounter++
					if qe.verbose {
						qe.logger.Debugf("[Auto-compact complete: %d -> %d messages, %d -> %d tokens]",
							result.OriginalMessageCount, result.CompactedMessageCount,
							result.PreCompactTokenCount, result.PostCompactTokenCount)
					}
				}
			} else {
				// Check for warning state
				warning, isBlocking := compact.GetWarningState(tokenCount, qe.compactConfig)
				if warning != "" {
					qe.logger.Warn(warning)
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
					qe.logger.Debugf("[Snip: removed %d messages, %d remaining]",
						snipResult.SnippedCount, len(snipResult.Remaining))
				}
				qe.messages = snipResult.Remaining
			}
		}

		// Build full system prompt with context
		fullSystemPrompt, memSummary := qe.buildFullSystemPrompt()

		// Build API request
		req := &api.MessageRequest{
			Model:         qe.client.Model,
			MaxTokens:     qe.maxTokens,
			System:        fullSystemPrompt,
			Messages:      qe.messages,
			Tools:         qe.toAPITools(),
			MemorySummary: memSummary,
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
		// Dump request/response in verbose mode
		if qe.verbose {
			qe.dumpMessageRequest(req)
			qe.logger.Debug("dumpMessageRequest1")
		}

		// Update heartbeat before API call
		qe.UpdateHeartbeat()

		// Call API
		resp, err := qe.client.SendMessage(ctx, req)
		if err == nil && qe.verbose {
			qe.dumpMessageResponse(resp)
			qe.logger.Debug("dumpMessageResponse1")
		}
		if err != nil {
			// Update heartbeat on error (still activity)
			qe.UpdateHeartbeat()
			// Handle context deadline exceeded (timeout) — retry with compacted messages
			if isTimeoutError(err) {
				recovered, retryErr := qe.tryRecoverFromTimeout(ctx, err)
				if recovered {
					timeoutRecoveryCount++
					if timeoutRecoveryCount > 2 {
						qe.logger.Warn("[⚠️  Timeout recovery exhausted — giving up after multiple attempts]")
						qe.FlushTranscript()
						return fmt.Errorf("API timeout unrecoverable after multiple recovery attempts: %w", err)
					}
					qe.logger.Warnf("[⏱️  Recovered from timeout (attempt %d/2) — retrying with reduced context]", timeoutRecoveryCount)
					time.Sleep(500 * time.Millisecond) // Safety delay to prevent tight-loop bursts
					continue
				}
				if retryErr != nil {
					qe.FlushTranscript()
					return retryErr
				}
				// Recovery failed (snip/compact couldn't help).
				// Flush transcript and return gracefully so user can /resume.
				qe.FlushTranscript()
				if len(qe.messages) > 0 {
					qe.logger.Warn("[⚠️  超时无法自动恢复，已保存当前会话快照，稍后可使用 /resume 恢复]")
					return nil
				}
				return fmt.Errorf("API timeout unrecoverable: %w", err)
			}
			// Try to recover from context length exceeded errors
			recovered, recoveryErr := qe.tryRecoverFromContextExceeded(ctx, err)
			if recovered {
				qe.logger.Warn("[🔄 Recovered from context length exceeded error]")
				time.Sleep(500 * time.Millisecond) // Safety delay to prevent tight-loop bursts
				continue                           // Retry with compacted messages
			}
			if recoveryErr != nil {
				return recoveryErr
			}
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

		// Update heartbeat after receiving response
		qe.UpdateHeartbeat()

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
					qe.logger.Infof("[🧠 Thinking (%d chars)]\n%s\n[End Thinking]", len(block.Text), block.Text)
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

		// Fallback: if no tool_use blocks but text contains <function=...> tags,
		// parse them, create tool_use blocks, and clean the text.
		if len(toolUseBlocks) == 0 {
			// Use in-place extraction which also cleans the text
			toolUseBlocks = qe.extractToolCallsFromContent(&assistantContent)
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
					qe.logger.Info(detail)
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

		// Record assistant message to transcript AND update lastAssistantText for channels
		if len(assistantContent) > 0 {
			var textParts []string
			hasToolUse := false
			for _, block := range assistantContent {
				if block.Type == "text" && block.Text != "" {
					textParts = append(textParts, block.Text)
				}
				if block.Type == "tool_use" {
					hasToolUse = true
				}
			}

			if len(textParts) > 0 {
				userText := strings.Join(textParts, "\n\n")
				// Update cache for channel retrieval
				qe.lastAssistantText = userText
				// Record to transcript
				qe.RecordMessageToTranscript(transcript.MessageTypeAssistant, "assistant", []byte(userText))
				// If this is an intermediate turn (text + tool calls together), push text
				// to channels immediately via TextCallback so users see LLM commentary
				// in real-time rather than waiting for the full loop to finish.
				if hasToolUse && qe.TextCallback != nil {
					qe.TextCallback(userText)
				}
			} else if hasToolUse {
				// Tool-only turn: no text to show, leave lastAssistantText as-is
				// (do not overwrite with a placeholder — the real final text will come later)
			}
		}

		if len(toolUseBlocks) == 0 {
			// Final turn: no tool calls — capture text and notify channel
			var finalTextParts []string
			for _, block := range assistantContent {
				if block.Type == "text" && block.Text != "" {
					finalTextParts = append(finalTextParts, block.Text)
				}
			}
			if len(finalTextParts) > 0 {
				qe.lastAssistantText = strings.Join(finalTextParts, "\n\n")
				// TextCallback for the final reply: only fire if it wasn't already
				// sent during this same turn's text+tool handling above.
				// Since len(toolUseBlocks)==0 here, this IS the text-only final turn —
				// fire the callback so channels receive it even inside multi-turn loops.
				if qe.TextCallback != nil {
					qe.TextCallback(qe.lastAssistantText)
				}
			} else {
				// LLM returned no text at all (e.g. only thinking blocks)
				qe.lastAssistantText = "（模型未返回文字内容）"
			}

			if qe.verbose {
				qe.logger.Debug("[Response complete]")
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
				qe.logger.Debugf("[Tool call: %s (id=%s)]", toolName, toolUseID)
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
				qe.RecordToolResultToTranscript(toolUseID, toolName, "Invalid tool input", true)
				continue
			}

			// Record tool call to transcript
			qe.RecordToolCallToTranscript(toolUseID, toolName, inputMap)

			// Update heartbeat before tool execution
			qe.UpdateHeartbeat()

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
				qe.RecordToolResultToTranscript(toolUseID, toolName, err.Error(), true)
				continue
			}

			// Update heartbeat after tool execution
			qe.UpdateHeartbeat()

			// Log tool result summary
			resultStr, _ := json.Marshal(result.Data)
			if qe.verbose {
				preview := string(resultStr)
				if len(preview) > 200 {
					preview = preview[:200] + "..."
				}
				qe.logger.Debugf("  ✅ Result: %s", preview)
			}

			// Collect tool result for optional reply
			status := "✅"
			if result.IsError {
				status = "❌"
			}
			toolResults = append(toolResults, fmt.Sprintf("- **%s**: %s", toolName, status))

			// Add tool result
			qe.addToolResult(toolUseID, string(resultStr), result.IsError)

			// Update heartbeat after adding tool result
			qe.UpdateHeartbeat()
		}

		// If showToolUsageInReply is enabled, append tool usage summary to the cache for channels
		// DO NOT modify assistantContent directly as it will pollute the LLM message history context.
		if qe.showToolUsageInReply && len(toolUseBlocks) > 0 {
			summary := "\n\n---\n**🔧 Tool Usage:**\n" + strings.Join(toolResults, "\n")
			if qe.lastAssistantText == "(正在执行工具操作…)" || qe.lastAssistantText == "" {
				qe.lastAssistantText = "**🔧 Tool Usage:**\n" + strings.Join(toolResults, "\n")
			} else {
				qe.lastAssistantText += summary
			}
		}
	}

	return fmt.Errorf("reached maximum turns (%d)", qe.effectiveMaxTurns())
}

// RunMainLoop exposes the internal tool-call loop for external channels.
// It assumes the caller has already added a user message to qe.messages
// and will run the turn loop until a text-only response is returned,
// the budget/turn limit is hit, or a hard reset occurs.
func (qe *QueryEngine) RunMainLoop(ctx context.Context) error {
	qe.SetProcessing(true)
	defer qe.SetProcessing(false)

	// Clear any previous assistant text for this session
	qe.lastAssistantText = ""

	// Ensure API client can notify us of activity (heartbeat)
	if qe.client != nil {
		qe.client.OnActivity = qe.UpdateHeartbeat
	}

	// Start heartbeat monitoring. 
	// We enable it by default during RunMainLoop unless the user explicitly 
	// wants it off, but based on feedback it should be active.
	qe.SetHeartbeatEnabled(true)
	qe.StartHeartbeat(ctx)
	defer qe.StopHeartbeat()

	// One-time memory initialization (semantic index + compaction)
	qe.initMemoryIndex(ctx)
	qe.tryCompactMemory(ctx)

	// Main query loop
	for qe.currentTurn < qe.maxTurns {
		qe.currentTurn++

		// Update heartbeat at start of each turn
		qe.UpdateHeartbeat()

		if qe.verbose {
			qe.logger.Infof("[Turn %d/%d]", qe.currentTurn, qe.maxTurns)
		}

		// Check if auto-compact is needed
		if qe.compactConfig.Enabled {
			shouldCompact, tokenCount, threshold := compact.CheckAutoCompact(qe.messages, qe.compactConfig, qe.compactTracker)
			if shouldCompact {
				if qe.verbose {
					qe.logger.Infof("[Auto-compact triggered: %d tokens >= threshold %d]", tokenCount, threshold)
				}
				result, err := compact.CompactMessages(ctx, qe.client, qe.messages, qe.systemPrompt, qe.compactConfig)
				if err != nil {
					qe.logger.Infof("[Auto-compact error: %v]", err)
				} else if result != nil {
					qe.messages = compact.ApplyCompactResult(qe.messages, result)
					qe.compactTracker.Compacted = true
					qe.compactTracker.TurnCounter++
					if qe.verbose {
						qe.logger.Debugf("[Auto-compact complete: %d -> %d messages, %d -> %d tokens]",
							result.OriginalMessageCount, result.CompactedMessageCount,
							result.PreCompactTokenCount, result.PostCompactTokenCount)
					}
				}
			} else {
				warning, isBlocking := compact.GetWarningState(tokenCount, qe.compactConfig)
				if warning != "" {
					qe.logger.Info(warning)
				}
				if isBlocking {
					return fmt.Errorf("context window is full (blocking limit reached). Please start a new conversation.")
				}
			}
		}

		// Check if snip is needed
		if qe.snipConfig.Enabled {
			snipResult := compact.SnipHistory(qe.messages, qe.snipConfig)
			if snipResult != nil {
				if qe.verbose {
					qe.logger.Debugf("[Snip: removed %d messages, %d remaining]",
						snipResult.SnippedCount, len(snipResult.Remaining))
				}
				qe.messages = snipResult.Remaining
			}
		}

		// Build API request
		fullSystemPrompt, memSummary := qe.buildFullSystemPrompt()
		req := &api.MessageRequest{
			Model:         qe.client.Model,
			MaxTokens:     qe.maxTokens,
			System:        fullSystemPrompt,
			Messages:      qe.messages,
			Tools:         qe.toAPITools(),
			MemorySummary: memSummary,
		}

		// Configure thinking
		if qe.thinkingConfig.Enabled {
			if qe.thinkingConfig.Type == "adaptive" {
				req.Thinking = &api.ThinkingConfig{Type: "enabled"}
			} else {
				req.Thinking = &api.ThinkingConfig{
					Type:         "enabled",
					BudgetTokens: qe.thinkingConfig.BudgetTokens,
				}
			}
		} else {
			req.Thinking = &api.ThinkingConfig{Type: "disabled"}
		}

		if qe.fastModeManager.IsActive() {
			req.Model = qe.fastModeManager.GetModel()
		}

		if qe.verbose {
			qe.dumpMessageRequest(req)
			logger.Debug("dumpMessageRequest2")
		}

		// Update heartbeat before API call
		qe.UpdateHeartbeat()

		resp, err := qe.client.SendMessage(ctx, req)
		if err == nil && qe.verbose {
			qe.dumpMessageResponse(resp)
			logger.Debug("dumpMessageResponse2")
		}
		if err != nil {
			// Update heartbeat on error
			qe.UpdateHeartbeat()
			// Handle context deadline exceeded (timeout) — retry with compacted messages
			if isTimeoutError(err) {
				recovered, retryErr := qe.tryRecoverFromTimeout(ctx, err)
				if recovered {
					qe.logger.Infof("[⏱️  Recovered from timeout — retrying with reduced context]\n")
					continue
				}
				if retryErr != nil {
					qe.FlushTranscript()
					return retryErr
				}
				// Recovery failed (snip/compact couldn't help).
				// Flush transcript and return gracefully so user can /resume.
				qe.FlushTranscript()
				if len(qe.messages) > 0 {
					qe.logger.Infof("[⚠️  超时无法自动恢复，已保存当前会话快照，稍后可使用 /resume 恢复]\n")
					return nil
				}
				return fmt.Errorf("API timeout unrecoverable: %w", err)
			}
			recovered, recoveryErr := qe.tryRecoverFromContextExceeded(ctx, err)
			if recovered {
				qe.logger.Infof("[🔄 Recovered from context length exceeded error]\n")
				continue
			}
			if recoveryErr != nil {
				return recoveryErr
			}
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

		// Update heartbeat after receiving response
		qe.UpdateHeartbeat()

		var assistantContent []api.ContentBlockParam

		for _, block := range resp.Content {
			if block.Type == "text" && block.Text != "" {
				assistantContent = append(assistantContent, api.ContentBlockParam{
					Type: "text",
					Text: block.Text,
				})
			}
			if block.Type == "thinking" && block.Text != "" {
				if qe.showThinkingInLog {
					qe.logger.Infof("[🧠 Thinking (%d chars)]\n%s\n[End Thinking]", len(block.Text), block.Text)
				}
			}
		}

		var toolUseBlocks []api.ContentBlock
		for _, block := range resp.Content {
			if block.Type == "tool_use" {
				toolUseBlocks = append(toolUseBlocks, block)
			}
		}

		if len(toolUseBlocks) > 0 {
			for _, block := range toolUseBlocks {
				assistantContent = append(assistantContent, api.ContentBlockParam{
					Type:  "tool_use",
					ID:    block.ID,
					Name:  block.Name,
					Input: block.Input,
				})
			}

			if qe.verbose {
				for _, block := range toolUseBlocks {
					inputJSON, _ := json.Marshal(block.Input)
					qe.logger.Infof("  📦 %s (id=%s): %s", block.Name, block.ID, string(inputJSON))
				}
			}
		}

		if len(assistantContent) > 0 {
			qe.messages = append(qe.messages, api.MessageParam{
				Role:    "assistant",
				Content: assistantContent,
			})
		}

		// Record assistant message to transcript AND update lastAssistantText for channels
		if len(assistantContent) > 0 {
			var textParts []string
			hasToolUse := false
			for _, block := range assistantContent {
				if block.Type == "text" && block.Text != "" {
					textParts = append(textParts, block.Text)
				}
				if block.Type == "tool_use" {
					hasToolUse = true
				}
			}

			// Determine user-facing text
			var userText string
			if len(textParts) > 0 {
				userText = strings.Join(textParts, "\n\n")
			} else if hasToolUse {
				userText = "(正在执行工具操作…)"
			}
			// Update cache for channel retrieval
			if userText != "" {
				qe.lastAssistantText = userText
			}
			// Record to transcript
			if userText != "" {
				qe.RecordMessageToTranscript(transcript.MessageTypeAssistant, "assistant", []byte(userText))
			}
		}

		if len(toolUseBlocks) == 0 {
			// No tools - reply already cached above
			if qe.verbose {
				qe.logger.Debug("[Response complete]")
			}
			return nil
		}

		qe.LastTurnToolCalls = nil

		var toolResults []string
		for _, block := range toolUseBlocks {
			toolName := block.Name
			toolInput := block.Input
			toolUseID := block.ID

			if qe.verbose {
				qe.logger.Debugf("[Tool call: %s (id=%s)]", toolName, toolUseID)
			}

			summary := buildToolCallSummary(toolName, toolInput)
			inputJSON, _ := json.Marshal(toolInput)

			qe.LastTurnToolCalls = append(qe.LastTurnToolCalls, ToolCallInfo{
				Name:    toolName,
				Input:   string(inputJSON),
				Summary: summary,
			})

			if qe.ToolCallCallback != nil {
				qe.ToolCallCallback(toolName, summary)
			}

			tool := qe.findTool(toolName)
			if tool == nil {
				qe.addToolResult(toolUseID, fmt.Sprintf("Error: Unknown tool '%s'", toolName), true)
				toolResults = append(toolResults, fmt.Sprintf("- **%s**: ❌ Unknown tool", toolName))
				// Record tool result to transcript
				qe.RecordToolResultToTranscript(toolUseID, toolName, fmt.Sprintf("Unknown tool '%s'", toolName), true)
				continue
			}

			inputMap, ok := toolInput.(map[string]any)
			if !ok {
				qe.addToolResult(toolUseID, "Error: Invalid tool input", true)
				toolResults = append(toolResults, fmt.Sprintf("- **%s**: ❌ Invalid input", toolName))
				// Record tool result to transcript
				qe.RecordToolResultToTranscript(toolUseID, toolName, "Invalid tool input", true)
				continue
			}

			// Record tool call to transcript
			qe.RecordToolCallToTranscript(toolUseID, toolName, inputMap)

			toolCtx := types.ToolUseContext{
				Cwd:             qe.cwd,
				AbortController: ctx,
				Tools:           qe.tools,
			}

			result, err := tool.Call(ctx, inputMap, toolCtx, nil)
			// Update heartbeat immediately after tool returns
			qe.UpdateHeartbeat()

			if err != nil {
				qe.addToolResult(toolUseID, fmt.Sprintf("Error: %v", err), true)
				toolResults = append(toolResults, fmt.Sprintf("- **%s**: ❌ Error: %v", toolName, err))
				// Record tool result to transcript
				qe.RecordToolResultToTranscript(toolUseID, toolName, err.Error(), true)
				continue
			}

			resultStr, _ := json.Marshal(result.Data)
			if qe.verbose {
				preview := string(resultStr)
				if len(preview) > 200 {
					preview = preview[:200] + "..."
				}
				qe.logger.Infof("  ✅ Result: %s", preview)
			}

			status := "✅"
			if result.IsError {
				status = "❌"
			}
			toolResults = append(toolResults, fmt.Sprintf("- **%s**: %s", toolName, status))
			qe.addToolResult(toolUseID, string(resultStr), result.IsError)
			// Record tool result to transcript
			qe.RecordToolResultToTranscript(toolUseID, toolName, string(resultStr), result.IsError)
		}

		if qe.showToolUsageInReply && len(toolUseBlocks) > 0 {
			summary := "\n\n---\n**🔧 Tool Usage:**\n" + strings.Join(toolResults, "\n")
			if qe.lastAssistantText == "(正在执行工具操作…)" || qe.lastAssistantText == "" {
				qe.lastAssistantText = "**🔧 Tool Usage:**\n" + strings.Join(toolResults, "\n")
			} else {
				qe.lastAssistantText += summary
			}
		}
	}

	return fmt.Errorf("reached maximum turns (%d)", qe.maxTurns)
}

// buildFullSystemPrompt combines base system prompt with context (git status, AGENT.md, etc.)
// Returns the prompt string and a MemorySummary of what was loaded.
func (qe *QueryEngine) buildFullSystemPrompt() (string, *api.MemorySummary) {
	var parts []string
	summary := &api.MemorySummary{}

	if qe.systemPrompt != "" {
		parts = append(parts, qe.systemPrompt)
	}

	skillsSection := qe.skillRegistry.FormatSkillsForPrompt()
	if skillsSection != "" {
		parts = append(parts, skillsSection)
	}

	memoryFiles, err := claudemd.GetMemoryFiles(qe.cwd)
	if err == nil {
		for _, f := range memoryFiles {
			summary.ClaudeMDFiles = append(summary.ClaudeMDFiles, f.Path)
		}
		summary.TotalFiles += len(memoryFiles)
		agentMdContext := claudemd.BuildAgentMdContext(memoryFiles)
		if agentMdContext != "" {
			parts = append(parts, agentMdContext)
		}
	}

	// Inject memory system prompt and relevant memories
	if qe.memoryIndex != nil {
		if qe.autoMemoryPrompt == "" {
			qe.autoMemoryPrompt = memory.BuildMemoryPrompt(memory.PromptConfig{
				DisplayName: "Auto Memory",
				MemoryDir:   qe.memoryDir,
			})
		}
		if qe.autoMemoryPrompt != "" {
			summary.AutoMemPrompt = true
			parts = append(parts, qe.autoMemoryPrompt)
		}

		// Search relevant memories based on recent conversation
		if len(qe.messages) > 0 {
			lastUserMsg := qe.findLastUserMessageText()
			if lastUserMsg != "" {
				results := qe.memoryIndex.Search(lastUserMsg, 5)
				if len(results) > 0 {
					var relCtx strings.Builder
					for _, r := range results {
						summary.SemanticHits = append(summary.SemanticHits, r.Entry.Name)
						summary.TotalFiles++
						relCtx.WriteString(fmt.Sprintf("### %s\n%s\n\n", r.Entry.Name, r.Entry.Description))
					}
					parts = append(parts, "## 相关记忆\n\n"+relCtx.String())
				}
			}
		}
	}

	sysCtx := ctxpkg.GetSystemContext()
	if sysCtx.GitStatus != "" {
		parts = append(parts, "## Git Status\n\n"+sysCtx.GitStatus)
	}
	parts = append(parts, sysCtx.CurrentDate)

	return strings.Join(parts, "\n\n"), summary
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

// findLastUserMessageText finds and returns the last user message text
func (qe *QueryEngine) findLastUserMessageText() string {
	for i := len(qe.messages) - 1; i >= 0; i-- {
		msg := qe.messages[i]
		if msg.Role == "user" {
			// Simple message
			if text, ok := msg.Content.(string); ok && text != "" {
				return text
			}
			// Array content blocks
			if blocks, ok := msg.Content.([]api.ContentBlockParam); ok {
				for _, block := range blocks {
					if block.Type == "text" && block.Text != "" {
						return block.Text
					}
				}
			}
		}
	}
	return ""
}

// GetLastAssistantText returns the last assistant's text response.
// It first checks the cached lastAssistantText (set at end of turn),
// then falls back to scanning qe.messages jeśli not set.
func (qe *QueryEngine) GetLastAssistantText() string {
	// Prefer cached value (set at turn completion)
	if qe.lastAssistantText != "" {
		return qe.lastAssistantText
	}
	// Fallback: scan message history
	for i := len(qe.messages) - 1; i >= 0; i-- {
		msg := qe.messages[i]
		if msg.Role == "assistant" {
			if blocks, ok := msg.Content.([]api.ContentBlockParam); ok {
				var texts []string
				hasToolUse := false
				for _, block := range blocks {
					if block.Type == "text" && block.Text != "" {
						texts = append(texts, block.Text)
					}
					if block.Type == "tool_use" {
						hasToolUse = true
					}
				}
				if len(texts) > 0 {
					return strings.Join(texts, "\n")
				}
				if hasToolUse {
					return "(正在执行工具操作…)"
				}
			}
			if text, ok := msg.Content.(string); ok && text != "" {
				return text
			}
		}
	}
	return ""
}

// tryRecoverFromContextExceeded handles context_length_exceeded errors with a
// tiered recovery strategy:
//
//	Tier 1 – aggressive snip (keep last N messages)
//	Tier 2 – LLM-assisted compact (summarise old messages into one summary)
//	Tier 3 – hard reset (only keep the very last user message)
//
// Returns (recovered=true, nil) on success, (recovered=false, err) on failure.
func (qe *QueryEngine) tryRecoverFromContextExceeded(ctx context.Context, err error) (bool, error) {
	var ctxErr *api.ContextLengthExceededError
	if !errors.As(err, &ctxErr) {
		return false, nil // not a context-length error
	}

	qe.logger.Infof("[⚠️  Context length exceeded detected (HTTP %d)]", ctxErr.StatusCode)

	// --- Tier 1: Aggressive snip ---
	tier1Preserve := 4
	if len(qe.messages) > tier1Preserve {
		if qe.verbose {
			qe.logger.Debugf("[🔄 Recovery Tier 1: Aggressive snip – keeping last %d messages (%d total)",
				tier1Preserve, len(qe.messages))
		}
		qe.messages = qe.messages[len(qe.messages)-tier1Preserve:]
		return true, nil
	}

	// --- Tier 2: LLM-assisted compact ---
	if qe.compactConfig.Enabled && len(qe.messages) >= 4 {
		if qe.verbose {
			qe.logger.Infof("[🔄 Recovery Tier 2: LLM-assisted compact with fallback preserve=2]\n")
		}
		fallbackConfig := &compact.AutoCompactConfig{
			Enabled:            qe.compactConfig.Enabled,
			ThresholdRatio:     0.50,
			WarningRatio:       qe.compactConfig.WarningRatio,
			MaxContextTokens:   qe.compactConfig.MaxContextTokens,
			ModelContextWindow: qe.compactConfig.ModelContextWindow,
		}

		result, compactErr := compact.CompactMessages(ctx, qe.client, qe.messages, qe.systemPrompt, fallbackConfig)
		if compactErr == nil && result != nil {
			qe.messages = compact.ApplyCompactResult(qe.messages, result)
			qe.compactTracker.Compacted = true
			if qe.verbose {
				qe.logger.Debugf("[✅ Compact recovery succeeded: %d → %d messages, %d → %d tokens]",
					result.OriginalMessageCount, result.CompactedMessageCount,
					result.PreCompactTokenCount, result.PostCompactTokenCount)
			}
			return true, nil
		}
	}

	// --- Tier 3: Hard reset ---
	if qe.verbose {
		qe.logger.Infof("[🔄 Recovery Tier 3: Hard reset – dropping conversation history]\n")
	}
	if len(qe.messages) >= 2 {
		lastMsg := qe.messages[len(qe.messages)-1]
		if lastMsg.Role == "user" {
			qe.messages = []api.MessageParam{lastMsg}
		} else {
			qe.messages = make([]api.MessageParam, 0)
		}
	} else {
		qe.messages = make([]api.MessageParam, 0)
	}
	// Hard reset succeeded, return recovered=true to let caller retry
	return true, nil
}

// isTimeoutError checks if an error is a context deadline exceeded / timeout error
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	var deadlineErr *api.ContextDeadlineExceededError
	if errors.As(err, &deadlineErr) {
		return true
	}
	// Also check generic context.DeadlineExceeded
	return errors.Is(err, context.DeadlineExceeded)
}

// tryRecoverFromTimeout handles context deadline exceeded errors by reducing
// context size (snip/compact) and allowing the caller to retry.
//
// Recovery strategy:
//   - Try snip first (fast, no API call needed)
//   - If snip isn't viable and compact is enabled, try compact
//   - If both fail or aren't applicable, return unrecoverable
//
// Returns (recovered=true, nil) on success, (recovered=false, err) on failure.
func (qe *QueryEngine) tryRecoverFromTimeout(ctx context.Context, err error) (bool, error) {
	// Check context first — if it's already done, recovery is impossible
	if err := ctx.Err(); err != nil {
		qe.logger.Warn("[⏱️  Task aborted: Upstream context timeout reached during recovery. This often happens due to API rate limits (HTTP 429) exhausting the limited task duration.]")
		return false, err
	}

	qe.logger.Infof("[⏱️  Request timeout detected, attempting context reduction recovery]\n")

	if qe.verbose {
		qe.logger.Infof("[Current context: %d messages]", len(qe.messages))
	}

	// Step 0: Handle zero-message timeouts (nothing to snip/compact)
	if len(qe.messages) == 0 {
		qe.logger.Debug("[⚠️  Timeout on empty context — nothing to reduce]")
		return false, nil
	}

	// Step 1: Aggressive snip — keep only last few messages
	preserveCount := 5
	if len(qe.messages) > preserveCount {
		if qe.verbose {
			qe.logger.Debugf("[🔄 Timeout recovery: Snip — keeping last %d messages (%d total)",
				preserveCount, len(qe.messages))
		}
		qe.messages = qe.messages[len(qe.messages)-preserveCount:]
		return true, nil
	}

	// Step 2: If snip didn't remove enough, try compact (if feasible)
	if qe.compactConfig.Enabled && len(qe.messages) >= 6 {
		if qe.verbose {
			qe.logger.Infof("[🔄 Timeout recovery: LLM-assisted compact (note: this requires another API call)]\n")
		}
		// For timeout recovery, we use a more aggressive compact config
		// This compact attempt may itself timeout, so we use a shorter deadline
		compactCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		result, compactErr := compact.CompactMessages(compactCtx, qe.client, qe.messages, qe.systemPrompt, qe.compactConfig)
		if compactErr == nil && result != nil {
			qe.messages = compact.ApplyCompactResult(qe.messages, result)
			qe.compactTracker.Compacted = true
			if qe.verbose {
				qe.logger.Debugf("[✅ Compact recovery succeeded: %d → %d messages]",
					result.OriginalMessageCount, result.CompactedMessageCount)
			}
			return true, nil
		}
		if qe.verbose {
			qe.logger.Debugf("[⚠️  Compact recovery failed: %v]", compactErr)
		}
	}

	// Step 3: Hard reset - drop conversation history, keep only last user message if present
	// This guarantees recovery so the turn can retry with fresh context.
	if qe.verbose {
		qe.logger.Infof("[🔄 Timeout recovery: Hard reset – clearing conversation history]\n")
	}
	if len(qe.messages) >= 2 {
		lastMsg := qe.messages[len(qe.messages)-1]
		if lastMsg.Role == "user" {
			qe.messages = []api.MessageParam{lastMsg}
		} else {
			qe.messages = make([]api.MessageParam, 0)
		}
	} else {
		qe.messages = make([]api.MessageParam, 0)
	}
	return true, nil
}

// buildToolCallSummary creates a human-readable summary of a tool call with Markdown formatting.
func buildToolCallSummary(toolName string, input any) string {
	inputMap, ok := input.(map[string]any)
	if !ok {
		return toolName
	}

	var sb strings.Builder
	sb.WriteString("**")
	sb.WriteString(toolName)
	sb.WriteString("**\n")

	switch toolName {
	case "Read":
		if path, ok := inputMap["file_path"].(string); ok {
			sb.WriteString("读取文件：`")
			sb.WriteString(path)
			sb.WriteString("`")
			return sb.String()
		}
	case "Write":
		if path, ok := inputMap["file_path"].(string); ok {
			sb.WriteString("写入文件：`")
			sb.WriteString(path)
			sb.WriteString("`")
			if content, ok := inputMap["content"].(string); ok && len(content) > 0 {
				sb.WriteString("\n内容：\n```\n")
				sb.WriteString(truncateString(content, 200))
				sb.WriteString("\n```")
			}
			return sb.String()
		}
	case "Edit":
		if path, ok := inputMap["file_path"].(string); ok {
			sb.WriteString("编辑文件：`")
			sb.WriteString(path)
			sb.WriteString("`")
			if oldStr, ok := inputMap["old_str"].(string); ok && len(oldStr) > 0 {
				sb.WriteString("\n原文本：\n```\n")
				sb.WriteString(truncateString(oldStr, 100))
				sb.WriteString("\n```")
			}
			if newStr, ok := inputMap["new_str"].(string); ok && len(newStr) > 0 {
				sb.WriteString("\n新文本：\n```\n")
				sb.WriteString(truncateString(newStr, 100))
				sb.WriteString("\n```")
			}
			return sb.String()
		}
	case "Bash":
		if cmd, ok := inputMap["command"].(string); ok {
			// Format: 🔧 **Bash**
			//
			// ```command: go build -o /tmp/dogclaw ./cmd/dogclaw```
			sb.WriteString("**Bash**\n\n")
			sb.WriteString("```\n")
			sb.WriteString(cmd)
			sb.WriteString("\n```")
			return sb.String()
		}
	case "Grep":
		if pattern, ok := inputMap["pattern"].(string); ok {
			sb.WriteString("搜索：`")
			sb.WriteString(pattern)
			sb.WriteString("`")
			if path, ok := inputMap["path"].(string); ok {
				sb.WriteString("\n路径：`")
				sb.WriteString(path)
				sb.WriteString("`")
			}
			return sb.String()
		}
	case "Glob":
		if pattern, ok := inputMap["pattern"].(string); ok {
			sb.WriteString("查找文件：`")
			sb.WriteString(pattern)
			sb.WriteString("`")
			return sb.String()
		}
	case "WebSearch":
		if query, ok := inputMap["query"].(string); ok {
			sb.WriteString("网络搜索：`")
			sb.WriteString(query)
			sb.WriteString("`")
			return sb.String()
		}
	case "WebFetch":
		if url, ok := inputMap["url"].(string); ok {
			sb.WriteString("获取网页：`")
			sb.WriteString(truncateString(url, 60))
			sb.WriteString("`")
			return sb.String()
		}
	}

	// Generic fallback: show all string parameters
	first := true
	for k, v := range inputMap {
		if s, ok := v.(string); ok && len(s) > 0 {
			if first {
				first = false
				sb.WriteString("\n")
			} else {
				sb.WriteString("\n")
			}
			sb.WriteString("")
			sb.WriteString(k)
			sb.WriteString(": `")
			sb.WriteString(truncateString(s, 60))
			sb.WriteString("`")
		}
	}
	return sb.String()
}

// truncateString truncates a string to n characters with ellipsis if needed.
func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
