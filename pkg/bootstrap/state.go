package bootstrap

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"dogclaw/pkg/types"
)

// ChannelEntry represents a registered channel (plugin or server).
type ChannelEntry struct {
	Kind        string // "plugin" or "server"
	Name        string
	Marketplace string // only for plugin kind
	Dev         bool
}

// SessionCronTask represents a session-only cron task (not persisted to disk).
type SessionCronTask struct {
	ID        string
	Cron      string
	Prompt    string
	CreatedAt int64
	Recurring *bool
	AgentID   *string // set when created by in-process teammate
}

// InvokedSkillInfo tracks a skill invoked during the session.
type InvokedSkillInfo struct {
	SkillName string
	SkillPath string
	Content   string
	InvokedAt int64
	AgentID   *string
}

// SlowOperation tracks a slow operation for dev bar display.
type SlowOperation struct {
	Operation  string
	DurationMs float64
	Timestamp  int64
}

// TeleportedSessionInfo tracks teleported session state.
type TeleportedSessionInfo struct {
	IsTeleported          bool
	HasLoggedFirstMessage bool
	SessionID             string
}

// State holds all global session state.
type State struct {
	mu sync.RWMutex

	// Working directory
	OriginalCwd string
	ProjectRoot string
	Cwd         string

	// Cost and duration tracking
	TotalCostUSD                   float64
	TotalAPIDuration               float64
	TotalAPIDurationWithoutRetries float64
	TotalToolDuration              float64
	TurnHookDurationMs             float64
	TurnToolDurationMs             float64
	TurnClassifierDurationMs       float64
	TurnToolCount                  int
	TurnHookCount                  int
	TurnClassifierCount            int

	// Timing
	StartTime           int64
	LastInteractionTime int64

	// Line changes
	TotalLinesAdded   int
	TotalLinesRemoved int

	HasUnknownModelCost bool

	// Model state
	ModelUsage            map[string]types.ModelUsage
	MainLoopModelOverride *string
	InitialMainLoopModel  *string
	ModelStrings          map[string]interface{}

	// Session flags
	IsInteractive                    bool
	KairosActive                     bool
	StrictToolResultPairing          bool
	SdkAgentProgressSummariesEnabled bool
	UserMsgOptIn                     bool
	ClientType                       string
	SessionSource                    *string
	QuestionPreviewFormat            *string // "markdown" or "html"
	FlagSettingsPath                 *string
	FlagSettingsInline               map[string]interface{}
	AllowedSettingSources            []string
	SessionIngressToken              *string
	OauthTokenFromFd                 *string
	ApiKeyFromFd                     *string

	// Session identity
	SessionID       types.SessionID
	ParentSessionID *types.SessionID

	// Telemetry (placeholders for now)
	Meter                       interface{}
	SessionCounter              interface{}
	LocCounter                  interface{}
	PrCounter                   interface{}
	CommitCounter               interface{}
	CostCounter                 interface{}
	TokenCounter                interface{}
	CodeEditToolDecisionCounter interface{}
	ActiveTimeCounter           interface{}
	StatsStore                  interface{}

	// Logging
	LoggerProvider interface{}
	EventLogger    interface{}
	MeterProvider  interface{}
	TracerProvider interface{}

	// Agent colors
	AgentColorMap   map[string]string
	AgentColorIndex int

	// Last API request (for bug reports)
	LastAPIRequest         map[string]interface{}
	LastAPIRequestMessages []interface{}
	LastClassifierRequests []interface{}
	CachedAgentMdContent   *string

	// Error log
	InMemoryErrorLog []map[string]string

	// Plugins
	InlinePlugins      []string
	ChromeFlagOverride *bool
	UseCoworkPlugins   bool

	// Permissions
	SessionBypassPermissionsMode bool

	// Scheduled tasks
	ScheduledTasksEnabled bool
	SessionCronTasks      []SessionCronTask

	// Teams
	SessionCreatedTeams map[string]bool

	// Trust
	SessionTrustAccepted       bool
	SessionPersistenceDisabled bool

	// Plan mode
	HasExitedPlanMode           bool
	NeedsPlanModeExitAttachment bool
	NeedsAutoModeExitAttachment bool

	// LSP recommendation
	LspRecommendationShownThisSession bool

	// SDK init
	InitJsonSchema  map[string]interface{}
	RegisteredHooks map[string][]interface{}

	// Plan slug cache
	PlanSlugCache map[string]string

	// Teleported session
	TeleportedSessionInfo *TeleportedSessionInfo

	// Invoked skills
	InvokedSkills map[string]InvokedSkillInfo

	// Slow operations
	SlowOperations []SlowOperation

	// SDK betas
	SdkBetas []string

	// Agent type
	MainThreadAgentType *string

	// Remote mode
	IsRemoteMode           bool
	DirectConnectServerUrl *string

	// System prompt cache
	SystemPromptSectionCache map[string]*string

	// Last emitted date
	LastEmittedDate *string

	// Additional directories
	AdditionalDirectoriesForClaudeMd []string

	// Channels
	AllowedChannels []ChannelEntry
	HasDevChannels  bool

	// Session project dir
	SessionProjectDir *string

	// Prompt cache
	PromptCache1hAllowlist []string
	PromptCache1hEligible  *bool

	// Beta header latches
	AfkModeHeaderLatched      *bool
	FastModeHeaderLatched     *bool
	CacheEditingHeaderLatched *bool
	ThinkingClearLatched      *bool

	// Prompt correlation
	PromptId *string

	// Last main request
	LastMainRequestId *string

	// API completion timestamp
	LastApiCompletionTimestamp *int64

	// Post-compaction flag
	PendingPostCompaction bool
}

