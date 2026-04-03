package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"dogclaw/pkg/types"
)

// AgentExecuteParams Agent执行参数
type AgentExecuteParams struct {
	AgentType      string
	AgentID        string
	SystemPrompt   string
	Tools          []string
	ParentCtx      context.Context
	SessionID      string
	PermissionMode types.PermissionMode
	Model          string
	MaxTurns       int
	InitialPrompt  string
}

// AgentResult Agent执行结果
type AgentResult struct {
	Output     string
	ToolsUsed  []string
	TurnCount  int
	TokensUsed int
	Duration   time.Duration
	IsComplete bool
	ErrorMsg   string
}

// AgentManager 管理 Agent 的创建和执行
type AgentManager struct {
	mu          sync.RWMutex
	definitions []Definition
	agentDefs   map[string]Definition // agentType -> definition
	colorMgr    *AgentColorManager
	swarmMgr    *SwarmManager
}

// Definition Agent定义接口
type Definition interface {
	GetAgentType() string
	GetSystemPrompt() string
	GetTools() []string
	GetDisallowedTools() []string
	GetModel() string
	GetEffort() interface{}
	GetPermissionMode() types.PermissionMode
	GetMaxTurns() *int
	GetBackground() bool
	GetMemory() types.AgentMemoryScope
	GetInitialPrompt() string
}

// NewAgentManager 创建Agent管理器
func NewAgentManager() *AgentManager {
	return &AgentManager{
		agentDefs: make(map[string]Definition),
		colorMgr:  NewAgentColorManager(),
		swarmMgr:  NewSwarmManager(),
	}
}

// LoadDefinitions 加载Agent定义
func (m *AgentManager) LoadDefinitions(definitions []Definition) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.definitions = definitions
	m.agentDefs = make(map[string]Definition)

	for _, def := range definitions {
		m.agentDefs[def.GetAgentType()] = def
	}
}

// GetAgent 获取Agent定义
func (m *AgentManager) GetAgent(agentType string) (Definition, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	def, ok := m.agentDefs[agentType]
	if !ok {
		return nil, fmt.Errorf("agent type '%s' not found", agentType)
	}
	return def, nil
}

// ListAgents 列出所有可用的Agent
func (m *AgentManager) ListAgents() []Definition {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Definition, 0, len(m.definitions))
	for _, def := range m.definitions {
		result = append(result, def)
	}
	return result
}

// ExecuteAgent 执行一个Agent（简化版本）
func (m *AgentManager) ExecuteAgent(parentCtx context.Context, params AgentExecuteParams) (*AgentResult, error) {
	startTime := time.Now()

	// 1. 获取Agent定义
	def, err := m.GetAgent(params.AgentType)
	if err != nil {
		return nil, err
	}

	// 2. 计算有效工具集
	_, err = m.resolveEffectiveTools(def)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve tools: %w", err)
	}

	// 3. 解析模型
	parentModel := "claude-3-opus-20240229" // TODO: 从上下文中获取
	effectiveModel := GetAgentModel(def.GetModel(), parentModel, &params.Model, def.GetPermissionMode())
	_ = effectiveModel // TODO: 使用 effectiveModel

	// 4. 创建子上下文
	_ = parentCtx // 在实际实现中会创建带Agent context的context

	// 5. 执行循环（简化）
	result := &AgentResult{
		ToolsUsed:  make([]string, 0),
		IsComplete: false,
	}

	// 这里应该实现实际的 agent 执行循环:
	// - 构建消息
	// - 调用 LLM
	// - 处理工具调用
	// - 循环直到完成或达到最大回合数

	// 简化实现：返回一个模拟结果
	result.Output = fmt.Sprintf("Agent %s executed with model %s", params.AgentType, effectiveModel)
	result.TurnCount = 1
	result.Duration = time.Since(startTime)
	result.IsComplete = true

	return result, nil
}

