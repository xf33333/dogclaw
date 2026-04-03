package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// ========== Agent Context 定义 ==========

// AgentType 区分 agent 类型
type AgentType string

const (
	AgentTypeSubagent AgentType = "subagent"
	AgentTypeTeammate AgentType = "teammate"
)

// SubagentContext 子代理上下文（Agent tool 代理）
type SubagentContext struct {
	// 代理的唯一ID
	AgentID string
	// 父会话ID（CLAUDE_CODE_PARENT_SESSION_ID 环境变量），主REPL子代理可能为空
	ParentSessionID *string
	// 代理类型
	AgentType AgentType
	// 子代理的类型名称（如 "Explore", "Bash", "code-reviewer"）
	SubagentName *string
	// 是否为内置代理（true：内置；false：用户自定义）
	IsBuiltIn *bool
	// 调用此代理的 request_id（对于嵌套子代理，这是直接调用者，而非根）
	// 每次恢复时更新
	InvokingRequestID *string
	// 初始生成还是后续恢复：spawn（首次生成）或 resume（恢复）
	InvocationKind *string
	// 可变标志：此调用的边是否已发射到遥测
	// 每次生成/恢复时重置为 false；由 consumeInvokingRequestId 在首次终端API事件时翻转
	InvocationEmitted *bool
}

// TeammateAgentContext 队友代理上下文（Swarm 成员）
type TeammateAgentContext struct {
	// 完整代理ID，如 "researcher@my-team"
	AgentID string
	// 显示名称，如 "researcher"
	AgentName string
	// 所属团队名称
	TeamName string
	// UI 显示颜色
	AgentColor *string
	// 队友是否必须在实施前进入计划模式
	PlanModeRequired bool
	// 团队领导者的会话ID，用于转录关联
	ParentSessionID string
	// 此代理是否为团队领导者
	IsTeamLead bool
	// 代理类型
	AgentType AgentType
	// invoking agent 的 request_id，该 agent 生成或恢复了此队友
	// 由工具调用外启动的队友（例如会话启动）则为 undefined
	InvokingRequestID *string
	// 同 SubagentContext.invocationKind
	InvocationKind *string
	// 同 SubagentContext.invocationEmitted
	InvocationEmitted *bool
}

// AgentContext 代理上下文类型（可以是 subagent 或 teammate）
type AgentContext interface {
	GetAgentType() AgentType
	GetAgentID() string
	GetParentSessionID() *string
	GetInvokingRequestID() *string
	GetInvocationKind() *string
	IsInvocationEmitted() bool
	SetInvocationEmitted(emitted bool)
}

// GetAgentType 返回代理类型
func (s *SubagentContext) GetAgentType() AgentType      { return s.AgentType }
func (t *TeammateAgentContext) GetAgentType() AgentType { return t.AgentType }

// GetAgentID 返回代理ID
func (s *SubagentContext) GetAgentID() string      { return s.AgentID }
func (t *TeammateAgentContext) GetAgentID() string { return t.AgentID }

// GetParentSessionID 返回父会话ID
func (s *SubagentContext) GetParentSessionID() *string      { return s.ParentSessionID }
func (t *TeammateAgentContext) GetParentSessionID() *string { return &t.ParentSessionID }

// GetInvokingRequestID 返回调用者的请求ID
func (s *SubagentContext) GetInvokingRequestID() *string      { return s.InvokingRequestID }
func (t *TeammateAgentContext) GetInvokingRequestID() *string { return t.InvokingRequestID }

// GetInvocationKind 返回调用类型
func (s *SubagentContext) GetInvocationKind() *string      { return s.InvocationKind }
func (t *TeammateAgentContext) GetInvocationKind() *string { return t.InvocationKind }

// IsInvocationEmitted 返回是否已发射
func (s *SubagentContext) IsInvocationEmitted() bool      { return *s.InvocationEmitted }
func (t *TeammateAgentContext) IsInvocationEmitted() bool { return *t.InvocationEmitted }

// SetInvocationEmitted 设置发射状态
func (s *SubagentContext) SetInvocationEmitted(emitted bool) {
	if s.InvocationEmitted != nil {
		*s.InvocationEmitted = emitted
	}
}
func (t *TeammateAgentContext) SetInvocationEmitted(emitted bool) {
	if t.InvocationEmitted != nil {
		*t.InvocationEmitted = emitted
	}
}