var (
	state State
	once  sync.Once

	// Session switched signal
	sessionSwitchCallbacks []func(id types.SessionID)
	sessionSwitchMu        sync.RWMutex
)

func getState() *State {
	once.Do(func() {
		state = getInitialState()
	})
	return &state
}

func getInitialState() State {
	resolvedCwd := resolveCwd()

	return State{
		OriginalCwd: resolvedCwd,
		ProjectRoot: resolvedCwd,
		Cwd:         resolvedCwd,

		TotalCostUSD:                   0,
		TotalAPIDuration:               0,
		TotalAPIDurationWithoutRetries: 0,
		TotalToolDuration:              0,
		TurnHookDurationMs:             0,
		TurnToolDurationMs:             0,
		TurnClassifierDurationMs:       0,
		TurnToolCount:                  0,
		TurnHookCount:                  0,
		TurnClassifierCount:            0,

		StartTime:           time.Now().UnixMilli(),
		LastInteractionTime: time.Now().UnixMilli(),

		TotalLinesAdded:     0,
		TotalLinesRemoved:   0,
		HasUnknownModelCost: false,

		ModelUsage:            make(map[string]types.ModelUsage),
		MainLoopModelOverride: nil,
		InitialMainLoopModel:  nil,
		ModelStrings:          nil,

		IsInteractive:                    false,
		KairosActive:                     false,
		StrictToolResultPairing:          false,
		SdkAgentProgressSummariesEnabled: false,
		UserMsgOptIn:                     false,
		ClientType:                       "cli",
		SessionSource:                    nil,
		QuestionPreviewFormat:            nil,
		FlagSettingsPath:                 nil,
		FlagSettingsInline:               nil,
		AllowedSettingSources: []string{
			"userSettings", "projectSettings", "localSettings",
			"flagSettings", "policySettings",
		},
		SessionIngressToken: nil,
		OauthTokenFromFd:    nil,
		ApiKeyFromFd:        nil,

		SessionID: types.SessionID(generateUUID()),

		AgentColorMap:   make(map[string]string),
		AgentColorIndex: 0,

		LastAPIRequest:         nil,
		LastAPIRequestMessages: nil,
		LastClassifierRequests: nil,
		CachedAgentMdContent:   nil,

		InMemoryErrorLog:   []map[string]string{},
		InlinePlugins:      []string{},
		ChromeFlagOverride: nil,
		UseCoworkPlugins:   false,

		SessionBypassPermissionsMode: false,
		ScheduledTasksEnabled:        false,
		SessionCronTasks:             []SessionCronTask{},
		SessionCreatedTeams:          make(map[string]bool),

		SessionTrustAccepted:       false,
		SessionPersistenceDisabled: false,

		HasExitedPlanMode:           false,
		NeedsPlanModeExitAttachment: false,
		NeedsAutoModeExitAttachment: false,

		LspRecommendationShownThisSession: false,

		InitJsonSchema:  nil,
		RegisteredHooks: nil,

		PlanSlugCache: make(map[string]string),

		TeleportedSessionInfo: nil,
		InvokedSkills:         make(map[string]InvokedSkillInfo),
		SlowOperations:        []SlowOperation{},

		SdkBetas:               nil,
		MainThreadAgentType:    nil,
		IsRemoteMode:           false,
		DirectConnectServerUrl: nil,

		SystemPromptSectionCache:         make(map[string]*string),
		LastEmittedDate:                  nil,
		AdditionalDirectoriesForClaudeMd: []string{},
		AllowedChannels:                  []ChannelEntry{},
		HasDevChannels:                   false,
		SessionProjectDir:                nil,

		PromptCache1hAllowlist: nil,
		PromptCache1hEligible:  nil,

		AfkModeHeaderLatched:      nil,
		FastModeHeaderLatched:     nil,
		CacheEditingHeaderLatched: nil,
		ThinkingClearLatched:      nil,

		PromptId:                   nil,
		LastMainRequestId:          nil,
		LastApiCompletionTimestamp: nil,
		PendingPostCompaction:      false,
	}
}