// resolveEffectiveTools 解析Agent的有效工具集
func (m *AgentManager) resolveEffectiveTools(def Definition) ([]string, error) {
	agentTools := def.GetTools()
	disallowed := def.GetDisallowedTools()

	// 如果 tools 为 nil 或空，表示允许所有工具（系统需要提供完整工具列表）
	// 这里我们简化：返回允许的工具
	if len(agentTools) == 0 {
		return []string{}, nil // 应该返回所有可用工具
	}

	// 如果有通配符 "*"，也返回所有工具
	for _, tool := range agentTools {
		if tool == "*" {
			return []string{}, nil // 应该返回所有可用工具
		}
	}

	// 过滤被禁止的工具
	if len(disallowed) == 0 {
		return agentTools, nil
	}

	disallowedSet := make(map[string]bool)
	for _, tool := range disallowed {
		disallowedSet[tool] = true
	}

	allowed := make([]string, 0, len(agentTools))
	for _, tool := range agentTools {
		if !disallowedSet[tool] {
			allowed = append(allowed, tool)
		}
	}

	return allowed, nil
}

// GetAgentColor 获取Agent颜色
func (m *AgentManager) GetAgentColor(agentName string, def Definition) string {
	color := m.colorMgr.GetAgentColor(agentName, def)
	return string(color)
}

//  ========== Swarm 相关方法 ==========

// GetSwarmManager 获取Swarm管理器
func (m *AgentManager) GetSwarmManager() *SwarmManager {
	return m.swarmMgr
}

// CreateTeammate 创建队友代理
func (m *AgentManager) CreateTeammate(name, team string, opts TeammateOptions) (*Teammate, error) {
	agentID := FormatAgentID(name, team)

	teammate := &Teammate{
		ID:               agentID,
		Name:             name,
		Team:             team,
		Color:            string(m.colorMgr.GetAgentColor(name, nil)),
		PlanModeRequired: opts.PlanModeRequired,
		IsLead:           opts.IsLead,
		ParentSessionID:  opts.ParentSessionID,
		MaxTurns:         opts.MaxTurns,
		Background:       opts.Background,
		Memory:           opts.Memory,
		Isolation:        opts.Isolation,
		Model:            opts.Model,
		InitialPrompt:    opts.InitialPrompt,
		Tools:            opts.Tools,
		DisallowedTools:  opts.DisallowedTools,
	}

	if err := m.swarmMgr.RegisterTeammate(teammate); err != nil {
		return nil, err
	}

	return teammate, nil
}

// ========== Agent 辅助函数 ==========

// ShouldInjectAutoMemory 检查是否应该注入自动记忆工具
func ShouldInjectAutoMemory(def Definition) bool {
	memory := def.GetMemory()
	return memory != "" && memory != types.MemoryLocal
}

// RequiresFileToolsForMemory 需要哪些文件工具来支持记忆
func RequiresFileToolsForMemory(scope types.AgentMemoryScope) []string {
	return []string{"FileWrite", "FileEdit", "FileRead"}
}

// GetAgentCapabilities 获取Agent能力摘要
func GetAgentCapabilities(def Definition) map[string]interface{} {
	cap := map[string]interface{}{
		"agentType":      def.GetAgentType(),
		"tools":          def.GetTools(),
		"model":          def.GetModel(),
		"permissionMode": def.GetPermissionMode(),
		"background":     def.GetBackground(),
		"memory":         def.GetMemory(),
	}

	if maxTurns := def.GetMaxTurns(); maxTurns != nil {
		cap["maxTurns"] = *maxTurns
	}

	return cap
}

// ========== TeammateOptions 创建队友选项 ==========

type TeammateOptions struct {
	PlanModeRequired bool
	IsLead           bool
	ParentSessionID  string
	MaxTurns         int
	Background       bool
	Memory           types.AgentMemoryScope
	Isolation        string
	Model            string
	InitialPrompt    string
	Tools            []string
	DisallowedTools  []string
}

// DefaultTeammateOptions 默认队友选项
func DefaultTeammateOptions() *TeammateOptions {
	return &TeammateOptions{
		PlanModeRequired: false,
		IsLead:           false,
		MaxTurns:         0, // 无限制
		Background:       false,
		Memory:           types.MemoryProject,
		Isolation:        "", // 默认无隔离
		Model:            "inherit",
	}
}
