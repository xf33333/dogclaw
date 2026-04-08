package query

import (
	"context"
	"dogclaw/internal/logger"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"dogclaw/internal/api"
	"dogclaw/pkg/compact"
	"dogclaw/pkg/compactmem"
	"dogclaw/pkg/core"
	"dogclaw/pkg/fastmode"
	"dogclaw/pkg/history"
	"dogclaw/pkg/memory"
	"dogclaw/pkg/semantic"
	"dogclaw/pkg/slash"
	"dogclaw/pkg/thinking"
	"dogclaw/pkg/transcript"
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
	maxTokens      int
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

	// Display settings
	showToolUsageInReply bool // Whether to show tool usage explanation in replies
	showThinkingInLog    bool // Whether to log LLM thinking content

	// ToolCallCallback is called each time a tool is invoked during SubmitMessage.
	// It receives the tool name and a human-readable summary of the input.
	ToolCallCallback func(toolName string, summary string)

	// TextCallback is called each time the LLM emits a text block during SubmitMessage,
	// including turns that also contain tool calls. This allows channels to forward
	// intermediate LLM commentary in real-time without waiting for the full loop to finish.
	TextCallback func(text string)

	// LastTurnToolCalls records the last turn's tool use blocks (for channels to consume after SubmitMessage)
	LastTurnToolCalls []ToolCallInfo

	// Memory system
	memoryDir        string
	memoryIndex      *semantic.MemoryIndex
	memoryCompactor  *compactmem.CompactionConfig
	autoMemoryPrompt string
	memoryInitOnce   sync.Once
	memoryCompacted  bool // tracks whether compaction has been attempted this session

	// Transcript-based session resume
	transcriptProjectMgr *transcript.ProjectManager
	transcriptFile       *transcript.TranscriptFile
	sessionManager       *transcript.SessionManager // for advanced session operations (search, summaries)

	// Per-query turn budget: when set > 0, currentTurn is checked against this value.
	// Reset to queryMaxTurns at the start of each SubmitMessage / RunMainLoop call
	// so each query gets its own turn budget.
	queryMaxTurns int
	// queryLimitGraceMode: when true and queryMaxTurns is reached, add a system
	// prompt asking for a summary and allow one final turn before stopping.
	queryLimitGraceMode bool

	// logger is the logrus instance for structured logging
	logger            *logrus.Logger
	lastAssistantText string // cached text of most recent assistant reply (for channels)
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

	// Initialize logger with global logger
	logger := logger.GetGlobalLogger()

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

	// Initialize memory system
	memoryDir := memory.GetAutoMemPath()
	memoryCompactor := compactmem.DefaultCompactionConfig()

	// Initialize transcript project manager
	pm, _ := transcript.NewProjectManager("")

	// Initialize session manager
	baseDir := pm.GetBaseDir()
	sm, _ := transcript.NewSessionManager(baseDir)

	return &QueryEngine{
		client:         client,
		tools:          tools,
		messages:       make([]api.MessageParam, 0),
		systemPrompt:   systemPrompt,
		maxTurns:       maxTurns,
		maxTokens:      8192,
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

		// Memory system
		memoryDir:       memoryDir,
		memoryIndex:     semantic.NewMemoryIndex(semantic.DefaultEmbeddingDim),
		memoryCompactor: memoryCompactor,

		// Transcript system
		transcriptProjectMgr: pm,
		sessionManager:       sm,

		// Logger
		logger: logger,
	}
}