func resolveCwd() string {
	rawCwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(rawCwd)
	if err != nil {
		return rawCwd
	}
	return resolved
}

func generateUUID() string {
	// Placeholder - use proper UUID generation
	return "00000000-0000-0000-0000-000000000000"
}

// Session ID accessors

func GetSessionID() types.SessionID {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SessionID
}

func RegenerateSessionID(setCurrentAsParent bool) types.SessionID {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()

	if setCurrentAsParent {
		s.ParentSessionID = &s.SessionID
	}

	// Drop plan slug cache entry
	delete(s.PlanSlugCache, string(s.SessionID))

	s.SessionID = types.SessionID(generateUUID())
	s.SessionProjectDir = nil

	return s.SessionID
}

func GetParentSessionID() *types.SessionID {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ParentSessionID
}

// SwitchSession atomically switches the active session.
func SwitchSession(sessionID types.SessionID, projectDir *string) {
	s := getState()
	s.mu.Lock()

	// Drop outgoing session's plan-slug entry
	delete(s.PlanSlugCache, string(s.SessionID))

	s.SessionID = sessionID
	s.SessionProjectDir = projectDir
	s.mu.Unlock()

	// Notify callbacks
	sessionSwitchMu.RLock()
	callbacks := make([]func(id types.SessionID), len(sessionSwitchCallbacks))
	copy(callbacks, sessionSwitchCallbacks)
	sessionSwitchMu.RUnlock()

	for _, cb := range callbacks {
		cb(sessionID)
	}
}

// OnSessionSwitch registers a callback for session switches.
func OnSessionSwitch(cb func(id types.SessionID)) {
	sessionSwitchMu.Lock()
	defer sessionSwitchMu.Unlock()
	sessionSwitchCallbacks = append(sessionSwitchCallbacks, cb)
}

func GetSessionProjectDir() *string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SessionProjectDir
}

func GetOriginalCwd() string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.OriginalCwd
}

func GetProjectRoot() string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ProjectRoot
}

func SetOriginalCwd(cwd string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.OriginalCwd = filepath.Clean(cwd)
}

func SetProjectRoot(cwd string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ProjectRoot = filepath.Clean(cwd)
}

func GetCwd() string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Cwd
}

func SetCwd(cwd string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Cwd = filepath.Clean(cwd)
}

func GetDirectConnectServerUrl() *string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.DirectConnectServerUrl
}

func SetDirectConnectServerUrl(url string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.DirectConnectServerUrl = &url
}

// Cost and duration

func AddToTotalDuration(duration, durationWithoutRetries float64) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalAPIDuration += duration
	s.TotalAPIDurationWithoutRetries += durationWithoutRetries
}

func AddToTotalCost(cost float64, modelUsage types.ModelUsage, model string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ModelUsage[model] = modelUsage
	s.TotalCostUSD += cost
}

func GetTotalCostUSD() float64 {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TotalCostUSD
}

func GetTotalAPIDuration() float64 {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TotalAPIDuration
}

func GetTotalDuration() float64 {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return float64(time.Now().UnixMilli() - s.StartTime)
}

func GetTotalAPIDurationWithoutRetries() float64 {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TotalAPIDurationWithoutRetries
}

func GetTotalToolDuration() float64 {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TotalToolDuration
}

func AddToToolDuration(duration float64) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalToolDuration += duration
	s.TurnToolDurationMs += duration
	s.TurnToolCount++
}

func GetTurnHookDurationMs() float64 {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TurnHookDurationMs
}

func AddToTurnHookDuration(duration float64) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TurnHookDurationMs += duration
	s.TurnHookCount++
}

func ResetTurnHookDuration() {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TurnHookDurationMs = 0
	s.TurnHookCount = 0
}

func GetTurnHookCount() int {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TurnHookCount
}

func GetTurnToolDurationMs() float64 {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TurnToolDurationMs
}

func ResetTurnToolDuration() {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TurnToolDurationMs = 0
	s.TurnToolCount = 0
}