// ========== Context 键和存储 ==========

type agentContextKey struct{}

// contextWithAgent 将代理上下文存储到 context 中
func contextWithAgent(parent context.Context, ctx AgentContext) context.Context {
	return context.WithValue(parent, agentContextKey{}, ctx)
}

// GetAgentContext 从 context 中获取代理上下文
// 如果当前不在任何代理上下文中，返回 nil
func GetAgentContext(ctx context.Context) AgentContext {
	if val, ok := ctx.Value(agentContextKey{}).(AgentContext); ok {
		return val
	}
	return nil
}

// IsSubagentContext 检查上下文是否为子代理上下文
func IsSubagentContext(ctx context.Context) bool {
	if agentCtx := GetAgentContext(ctx); agentCtx != nil {
		return agentCtx.GetAgentType() == AgentTypeSubagent
	}
	return false
}

// IsTeammateAgentContext 检查上下文是否为队友代理上下文
func IsTeammateAgentContext(ctx context.Context) bool {
	if agentCtx := GetAgentContext(ctx); agentCtx != nil {
		return agentCtx.GetAgentType() == AgentTypeTeammate
	}
	return false
}

// GetAgentIDFromContext 从上下文中获取代理ID
func GetAgentIDFromContext(ctx context.Context) *string {
	if agentCtx := GetAgentContext(ctx); agentCtx != nil {
		id := agentCtx.GetAgentID()
		return &id
	}
	return nil
}

// GetSubagentNameFromContext 获取子代理名称（用于日志/遥测）
// 返回：内置代理返回类型名称，自定义代理返回"user-defined"，不在子代理上下文返回nil
func GetSubagentLogName(ctx context.Context) *string {
	agentCtx := GetAgentContext(ctx)
	if agentCtx == nil || agentCtx.GetAgentType() != AgentTypeSubagent {
		return nil
	}

	subagentCtx, ok := agentCtx.(*SubagentContext)
	if !ok || subagentCtx.SubagentName == nil {
		return nil
	}

	if subagentCtx.IsBuiltIn != nil && *subagentCtx.IsBuiltIn {
		return subagentCtx.SubagentName
	}
	userDefined := "user-defined"
	return &userDefined
}

// ConsumeInvokingRequestID 消费 invokingRequestId ——每个调用仅返回一次
// 首次生成/恢复后返回边标记数据，之后返回 nil 直到下一个边界
// 主线程或初始调用没有 request_id 时也会返回 nil
func ConsumeInvokingRequestID(ctx context.Context) *InvocationEdgeInfo {
	agentCtx := GetAgentContext(ctx)
	if agentCtx == nil {
		return nil
	}

	invokingID := agentCtx.GetInvokingRequestID()
	if invokingID == nil || *invokingID == "" {
		return nil
	}

	if agentCtx.IsInvocationEmitted() {
		return nil
	}

	agentCtx.SetInvocationEmitted(true)
	kind := agentCtx.GetInvocationKind()

	return &InvocationEdgeInfo{
		InvokingRequestID: *invokingID,
		InvocationKind:    *kind,
	}
}

// InvocationEdgeInfo 调用边信息（用于遥测）
type InvocationEdgeInfo struct {
	InvokingRequestID string
	InvocationKind    string // "spawn" 或 "resume"
}

// ========== Agent ID 工具函数（从 utils/agentId.ts 翻译） ==========

// FormatAgentID 格式化为代理ID：agentName@teamName
func FormatAgentID(agentName, teamName string) string {
	return fmt.Sprintf("%s@%s", agentName, teamName)
}

// ParseAgentID 解析代理ID
// 返回：agentName 和 teamName；无法解析返回 nil
func ParseAgentID(agentID string) *struct {
	AgentName string
	TeamName  string
} {
	atIndex := strings.Index(agentID, "@")
	if atIndex == -1 {
		return nil
	}
	return &struct {
		AgentName string
		TeamName  string
	}{
		AgentName: agentID[:atIndex],
		TeamName:  agentID[atIndex+1:],
	}
}

// GenerateRequestID 生成请求ID：{requestType}-{timestamp}@{agentId}
func GenerateRequestID(requestType, agentID string) string {
	timestamp := TimeNowMillis()
	return fmt.Sprintf("%s-%d@%s", requestType, timestamp)
}