// SetVerbose enables/disables verbose mode
func (qe *QueryEngine) SetVerbose(verbose bool) {
	qe.verbose = verbose
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

	// Capture output text natively for CLI presentation
	qe.lastAssistantText = result.Output

	// Handle specific commands that modify engine state
	name, _ := slash.ParseCommand(input)
	switch strings.ToLower(name) {
	case "clear", "reset":
		qe.messages = make([]api.MessageParam, 0)
		qe.currentTurn = 0
		qe.usageTracker = &usage.AccumulatedUsage{}
		qe.historyMgr.Init(qe.cwd, qe.sessionID)
		qe.logger.Info(result.Output)

	case "new":
		msg := qe.StartNewSession(ctx)
		qe.lastAssistantText = msg
		// Handled entirely by StartNewSession which also logs it

	case "model":
		_, args := slash.ParseCommand(input)
		if args != "" {
			qe.modelName = strings.ToLower(args)
			qe.client.Model = qe.modelName
		}
		qe.logger.Info(result.Output)

	case "verbose":
		qe.verbose = !qe.verbose
		qe.logger.Infof("Verbose mode: %v", qe.verbose)
		qe.lastAssistantText = fmt.Sprintf("Verbose mode: %v", qe.verbose)

	case "max-turns":
		_, args := slash.ParseCommand(input)
		if args != "" {
			var maxTurns int
			fmt.Sscanf(args, "%d", &maxTurns)
			if maxTurns > 0 {
				qe.maxTurns = maxTurns
			}
		}
		qe.logger.Info(result.Output)

	case "usage":
		cmdResult, err := slash.HandleUsageCommand(ctx, "", qe.usageTracker)
		if err != nil {
			return err
		}
		qe.lastAssistantText = cmdResult.Output
		qe.logger.Info(cmdResult.Output)

	case "skills":
		_, args := slash.ParseCommand(input)
		cmdResult, err := slash.HandleSkillsCommand(ctx, args, qe.skillRegistry)
		if err != nil {
			return err
		}
		qe.lastAssistantText = cmdResult.Output
		qe.logger.Info(cmdResult.Output)

	case "thinking":
		_, args := slash.ParseCommand(input)
		args = strings.TrimSpace(args)
		if args == "" {
			state := "enabled"
			if !qe.thinkingConfig.Enabled {
				state = "disabled"
			}
			msg := fmt.Sprintf("Thinking: %s (budget: %d tokens)", state, qe.thinkingConfig.BudgetTokens)
			qe.lastAssistantText = msg
			qe.logger.Info(msg)
		} else {
			thinkType, err := thinking.ParseThinkingType(args)
			if err != nil {
				qe.logger.Errorf("Error: %v", err)
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
			msg := fmt.Sprintf("Thinking set to: %s", thinkType)
			qe.lastAssistantText = msg
			qe.logger.Info(msg)
		}

	case "fast":
		_, args := slash.ParseCommand(input)
		args = strings.TrimSpace(args)
		if args == "" {
			state := qe.fastModeManager.GetState()
			msg := fmt.Sprintf("Fast Mode: %s", state)
			if state == fastmode.StateCooldown {
				remaining := qe.fastModeManager.TimeUntilCooldownEnd()
				msg += fmt.Sprintf("\nCooldown remaining: %v", remaining)
			}
			qe.lastAssistantText = msg
			qe.logger.Infof(msg)
		} else {
			switch strings.ToLower(args) {
			case "on", "enable":
				qe.fastModeManager = fastmode.NewManager(true)
				qe.fastModeManager.SetModel(qe.modelName)
				qe.lastAssistantText = "Fast mode enabled"
				qe.logger.Info("Fast mode enabled")
			case "off", "disable":
				qe.fastModeManager.Disable()
				qe.lastAssistantText = "Fast mode disabled"
				qe.logger.Info("Fast mode disabled")
			case "status":
				state := qe.fastModeManager.GetState()
				msg := fmt.Sprintf("Fast Mode: %s (model: %s)", state, qe.fastModeManager.GetModel())
				qe.lastAssistantText = msg
				qe.logger.Infof(msg)
			default:
				qe.logger.Warnf("Unknown fast mode argument: %s. Use: on/off/status", args)
			}
		}

	case "snip":
		_, args := slash.ParseCommand(input)
		args = strings.TrimSpace(args)
		if args == "" {
			snipResult := compact.SnipHistory(qe.messages, qe.snipConfig)
			if snipResult != nil {
				qe.messages = snipResult.Remaining
				msg := fmt.Sprintf("Snipped %d messages, %d remaining", snipResult.SnippedCount, len(snipResult.Remaining))
				qe.lastAssistantText = msg
				qe.logger.Infof(msg)
			} else {
				qe.lastAssistantText = "No snip needed"
				qe.logger.Info("No snip needed")
			}
		} else {
			switch strings.ToLower(args) {
			case "on", "enable":
				qe.snipConfig.Enabled = true
				qe.lastAssistantText = "Snip enabled"
				qe.logger.Info("Snip enabled")
			case "off", "disable":
				qe.snipConfig.Enabled = false
				qe.lastAssistantText = "Snip disabled"
				qe.logger.Info("Snip disabled")
			case "status":
				msg := fmt.Sprintf("Snip: enabled=%v, max_messages=%d, preserve=%d",
					qe.snipConfig.Enabled, qe.snipConfig.MaxMessages, qe.snipConfig.PreserveCount)
				qe.lastAssistantText = msg
				qe.logger.Infof(msg)
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
			msg := fmt.Sprintf("Show tool usage in reply: %s", state)
			qe.lastAssistantText = msg
			qe.logger.Infof(msg)
		} else {
			switch strings.ToLower(args) {
			case "on", "enable":
				qe.showToolUsageInReply = true
				qe.lastAssistantText = "Tool usage will be shown in replies ✅"
				qe.logger.Info("Tool usage will be shown in replies ✅")
			case "off", "disable":
				qe.showToolUsageInReply = false
				qe.lastAssistantText = "Tool usage hidden from replies"
				qe.logger.Info("Tool usage hidden from replies")
			default:
				qe.logger.Warnf("Unknown argument: %s. Use: on/off", args)
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
			msg := fmt.Sprintf("Show thinking in log: %s", state)
			qe.lastAssistantText = msg
			qe.logger.Infof(msg)
		} else {
			switch strings.ToLower(args) {
			case "on", "enable":
				qe.showThinkingInLog = true
				qe.lastAssistantText = "Thinking will be logged ✅"
				qe.logger.Info("Thinking will be logged ✅")
			case "off", "disable":
				qe.showThinkingInLog = false
				qe.lastAssistantText = "Thinking hidden from logs"
				qe.logger.Info("Thinking hidden from logs")
			default:
				qe.logger.Warnf("Unknown argument: %s. Use: on/off", args)
			}
		}

	case "sessions":
		_, args := slash.ParseCommand(input)
		args = strings.TrimSpace(args)
		out, err := qe.handleSessionsCommand(ctx, args)
		if err != nil {
			errStr := fmt.Sprintf("Error listing sessions: %v", err)
			qe.lastAssistantText = errStr
			qe.logger.Error(errStr)
		} else {
			qe.lastAssistantText = out
			qe.logger.Info(out)
		}

	case "resume":
		_, args := slash.ParseCommand(input)
		args = strings.TrimSpace(args)
		out, err := qe.handleResumeCommand(ctx, args)
		if err != nil {
			errStr := fmt.Sprintf("Error resuming session: %v", err)
			qe.lastAssistantText = errStr
			qe.logger.Error(errStr)
		} else {
			qe.lastAssistantText = out
			qe.logger.Info(out)
		}

	default:
		qe.logger.Info(result.Output)
	}

	return nil
}

// handleSessionsCommand handles the /sessions command to list available sessions
func (qe *QueryEngine) handleSessionsCommand(ctx context.Context, args string) (string, error) {
	// Parse optional search query
	query := strings.TrimSpace(args)

	// Ensure managers exist
	if qe.transcriptProjectMgr == nil {
		pm, err := transcript.NewProjectManager("")
		if err != nil {
			return "", fmt.Errorf("failed to create transcript manager: %w", err)
		}
		qe.transcriptProjectMgr = pm
	}
	if qe.sessionManager == nil {
		sm, err := transcript.NewSessionManager("")
		if err != nil {
			return "", fmt.Errorf("failed to create session manager: %w", err)
		}
		qe.sessionManager = sm
	}

	var sessions []transcript.SessionSummary
	var err error

	if query != "" {
		// Search sessions across all projects using SessionManager
		sessions, err = qe.sessionManager.SearchSessions(query)
		if err != nil {
			return "", fmt.Errorf("failed to search sessions: %w", err)
		}
	} else {
		// List sessions for current cwd with detailed summaries
		sessions, err = qe.listSessionsWithSummary(qe.cwd)
		if err != nil {
			return "", fmt.Errorf("failed to list sessions: %w", err)
		}
	}

	if len(sessions) == 0 {
		return "No previous sessions found.", nil
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Found %d session(s):\n", len(sessions)))
	for i, s := range sessions {
		// Build a rich one-line summary
		builder.WriteString(fmt.Sprintf("  [%d] %s\n", i+1, s.FormatSummary()))
	}
	return builder.String(), nil
}

// listSessionsWithSummary returns SessionSummary for current working directory
func (qe *QueryEngine) listSessionsWithSummary(cwd string) ([]transcript.SessionSummary, error) {
	// Normalize cwd to project key
	projectDir := normalizeSessionPath(cwd)
	if projectDir == "" {
		projectDir = "no-cwd"
	}

	// Use the sessionManager to get detailed summaries
	if qe.sessionManager != nil {
		summaries, err := qe.sessionManager.ListSessionsForCwd(projectDir)
		if err == nil && len(summaries) > 0 {
			return summaries, nil
		}
		// If no summaries for this cwd, try all sessions
		all, err := qe.sessionManager.ListAllSessions()
		if err == nil && len(all) > 0 {
			return all, nil
		}
	}

	// Fallback: use simple ListSessions and convert to minimal summaries
	infos, err := qe.ListSessions()
	if err != nil {
		return nil, err
	}
	var summaries []transcript.SessionSummary
	for _, info := range infos {
		summaries = append(summaries, transcript.SessionSummary{
			SessionID:  info.SessionID,
			FilePath:   info.FilePath,
			ProjectDir: projectDir,
		})
	}
	return summaries, nil
}

// handleResumeCommand handles the /resume command to restore a previous session
func (qe *QueryEngine) handleResumeCommand(ctx context.Context, args string) (string, error) {
	sessionID := strings.TrimSpace(args)
	
	// Fetch sessions to support index-based selection and listing
	var sessions []transcript.SessionSummary
	var err error
	
	// Make sure session manager exists
	if qe.sessionManager == nil {
		sm, errInit := transcript.NewSessionManager("")
		if errInit == nil {
			qe.sessionManager = sm
		}
	}
	
	sessions, err = qe.listSessionsWithSummary(qe.cwd)
	if err != nil {
		return "", fmt.Errorf("failed to list sessions: %w", err)
	}

	if sessionID == "" {
		if len(sessions) == 0 {
			return "", fmt.Errorf("no previous sessions found")
		}

		if len(sessions) == 1 {
			// Auto-resume the only session
			sessionID = sessions[0].SessionID
		} else {
			// Multiple sessions - show list and ask user to specify
			var builder strings.Builder
			builder.WriteString("Multiple sessions found. Use /resume <session-id> or index to select:\n")
			for i, s := range sessions {
				if i >= 5 {
					builder.WriteString("  ...\n")
					break // show top 5
				}
				builder.WriteString(fmt.Sprintf("  [%d] %s\n", i+1, s.SessionID))
			}
			return builder.String(), fmt.Errorf("please specify which session to resume")
		}
	} else {
		// Try to parse the input as an index (1-based)
		if idx, errParse := strconv.Atoi(sessionID); errParse == nil {
			if idx > 0 && idx <= len(sessions) {
				sessionID = sessions[idx-1].SessionID
			} else {
				return "", fmt.Errorf("invalid session index: %d (valid range: 1-%d)", idx, len(sessions))
			}
		}
	}

	// Resume specific session
	err = qe.ResumeFromTranscript(sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to resume session %s: %w", sessionID, err)
	}
	
	msg := fmt.Sprintf("✅ Resumed session: %s\n   Messages: %d, Turns: %d", qe.sessionID, len(qe.messages), qe.currentTurn)
	return msg, nil
}

// AutoResumeLatestSession automatically resumes the most recent session if one exists.
func (qe *QueryEngine) AutoResumeLatestSession(ctx context.Context) error {
	sessions, err := qe.listSessionsWithSummary(qe.cwd)
	if err != nil || len(sessions) == 0 {
		return nil // Normal behavior if no sessions exist
	}
	
	sessionID := sessions[0].SessionID
	err = qe.ResumeFromTranscript(sessionID)
	if err != nil {
		qe.logger.Errorf("Failed to auto-resume latest session: %v", err)
		return err
	}
	
	msg := fmt.Sprintf("♻️  Auto-resumed latest session: %s\n   Messages: %d, Turns: %d", qe.sessionID, len(qe.messages), qe.currentTurn)
	qe.lastAssistantText = msg
	qe.logger.Info(msg)
	return nil
}

// StartNewSession creates a fresh session with a new ID
func (qe *QueryEngine) StartNewSession(ctx context.Context) string {
	qe.messages = make([]api.MessageParam, 0)
	qe.currentTurn = 0
	qe.usageTracker = &usage.AccumulatedUsage{}
	
	// Close current transcript file
	qe.transcriptFile = nil
	
	// Generate new session ID
	qe.sessionID = fmt.Sprintf("session-%d", time.Now().UnixMilli())
	qe.historyMgr.Init(qe.cwd, qe.sessionID)
	qe.initTranscript()
	
	msg := fmt.Sprintf("✅ Started new session: %s", qe.sessionID)
	qe.logger.Info(msg)
	return msg
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

// SetMaxTokens sets the maximum tokens for the model response
func (qe *QueryEngine) SetMaxTokens(tokens int) {
	if tokens > 0 {
		qe.maxTokens = tokens
	}
}

// SetQueryMaxTurns sets the per-query turn budget.
// When > 0, each call to SubmitMessage or RunMainLoop resets currentTurn
// to 0 and enforces this limit individually for that query.
// When 0 (default), the session-wide maxTurns is used (accumulated across queries).
// When set, reaching the limit triggers a grace turn where the model is asked
// to summarize its findings before stopping cleanly.
func (qe *QueryEngine) SetQueryMaxTurns(n int) {
	qe.queryMaxTurns = n
}

// resetForNewQuery resets the turn counter for a new query and logs if verbose.
// Called at the start of SubmitMessage and RunMainLoop when queryMaxTurns > 0.
func (qe *QueryEngine) resetForNewQuery() {
	if qe.queryMaxTurns > 0 {
		qe.currentTurn = 0
		qe.queryLimitGraceMode = false
	}
}

// effectiveMaxTurns returns the turn limit to use for the current query.
// If queryMaxTurns > 0, it returns queryMaxTurns + 1 (accounting for the grace turn).
// Otherwise it returns the session-wide maxTurns.
func (qe *QueryEngine) effectiveMaxTurns() int {
	if qe.queryMaxTurns > 0 {
		return qe.queryMaxTurns + 1 // +1 for the summary grace turn
	}
	return qe.maxTurns
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

// initTranscript initializes the transcript system for the current session.
// If no transcriptFile is active, it creates one using the sessionID and cwd.
func (qe *QueryEngine) initTranscript() {
	if qe.transcriptFile != nil {
		return
	}
	if qe.transcriptProjectMgr == nil {
		pm, err := transcript.NewProjectManager("")
		if err != nil {
			if qe.verbose {
				qe.logger.Debugf("[Transcript] Failed to create project manager: %v", err)
			}
			return
		}
		qe.transcriptProjectMgr = pm
	}
	if qe.sessionID == "" {
		qe.sessionID = fmt.Sprintf("session-%d", time.Now().UnixMilli())
	}
	qe.transcriptFile = qe.transcriptProjectMgr.GetTranscriptFile(qe.sessionID, qe.cwd)
	if qe.verbose {
		qe.logger.Debugf("[Transcript] Initialized for session: %s", qe.sessionID)
	}
}

// ResumeFromTranscript loads a previous session from its transcript file
// and restores the conversation state (messages, turn count, etc.)
func (qe *QueryEngine) ResumeFromTranscript(sessionID string) error {
	if qe.transcriptProjectMgr == nil {
		pm, err := transcript.NewProjectManager("")
		if err != nil {
			return fmt.Errorf("failed to create transcript manager: %w", err)
		}
		qe.transcriptProjectMgr = pm
	}

	// If sessionID is empty, find the most recent session for this cwd
	if sessionID == "" {
		sessions, err := qe.transcriptProjectMgr.ListSessions()
		if err != nil {
			return fmt.Errorf("failed to list sessions: %w", err)
		}
		// Find the most recent session for this cwd
		recentSession := qe.findMostRecentSessionForCwd(sessions, qe.cwd)
		if recentSession == "" {
			return fmt.Errorf("no previous sessions found for current working directory")
		}
		sessionID = recentSession
		if qe.verbose {
			qe.logger.Debugf("[Resume] Auto-selected most recent session: %s", sessionID)
		}
	}

	tf := qe.transcriptProjectMgr.GetTranscriptFile(sessionID, qe.cwd)
	info, err := tf.Replay()
	if err != nil {
		return fmt.Errorf("failed to replay transcript: %w", err)
	}

	// Convert transcript records back to messages
	messages, turns, err := qe.convertTranscriptToMessages(info.Records)
	if err != nil {
		return fmt.Errorf("failed to convert transcript: %w", err)
	}

	// Restore state
	qe.messages = messages
	qe.currentTurn = turns
	qe.sessionID = sessionID
	qe.transcriptFile = tf

	if qe.verbose {
		qe.logger.Debugf("[Resume] Restored session: %s — %d messages, %d turns, %d user, %d assistant, %d tool calls, %d tool results",
			sessionID, len(messages), turns,
			info.Stats.UserMessages, info.Stats.AssistantMessages,
			info.Stats.ToolCalls, info.Stats.ToolResults)
	}

	// Extract metadata (e.g., model, thinking config if stored)
	if info.Stats.MetadataEntries > 0 {
		qe.restoreTranscriptMetadata(tf)
	}

	// 恢复后立即检查并压缩，防止长会话立即超限
	qe.autoCompactAfterResume()

	return nil
}

// autoCompactAfterResume checks if the resumed session exceeds limits
// and performs automatic compaction if needed.
func (qe *QueryEngine) autoCompactAfterResume() {
	if qe.messages == nil || len(qe.messages) == 0 {
		return
	}

	// 检查是否需要压缩（使用相同的逻辑作为常规自动压缩）
	if qe.compactConfig.Enabled {
		shouldCompact, tokenCount, threshold := compact.CheckAutoCompact(qe.messages, qe.compactConfig, qe.compactTracker)
		if shouldCompact {
			if qe.verbose {
				qe.logger.Debugf("[Resume] Auto-compact triggered: %d tokens >= threshold %d", tokenCount, threshold)
			}
			result, err := compact.CompactMessages(context.Background(), qe.client, qe.messages, qe.systemPrompt, qe.compactConfig)
			if err != nil {
				qe.logger.Warnf("[Resume] CompactMessages failed: %v", err)
			} else if result != nil {
				qe.messages = compact.ApplyCompactResult(qe.messages, result)
				qe.compactTracker.Compacted = true
				qe.compactTracker.TurnCounter++
				if qe.verbose {
					qe.logger.Debugf("[Resume] Compaction complete: %d -> %d messages, %d -> %d tokens",
						result.OriginalMessageCount, result.CompactedMessageCount,
						result.PreCompactTokenCount, result.PostCompactTokenCount)
				}
			}
		} else {
			// 检查警告状态
			warning, isBlocking := compact.GetWarningState(tokenCount, qe.compactConfig)
			if warning != "" && qe.verbose {
				qe.logger.Warn(warning)
			}
			if isBlocking {
				if qe.verbose {
					qe.logger.Warn("[Resume] Context blocking limit reached, will need compaction before next turn")
				}
			}
		}
	}

	// 同时检查是否需要 snip (更激进的消息数裁剪)
	if qe.snipConfig.Enabled {
		snipResult := compact.SnipHistory(qe.messages, qe.snipConfig)
		if snipResult != nil && snipResult.SnippedCount > 0 {
			if qe.verbose {
				qe.logger.Debugf("[Resume] Snip: removed %d messages, %d remaining", snipResult.SnippedCount, len(snipResult.Remaining))
			}
			qe.messages = snipResult.Remaining
		}
	}
}

// findMostRecentSessionForCwd finds the most recent session ID for a given cwd
func (qe *QueryEngine) findMostRecentSessionForCwd(sessions []transcript.SessionInfo, targetCwd string) string {
	// Normalize target cwd for matching
	targetNormalized := normalizeSessionPath(targetCwd)

	var best transcript.SessionInfo
	var bestTime int64

	for _, s := range sessions {
		// Match by project path
		if normalizeSessionPath(s.ProjectPath) == targetNormalized {
			// Try to get file mod time
			if info, err := os.Stat(s.FilePath); err == nil {
				mt := info.ModTime().UnixMilli()
				if mt > bestTime {
					best = s
					bestTime = mt
				}
			} else {
				// Fall back: just pick the first match
				if best.SessionID == "" {
					best = s
				}
			}
		}
	}

	if best.SessionID == "" {
		return ""
	}
	return best.SessionID
}

// normalizeSessionPath converts a cwd to the sanitized directory name used for transcript storage.
// It applies the same sanitization logic as transcript.ProjectManager.sanitizeCWDForPath.
func normalizeSessionPath(cwd string) string {
	if cwd == "" {
		return "no-cwd"
	}

	// Sanitize to a safe directory name (match transcript.ProjectManager.sanitizeCWDForPath)
	// Replace path separators with underscores
	safe := strings.ReplaceAll(cwd, string(filepath.Separator), "_")
	safe = strings.ReplaceAll(safe, "/", "_")
	safe = strings.ReplaceAll(safe, "\\", "_")

	// Replace Windows drive colon (for ~/C_... pattern)
	if len(safe) > 1 && safe[1] == ':' {
		safe = safe[:1] + "_" + safe[2:]
	}

	// Remove or replace any remaining problematic characters
	re := regexp.MustCompile(`[^a-zA-Z0-9_.-]`)
	safe = re.ReplaceAllString(safe, "_")

	// Collapse multiple underscores
	for strings.Contains(safe, "__") {
		safe = strings.ReplaceAll(safe, "__", "_")
	}

	// Trim leading/trailing underscores
	safe = strings.Trim(safe, "_")

	if safe == "" {
		safe = "unknown"
	}

	return safe
}

// restoreTranscriptMetadata reads metadata from transcript and restores engine settings
func (qe *QueryEngine) restoreTranscriptMetadata(tf *transcript.TranscriptFile) {
	meta, err := tf.ReadMetadata()
	if err != nil {
		if qe.verbose {
			qe.logger.Debugf("[Resume] Failed to read metadata: %v", err)
		}
		return
	}

	if model, ok := meta[string(transcript.MetadataLastPrompt)]; ok {
		_ = model // could restore model if stored
	}

	if qe.verbose {
		count := len(meta)
		qe.logger.Debugf("[Resume] Restored %d metadata entries", count)
	}
}

// convertTranscriptToMessages converts transcript records into API messages
func (qe *QueryEngine) convertTranscriptToMessages(records []transcript.TranscriptRecord) ([]api.MessageParam, int, error) {
	var messages []api.MessageParam
	turns := 0

	for _, r := range records {
		if r.IsSidechain {
			continue
		}

		switch r.Type {
		case transcript.MessageTypeUser:
			var content any = r.Content
			// Try to parse as structured content blocks
			var blocks []api.ContentBlockParam
			if err := json.Unmarshal([]byte(r.Content), &blocks); err == nil && len(blocks) > 0 {
				content = blocks
			}
			msg := api.MessageParam{
				Role:    "user",
				Content: content,
			}
			messages = append(messages, msg)

		case transcript.MessageTypeAssistant:
			var contentBlocks []api.ContentBlockParam

			// Try to parse structured content
			var blocks []api.ContentBlockParam
			if r.Content != "" {
				if err := json.Unmarshal([]byte(r.Content), &blocks); err == nil && len(blocks) > 0 {
					contentBlocks = blocks
				} else {
					// Plain text content
					if r.Content != "" {
						contentBlocks = append(contentBlocks, api.ContentBlockParam{
							Type: "text",
							Text: r.Content,
						})
					}
				}
			}

			if len(contentBlocks) > 0 {
				msg := api.MessageParam{
					Role:    "assistant",
					Content: contentBlocks,
				}
				messages = append(messages, msg)
				turns++
			}

		case transcript.MessageTypeMetadata:
			// Skip metadata, handled separately
			continue

		default:
			// Handle tool results that might be standalone records
			if r.ToolUseID != "" && r.Content != "" {
				// This is a tool_result record
				isError := strings.HasPrefix(r.Content, "Error:") || strings.HasPrefix(r.Content, "{\"error\"")
				msg := api.MessageParam{
					Role: "user",
					Content: []api.ContentBlockParam{
						{
							Type:      "tool_result",
							ToolUseID: r.ToolUseID,
							Content: []api.ContentBlockParam{
								{Type: "text", Text: r.Content},
							},
							IsError: isError,
						},
					},
				}
				messages = append(messages, msg)
			}
		}
	}

	return messages, turns, nil
}

// RecordMessageToTranscript records a message to the transcript file
func (qe *QueryEngine) RecordMessageToTranscript(msgType transcript.MessageType, role string, contentBytes []byte) {
	qe.initTranscript()
	if qe.transcriptFile == nil {
		return
	}

	contentStr := string(contentBytes)

	record := transcript.TranscriptRecord{
		Type:      msgType,
		UUID:      fmt.Sprintf("msg-%d-%d", time.Now().UnixMilli(), len(qe.messages)),
		SessionID: qe.sessionID,
		Cwd:       qe.cwd,
		Version:   transcript.TranscriptVersion,
		GitBranch: qe.getGitBranch(),
		Timestamp: time.Now().UnixMilli(),
		Role:      role,
		Content:   contentStr,
	}

	qe.transcriptFile.Queue(record)
}

// RecordToolCallToTranscript records a tool call to the transcript
func (qe *QueryEngine) RecordToolCallToTranscript(toolUseID, toolName string, input map[string]any) {
	qe.initTranscript()
	if qe.transcriptFile == nil {
		return
	}

	inputJSON, _ := json.Marshal(input)

	record := transcript.TranscriptRecord{
		Type:      transcript.MessageTypeAssistant,
		UUID:      fmt.Sprintf("tool-use-%s", toolUseID),
		SessionID: qe.sessionID,
		Cwd:       qe.cwd,
		Version:   transcript.TranscriptVersion,
		GitBranch: qe.getGitBranch(),
		Timestamp: time.Now().UnixMilli(),
		Role:      "assistant",
		Content:   string(inputJSON),
		ToolUseID: toolUseID,
		ToolName:  toolName,
	}

	qe.transcriptFile.Queue(record)
}

// RecordToolResultToTranscript records a tool result to the transcript
func (qe *QueryEngine) RecordToolResultToTranscript(toolUseID, toolName, resultContent string, isError bool) {
	qe.initTranscript()
	if qe.transcriptFile == nil {
		return
	}

	prefix := ""
	if isError {
		prefix = "Error: "
	}

	record := transcript.TranscriptRecord{
		Type:      "tool_result",
		UUID:      fmt.Sprintf("tool-result-%s", toolUseID),
		SessionID: qe.sessionID,
		Cwd:       qe.cwd,
		Version:   transcript.TranscriptVersion,
		GitBranch: qe.getGitBranch(),
		Timestamp: time.Now().UnixMilli(),
		Role:      "user",
		Content:   prefix + resultContent,
		ToolUseID: toolUseID,
		ToolName:  toolName,
	}

	qe.transcriptFile.Queue(record)
}

// FlushTranscript ensures all pending transcript records are written to disk
func (qe *QueryEngine) FlushTranscript() {
	if qe.transcriptFile != nil {
		if err := qe.transcriptFile.Flush(); err != nil && qe.verbose {
			qe.logger.Debugf("[Transcript] Flush error: %v", err)
		}
	}
}

// GetTranscriptFile returns the current transcript file for external use
func (qe *QueryEngine) GetTranscriptFile() *transcript.TranscriptFile {
	qe.initTranscript()
	return qe.transcriptFile
}

// GetSessionID returns the current session ID
func (qe *QueryEngine) GetSessionID() string {
	return qe.sessionID
}

// ListSessions lists all sessions for the current working directory
func (qe *QueryEngine) ListSessions() ([]transcript.SessionInfo, error) {
	if qe.transcriptProjectMgr == nil {
		pm, err := transcript.NewProjectManager("")
		if err != nil {
			return nil, err
		}
		qe.transcriptProjectMgr = pm
	}

	sessions, err := qe.transcriptProjectMgr.ListSessions()
	if err != nil {
		return nil, err
	}

	// Filter to only sessions for this cwd
	targetNormalized := normalizeSessionPath(qe.cwd)
	var filtered []transcript.SessionInfo
	for _, s := range sessions {
		if normalizeSessionPath(s.ProjectPath) == targetNormalized {
			filtered = append(filtered, s)
		}
	}

	return filtered, nil
}

// getGitBranch returns the current git branch (if applicable)
func (qe *QueryEngine) getGitBranch() string {
	// Simple fallback — can be enhanced with actual git parsing
	return ""
}

// initMemoryIndex scans the memory directory, reads all memory files,
// generates LLM embeddings, and populates the semantic index.
// Safe to call multiple times (uses sync.Once internally).
func (qe *QueryEngine) initMemoryIndex(ctx context.Context) {
	qe.memoryInitOnce.Do(func() {
		if !memory.IsAutoMemoryEnabled() {
			return
		}

		// Read MEMORY.md to get the index
		entryPoint := memory.GetAutoMemEntrypoint()
		indexContent, err := os.ReadFile(entryPoint)
		if err != nil || len(strings.TrimSpace(string(indexContent))) == 0 {
			if qe.verbose {
				qe.logger.Debug("[Memory index is empty — skipping semantic embedding initialization]")
			}
			return
		}

		// Parse index entries to get filenames
		indexEntries := parseMemoryIndexLinks(string(indexContent))
		if len(indexEntries) == 0 {
			if qe.verbose {
				qe.logger.Debug("[Memory index has no entries — skipping semantic embedding initialization]")
			}
			return
		}

		// Scan memory files and build index entries
		headers, err := memory.ScanMemoryFiles(qe.memoryDir)
		if err != nil {
			if qe.verbose {
				qe.logger.Debugf("[Memory scan failed: %v — skipping semantic embedding]", err)
			}
			return
		}

		relevant, err := core.Ingest(headers)
		if err != nil {
			if qe.verbose {
				qe.logger.Debugf("[Memory ingest failed: %v — skipping semantic embedding]", err)
			}
			return
		}

		if len(relevant) == 0 {
			return
		}

		// Cap memories to embed per batch
		maxMemories := len(relevant)
		if maxMemories > semantic.MaxMemoriesToEmbed {
			maxMemories = semantic.MaxMemoriesToEmbed
			if qe.verbose {
				qe.logger.Debugf("[Memory limit hit: truncating to %d memories for embedding]", maxMemories)
			}
		}

		// Build index entries and generate embeddings
		var entries []semantic.IndexEntry
		for i := 0; i < maxMemories; i++ {
			m := relevant[i]
			// Determine name from path
			name := m.Path
			if idx := strings.LastIndex(name, "/"); idx >= 0 {
				name = name[idx+1:]
			}
			name = strings.TrimSuffix(name, ".md")

			// Find matching header for description
			var desc string
			for _, h := range headers {
				if h.FilePath == m.Path {
					desc = h.Description
					break
				}
			}

			// Generate embedding via LLM (skip on error to keep system resilient)
			embedText := fmt.Sprintf("%s: %s\n%s", name, desc, m.Content)
			if len(embedText) > 3000 {
				embedText = embedText[:3000]
			}

			vec, err := semantic.GenerateEmbedding(ctx, qe.client, embedText, semantic.DefaultEmbeddingDim)
			if err != nil {
				if qe.verbose {
					qe.logger.Debugf("[Failed to embed memory %s: %v]", name, err)
				}
				// Still add without embedding — will use keyword fallback
			}

			entries = append(entries, semantic.IndexEntry{
				Path:        m.Path,
				Name:        name,
				Description: desc,
				Content:     m.Content,
				Embedding:   vec,
				MtimeMs:     m.MtimeMs,
			})
		}

		qe.memoryIndex.AddEntries(entries)
		if qe.verbose {
			qe.logger.Debugf("[Semantic memory index initialized: %d entries]", len(entries))
		}
	})
}

// tryCompactMemory checks if memory compaction is needed and performs it.
// It's designed to run once per session (on first SubmitMessage/RunMainLoop).
func (qe *QueryEngine) tryCompactMemory(ctx context.Context) {
	if qe.memoryCompacted || !memory.IsAutoMemoryEnabled() || qe.memoryCompactor == nil {
		return
	}
	qe.memoryCompacted = true

	memoryDir := qe.memoryDir
	if memoryDir != "" && qe.memoryCompactor.Enabled {
		res, err := compactmem.CompactIfNeeded(ctx, qe.client, memoryDir, qe.memoryCompactor)
		if err != nil {
			if qe.verbose {
				qe.logger.Debugf("[Memory compaction failed: %v]", err)
			}
		} else if res != nil {
			if qe.verbose {
				qe.logger.Debugf("[Memory compaction: %d -> %d files]", res.OriginalCount, res.NewCount)
			}
		}
	}
}

// parseMemoryIndexLinks parses the MEMORY.md index and extracts
// filenames from markdown links like `- [title](file.md) — desc`.
func parseMemoryIndexLinks(content string) []string {
	var results []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		start := strings.Index(line, "](")
		end := strings.Index(line, ")")
		if start >= 0 && end > start+2 {
			filename := line[start+2 : end]
			results = append(results, filename)
		}
	}
	return results
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