func GetTurnToolCount() int {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TurnToolCount
}

func GetTurnClassifierDurationMs() float64 {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TurnClassifierDurationMs
}

func AddToTurnClassifierDuration(duration float64) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TurnClassifierDurationMs += duration
	s.TurnClassifierCount++
}

func ResetTurnClassifierDuration() {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TurnClassifierDurationMs = 0
	s.TurnClassifierCount = 0
}

func GetTurnClassifierCount() int {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TurnClassifierCount
}

// Lines changed

func AddToTotalLinesChanged(added, removed int) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalLinesAdded += added
	s.TotalLinesRemoved += removed
}

func GetTotalLinesAdded() int {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TotalLinesAdded
}

func GetTotalLinesRemoved() int {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TotalLinesRemoved
}

// Token totals

func GetTotalInputTokens() int {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := 0
	for _, u := range s.ModelUsage {
		total += u.InputTokens
	}
	return total
}

func GetTotalOutputTokens() int {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := 0
	for _, u := range s.ModelUsage {
		total += u.OutputTokens
	}
	return total
}

func GetTotalCacheReadInputTokens() int {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := 0
	for _, u := range s.ModelUsage {
		total += u.CacheReadInputTokens
	}
	return total
}

func GetTotalCacheCreationInputTokens() int {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := 0
	for _, u := range s.ModelUsage {
		total += u.CacheCreationInputTokens
	}
	return total
}

// Turn token budget tracking
var (
	outputTokensAtTurnStart int
	currentTurnTokenBudget  *int
	budgetContinuationCount int
	budgetMu                sync.Mutex
)

func GetTurnOutputTokens() int {
	return GetTotalOutputTokens() - outputTokensAtTurnStart
}

func GetCurrentTurnTokenBudget() *int {
	return currentTurnTokenBudget
}

func SnapshotOutputTokensForTurn(budget *int) {
	budgetMu.Lock()
	defer budgetMu.Unlock()
	outputTokensAtTurnStart = GetTotalOutputTokens()
	currentTurnTokenBudget = budget
	budgetContinuationCount = 0
}

func GetBudgetContinuationCount() int {
	return budgetContinuationCount
}

func IncrementBudgetContinuationCount() {
	budgetMu.Lock()
	defer budgetMu.Unlock()
	budgetContinuationCount++
}

// Unknown model cost

func SetHasUnknownModelCost() {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.HasUnknownModelCost = true
}

func HasUnknownModelCost() bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.HasUnknownModelCost
}

// Last main request ID

func GetLastMainRequestId() *string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LastMainRequestId
}

func SetLastMainRequestId(requestID string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastMainRequestId = &requestID
}

// Last API completion timestamp

func GetLastApiCompletionTimestamp() *int64 {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LastApiCompletionTimestamp
}

func SetLastApiCompletionTimestamp(timestamp int64) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastApiCompletionTimestamp = &timestamp
}

// Post-compaction

func MarkPostCompaction() {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PendingPostCompaction = true
}

func ConsumePostCompaction() bool {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	was := s.PendingPostCompaction
	s.PendingPostCompaction = false
	return was
}

// Last interaction time

func UpdateLastInteractionTime(immediate bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastInteractionTime = time.Now().UnixMilli()
}

func GetLastInteractionTime() int64 {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LastInteractionTime
}

// Model usage

func GetModelUsage() map[string]types.ModelUsage {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]types.ModelUsage)
	for k, v := range s.ModelUsage {
		result[k] = v
	}
	return result
}

func GetUsageForModel(model string) *types.ModelUsage {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if u, ok := s.ModelUsage[model]; ok {
		return &u
	}
	return nil
}

// Model override

func GetMainLoopModelOverride() *string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.MainLoopModelOverride
}

func GetInitialMainLoopModel() *string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.InitialMainLoopModel
}

func SetMainLoopModelOverride(model *string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.MainLoopModelOverride = model
}

func SetInitialMainLoopModel(model string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.InitialMainLoopModel = &model
}

// SDK betas

func GetSdkBetas() []string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.SdkBetas == nil {
		return nil
	}
	result := make([]string, len(s.SdkBetas))
	copy(result, s.SdkBetas)
	return result
}

func SetSdkBetas(betas []string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	if betas == nil {
		s.SdkBetas = nil
		return
	}
	s.SdkBetas = make([]string, len(betas))
	copy(s.SdkBetas, betas)
}

// Cost state reset