// ParseRequestID 解析请求ID
// 返回：requestType, timestamp, agentID；无法解析返回 nil
func ParseRequestID(requestID string) *struct {
	RequestType string
	Timestamp   int64
	AgentID     string
} {
	atIndex := strings.Index(requestID, "@")
	if atIndex == -1 {
		return nil
	}

	prefix := requestID[:atIndex]
	agentID := requestID[atIndex+1:]

	dashIndex := strings.LastIndex(prefix, "-")
	if dashIndex == -1 {
		return nil
	}

	requestType := prefix[:dashIndex]
	timestampStr := prefix[dashIndex+1:]

	var timestamp int64
	fmt.Sscanf(timestampStr, "%d", &timestamp)
	if timestamp == 0 {
		return nil
	}

	return &struct {
		RequestType string
		Timestamp   int64
		AgentID     string
	}{
		RequestType: requestType,
		Timestamp:   timestamp,
		AgentID:     agentID,
	}
}

// TimeNowMillis 获取当前时间的毫秒数
func TimeNowMillis() int64 {
	return timeNow().UnixNano() / 1e6
}

var timeNow = func() time.Time { return time.Now() }

// ========== Agent Swarms 启用检查（从 utils/agentSwarmsEnabled.ts 翻译） ==========

// IsAgentSwarmsEnabled 检查是否启用了 agent swarms 功能
// 1. Ant 构建：始终启用
// 2. 外部构建：需要同时满足以下条件
//   - 通过 CLI 标志 --agent-teams 或环境变量 CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS 启用
//   - GrowthBook 功能标记 'tengu_amber_flint' 启用（killswitch）
func IsAgentSwarmsEnabled() bool {
	// Ant：始终启用
	if isEnvTruthy(os.Getenv("USER_TYPE")) {
		return true
	}

	// 外部：需要显式启用
	if !isEnvTruthy(os.Getenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS")) &&
		!IsAgentTeamsFlagSet() {
		return false
	}

	// Killswitch —— 外部用户始终尊重
	if !getFeatureValue_CACHED_MAY_BE_STALE("tengu_amber_flint", true) {
		return false
	}

	return true
}

// IsAgentTeamsFlagSet 检查是否设置了 --agent-teams CLI 标志
// TODO: 实际集成 CLI 标志解析
func IsAgentTeamsFlagSet() bool {
	// 占位符实现：检查环境变量作为备选
	return isEnvTruthy(os.Getenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS"))
}

// getFeatureValue_CACHED_MAY_BE_STALE 获取 GrowthBook 功能标记值（占位符）
// TODO: 实际集成 GrowthBook
func getFeatureValue_CACHED_MAY_BE_STALE(featureName string, defaultValue bool) bool {
	// 这里应该调用实际的 GrowthBook 服务
	// 暂时返回默认值
	return defaultValue
}

// ========== 工具函数 ==========

// GetSubagentAnalyticsName 获取适用于遥测的子代理名称
// 内置代理使用代码常量，自定义代理映射到字面量 "user-defined"
func GetSubagentAnalyticsName(ctx context.Context) *string {
	name := GetSubagentLogName(ctx)
	if name == nil {
		return nil
	}
	return name
}

// ========== 创建函数 ==========

// NewSubagentContext 创建新的子代理上下文
func NewSubagentContext(agentID string, parentSessionID *string, subagentName string, isBuiltIn bool) *SubagentContext {
	emitted := false
	return &SubagentContext{
		AgentID:           agentID,
		ParentSessionID:   parentSessionID,
		AgentType:         AgentTypeSubagent,
		SubagentName:      &subagentName,
		IsBuiltIn:         &isBuiltIn,
		InvocationEmitted: &emitted,
	}
}

// NewTeammateAgentContext 创建新的队友代理上下文
func NewTeammateAgentContext(agentID, agentName, teamName, parentSessionID string, isTeamLead bool) *TeammateAgentContext {
	emitted := false
	return &TeammateAgentContext{
		AgentID:           agentID,
		AgentName:         agentName,
		TeamName:          teamName,
		ParentSessionID:   parentSessionID,
		IsTeamLead:        isTeamLead,
		AgentType:         AgentTypeTeammate,
		InvocationEmitted: &emitted,
	}
}