func ResetCostState() {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalCostUSD = 0
	s.TotalAPIDuration = 0
	s.TotalAPIDurationWithoutRetries = 0
	s.TotalToolDuration = 0
	s.StartTime = time.Now().UnixMilli()
	s.TotalLinesAdded = 0
	s.TotalLinesRemoved = 0
	s.HasUnknownModelCost = false
	s.ModelUsage = make(map[string]types.ModelUsage)
	s.PromptId = nil
}

// Cost state restore

func SetCostStateForRestore(data map[string]interface{}) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()

	if v, ok := data["totalCostUSD"].(float64); ok {
		s.TotalCostUSD = v
	}
	if v, ok := data["totalAPIDuration"].(float64); ok {
		s.TotalAPIDuration = v
	}
	if v, ok := data["totalAPIDurationWithoutRetries"].(float64); ok {
		s.TotalAPIDurationWithoutRetries = v
	}
	if v, ok := data["totalToolDuration"].(float64); ok {
		s.TotalToolDuration = v
	}
	if v, ok := data["totalLinesAdded"].(float64); ok {
		s.TotalLinesAdded = int(v)
	}
	if v, ok := data["totalLinesRemoved"].(float64); ok {
		s.TotalLinesRemoved = int(v)
	}
	if v, ok := data["modelUsage"].(map[string]types.ModelUsage); ok {
		s.ModelUsage = v
	}
	if v, ok := data["lastDuration"].(float64); ok {
		s.StartTime = time.Now().UnixMilli() - int64(v)
	}
}

// Test reset

func ResetStateForTests() {
	if os.Getenv("NODE_ENV") != "test" {
		panic("ResetStateForTests can only be called in tests")
	}
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()

	initial := getInitialState()
	s.TotalCostUSD = initial.TotalCostUSD
	s.TotalAPIDuration = initial.TotalAPIDuration
	s.TotalAPIDurationWithoutRetries = initial.TotalAPIDurationWithoutRetries
	s.TotalToolDuration = initial.TotalToolDuration
	s.StartTime = initial.StartTime
	s.TotalLinesAdded = initial.TotalLinesAdded
	s.TotalLinesRemoved = initial.TotalLinesRemoved
	s.HasUnknownModelCost = initial.HasUnknownModelCost
	s.ModelUsage = make(map[string]types.ModelUsage)
	s.PromptId = nil

	outputTokensAtTurnStart = 0
	currentTurnTokenBudget = nil
	budgetContinuationCount = 0
}

// Model strings

func GetModelStrings() map[string]interface{} {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ModelStrings
}

func SetModelStrings(modelStrings map[string]interface{}) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ModelStrings = modelStrings
}

func ResetModelStringsForTestingOnly() {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ModelStrings = nil
}

// Interactive

func GetIsNonInteractiveSession() bool {
	return !GetIsInteractive()
}

func GetIsInteractive() bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.IsInteractive
}

func SetIsInteractive(value bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IsInteractive = value
}

// Client type

func GetClientType() string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ClientType
}

func SetClientType(t string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ClientType = t
}

// Kairos

func GetKairosActive() bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.KairosActive
}

func SetKairosActive(value bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.KairosActive = value
}

// Strict tool result pairing

func GetStrictToolResultPairing() bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.StrictToolResultPairing
}

func SetStrictToolResultPairing(value bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.StrictToolResultPairing = value
}

// User msg opt-in

func GetUserMsgOptIn() bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.UserMsgOptIn
}

func SetUserMsgOptIn(value bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UserMsgOptIn = value
}

// Session source

func GetSessionSource() *string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SessionSource
}

func SetSessionSource(source string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SessionSource = &source
}

// Question preview format

func GetQuestionPreviewFormat() *string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.QuestionPreviewFormat
}

func SetQuestionPreviewFormat(format string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.QuestionPreviewFormat = &format
}

// Flag settings

func GetFlagSettingsPath() *string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.FlagSettingsPath
}

func SetFlagSettingsPath(path *string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FlagSettingsPath = path
}

func GetFlagSettingsInline() map[string]interface{} {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.FlagSettingsInline
}

func SetFlagSettingsInline(settings map[string]interface{}) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FlagSettingsInline = settings
}

// Session ingress token

func GetSessionIngressToken() *string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SessionIngressToken
}

func SetSessionIngressToken(token *string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SessionIngressToken = token
}

// OAuth token from FD

func GetOauthTokenFromFd() *string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.OauthTokenFromFd
}

func SetOauthTokenFromFd(token *string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.OauthTokenFromFd = token
}

// API key from FD

func GetApiKeyFromFd() *string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ApiKeyFromFd
}

func SetApiKeyFromFd(key *string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ApiKeyFromFd = key
}

// Last API request

func SetLastAPIRequest(params map[string]interface{}) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastAPIRequest = params
}

func GetLastAPIRequest() map[string]interface{} {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LastAPIRequest
}

func SetLastAPIRequestMessages(messages []interface{}) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastAPIRequestMessages = messages
}

func GetLastAPIRequestMessages() []interface{} {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LastAPIRequestMessages
}

// Last classifier requests

func SetLastClassifierRequests(requests []interface{}) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastClassifierRequests = requests
}

func GetLastClassifierRequests() []interface{} {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LastClassifierRequests
}

// Cached AGENT.md content

func SetCachedAgentMdContent(content *string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CachedAgentMdContent = content
}

func GetCachedAgentMdContent() *string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CachedAgentMdContent
}

// Error log

func AddToInMemoryErrorLog(errorInfo map[string]string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()

	const maxErrors = 100
	if len(s.InMemoryErrorLog) >= maxErrors {
		s.InMemoryErrorLog = s.InMemoryErrorLog[1:]
	}
	s.InMemoryErrorLog = append(s.InMemoryErrorLog, errorInfo)
}

// Allowed setting sources

func GetAllowedSettingSources() []string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.AllowedSettingSources
}

func SetAllowedSettingSources(sources []string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AllowedSettingSources = sources
}

// Prefer third-party auth

func PreferThirdPartyAuthentication() bool {
	return GetIsNonInteractiveSession() && GetClientType() != "claude-vscode"
}

// Inline plugins

func SetInlinePlugins(plugins []string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.InlinePlugins = plugins
}

func GetInlinePlugins() []string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]string, len(s.InlinePlugins))
	copy(result, s.InlinePlugins)
	return result
}

// Chrome flag override

func SetChromeFlagOverride(value *bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ChromeFlagOverride = value
}

func GetChromeFlagOverride() *bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ChromeFlagOverride
}

// Cowork plugins

func SetUseCoworkPlugins(value bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UseCoworkPlugins = value
}

func GetUseCoworkPlugins() bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.UseCoworkPlugins
}

// Session bypass permissions

func SetSessionBypassPermissionsMode(enabled bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SessionBypassPermissionsMode = enabled
}

func GetSessionBypassPermissionsMode() bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SessionBypassPermissionsMode
}

// Scheduled tasks

func SetScheduledTasksEnabled(enabled bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ScheduledTasksEnabled = enabled
}

func GetScheduledTasksEnabled() bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ScheduledTasksEnabled
}

func GetSessionCronTasks() []SessionCronTask {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]SessionCronTask, len(s.SessionCronTasks))
	copy(result, s.SessionCronTasks)
	return result
}

func AddSessionCronTask(task SessionCronTask) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SessionCronTasks = append(s.SessionCronTasks, task)
}

func RemoveSessionCronTasks(ids []string) int {
	if len(ids) == 0 {
		return 0
	}
	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}

	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()

	var remaining []SessionCronTask
	for _, t := range s.SessionCronTasks {
		if !idSet[t.ID] {
			remaining = append(remaining, t)
		}
	}
	removed := len(s.SessionCronTasks) - len(remaining)
	if removed == 0 {
		return 0
	}
	s.SessionCronTasks = remaining
	return removed
}

// Trust

func SetSessionTrustAccepted(accepted bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SessionTrustAccepted = accepted
}

func GetSessionTrustAccepted() bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SessionTrustAccepted
}

// Session persistence

func SetSessionPersistenceDisabled(disabled bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SessionPersistenceDisabled = disabled
}

func IsSessionPersistenceDisabled() bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SessionPersistenceDisabled
}

// Plan mode

func HasExitedPlanModeInSession() bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.HasExitedPlanMode
}

func SetHasExitedPlanMode(value bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.HasExitedPlanMode = value
}

func NeedsPlanModeExitAttachment() bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.NeedsPlanModeExitAttachment
}

func SetNeedsPlanModeExitAttachment(value bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.NeedsPlanModeExitAttachment = value
}

func HandlePlanModeTransition(fromMode, toMode string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()

	if toMode == "plan" && fromMode != "plan" {
		s.NeedsPlanModeExitAttachment = false
	}
	if fromMode == "plan" && toMode != "plan" {
		s.NeedsPlanModeExitAttachment = true
	}
}

func NeedsAutoModeExitAttachment() bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.NeedsAutoModeExitAttachment
}

func SetNeedsAutoModeExitAttachment(value bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.NeedsAutoModeExitAttachment = value
}

func HandleAutoModeTransition(fromMode, toMode string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()

	// Auto↔plan transitions are handled elsewhere
	if (fromMode == "auto" && toMode == "plan") ||
		(fromMode == "plan" && toMode == "auto") {
		return
	}

	fromIsAuto := fromMode == "auto"
	toIsAuto := toMode == "auto"

	if toIsAuto && !fromIsAuto {
		s.NeedsAutoModeExitAttachment = false
	}
	if fromIsAuto && !toIsAuto {
		s.NeedsAutoModeExitAttachment = true
	}
}

// LSP recommendation

func HasShownLspRecommendationThisSession() bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LspRecommendationShownThisSession
}

func SetLspRecommendationShownThisSession(value bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LspRecommendationShownThisSession = value
}

// SDK init JSON schema

func SetInitJsonSchema(schema map[string]interface{}) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.InitJsonSchema = schema
}

func GetInitJsonSchema() map[string]interface{} {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.InitJsonSchema
}

// Registered hooks

func RegisterHookCallbacks(hooks map[string][]interface{}) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.RegisteredHooks == nil {
		s.RegisteredHooks = make(map[string][]interface{})
	}

	for event, matchers := range hooks {
		s.RegisteredHooks[event] = append(s.RegisteredHooks[event], matchers...)
	}
}

func GetRegisteredHooks() map[string][]interface{} {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.RegisteredHooks
}

func ClearRegisteredHooks() {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RegisteredHooks = nil
}

func ResetSdkInitState() {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.InitJsonSchema = nil
	s.RegisteredHooks = nil
}

// Plan slug cache

func GetPlanSlugCache() map[string]string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]string)
	for k, v := range s.PlanSlugCache {
		result[k] = v
	}
	return result
}

// Session created teams

func GetSessionCreatedTeams() map[string]bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]bool)
	for k, v := range s.SessionCreatedTeams {
		result[k] = v
	}
	return result
}

// Teleported session

func SetTeleportedSessionInfo(sessionID string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TeleportedSessionInfo = &TeleportedSessionInfo{
		IsTeleported:          true,
		HasLoggedFirstMessage: false,
		SessionID:             sessionID,
	}
}

func GetTeleportedSessionInfo() *TeleportedSessionInfo {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TeleportedSessionInfo
}

func MarkFirstTeleportMessageLogged() {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.TeleportedSessionInfo != nil {
		s.TeleportedSessionInfo.HasLoggedFirstMessage = true
	}
}

// Invoked skills

func AddInvokedSkill(skillName, skillPath, content string, agentID *string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()

	key := ""
	if agentID != nil {
		key = *agentID
	}
	key += ":" + skillName

	s.InvokedSkills[key] = InvokedSkillInfo{
		SkillName: skillName,
		SkillPath: skillPath,
		Content:   content,
		InvokedAt: time.Now().UnixMilli(),
		AgentID:   agentID,
	}
}

func GetInvokedSkills() map[string]InvokedSkillInfo {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]InvokedSkillInfo)
	for k, v := range s.InvokedSkills {
		result[k] = v
	}
	return result
}

func GetInvokedSkillsForAgent(agentID *string) map[string]InvokedSkillInfo {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]InvokedSkillInfo)
	normalizedID := agentID
	for _, skill := range s.InvokedSkills {
		if skill.AgentID == nil && normalizedID == nil {
			result[skill.SkillName] = skill
		} else if skill.AgentID != nil && normalizedID != nil && *skill.AgentID == *normalizedID {
			result[skill.SkillName] = skill
		}
	}
	return result
}

func ClearInvokedSkills(preservedAgentIDs map[string]bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(preservedAgentIDs) == 0 {
		s.InvokedSkills = make(map[string]InvokedSkillInfo)
		return
	}

	for key, skill := range s.InvokedSkills {
		if skill.AgentID == nil {
			delete(s.InvokedSkills, key)
		} else if skill.AgentID != nil {
			if _, ok := preservedAgentIDs[*skill.AgentID]; !ok {
				delete(s.InvokedSkills, key)
			}
		}
	}
}

func ClearInvokedSkillsForAgent(agentID string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, skill := range s.InvokedSkills {
		if skill.AgentID != nil && *skill.AgentID == agentID {
			delete(s.InvokedSkills, key)
		}
	}
}

// Slow operations

const (
	MaxSlowOperations  = 10
	SlowOperationTTLms = 10000
)

func AddSlowOperation(operation string, durationMs float64) {
	if os.Getenv("USER_TYPE") != "ant" {
		return
	}
	// Skip tracking for editor sessions
	if operation == "exec" && len(operation) > 4 {
		return
	}

	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli()

	// Remove stale
	var fresh []SlowOperation
	for _, op := range s.SlowOperations {
		if now-op.Timestamp < SlowOperationTTLms {
			fresh = append(fresh, op)
		}
	}
	s.SlowOperations = fresh

	// Add new
	s.SlowOperations = append(s.SlowOperations, SlowOperation{
		Operation:  operation,
		DurationMs: durationMs,
		Timestamp:  now,
	})

	// Trim
	if len(s.SlowOperations) > MaxSlowOperations {
		s.SlowOperations = s.SlowOperations[len(s.SlowOperations)-MaxSlowOperations:]
	}
}

func GetSlowOperations() []SlowOperation {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.SlowOperations) == 0 {
		return []SlowOperation{}
	}

	now := time.Now().UnixMilli()
	var fresh []SlowOperation
	for _, op := range s.SlowOperations {
		if now-op.Timestamp < SlowOperationTTLms {
			fresh = append(fresh, op)
		}
	}
	return fresh
}

// Main thread agent type

func GetMainThreadAgentType() *string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.MainThreadAgentType
}

func SetMainThreadAgentType(agentType *string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.MainThreadAgentType = agentType
}

// Remote mode

func GetIsRemoteMode() bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.IsRemoteMode
}

func SetIsRemoteMode(value bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IsRemoteMode = value
}

// System prompt section cache

func GetSystemPromptSectionCache() map[string]*string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]*string)
	for k, v := range s.SystemPromptSectionCache {
		result[k] = v
	}
	return result
}

func SetSystemPromptSectionCacheEntry(name string, value *string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SystemPromptSectionCache[name] = value
}

func ClearSystemPromptSectionState() {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SystemPromptSectionCache = make(map[string]*string)
}

// Last emitted date

func GetLastEmittedDate() *string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LastEmittedDate
}

func SetLastEmittedDate(date *string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastEmittedDate = date
}

// Additional directories

func GetAdditionalDirectoriesForClaudeMd() []string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]string, len(s.AdditionalDirectoriesForClaudeMd))
	copy(result, s.AdditionalDirectoriesForClaudeMd)
	return result
}

func SetAdditionalDirectoriesForClaudeMd(directories []string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AdditionalDirectoriesForClaudeMd = directories
}

// Allowed channels

func GetAllowedChannels() []ChannelEntry {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ChannelEntry, len(s.AllowedChannels))
	copy(result, s.AllowedChannels)
	return result
}

func SetAllowedChannels(entries []ChannelEntry) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AllowedChannels = entries
}

func GetHasDevChannels() bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.HasDevChannels
}

func SetHasDevChannels(value bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.HasDevChannels = value
}

// Prompt cache 1h

func GetPromptCache1hAllowlist() []string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.PromptCache1hAllowlist == nil {
		return nil
	}
	result := make([]string, len(s.PromptCache1hAllowlist))
	copy(result, s.PromptCache1hAllowlist)
	return result
}

func SetPromptCache1hAllowlist(allowlist []string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	if allowlist == nil {
		s.PromptCache1hAllowlist = nil
		return
	}
	s.PromptCache1hAllowlist = make([]string, len(allowlist))
	copy(s.PromptCache1hAllowlist, allowlist)
}

func GetPromptCache1hEligible() *bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.PromptCache1hEligible
}

func SetPromptCache1hEligible(eligible *bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PromptCache1hEligible = eligible
}

// Beta header latches

func GetAfkModeHeaderLatched() *bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.AfkModeHeaderLatched
}

func SetAfkModeHeaderLatched(v bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AfkModeHeaderLatched = &v
}

func GetFastModeHeaderLatched() *bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.FastModeHeaderLatched
}

func SetFastModeHeaderLatched(v bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FastModeHeaderLatched = &v
}

func GetCacheEditingHeaderLatched() *bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CacheEditingHeaderLatched
}

func SetCacheEditingHeaderLatched(v bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CacheEditingHeaderLatched = &v
}

func GetThinkingClearLatched() *bool {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ThinkingClearLatched
}

func SetThinkingClearLatched(v bool) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ThinkingClearLatched = &v
}

func ClearBetaHeaderLatches() {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AfkModeHeaderLatched = nil
	s.FastModeHeaderLatched = nil
	s.CacheEditingHeaderLatched = nil
	s.ThinkingClearLatched = nil
}

// Prompt ID

func GetPromptId() *string {
	s := getState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.PromptId
}

func SetPromptId(id *string) {
	s := getState()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PromptId = id
}
