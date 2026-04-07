package tools

import (
	"context"
	"fmt"

	"dogclaw/pkg/agent"
	"dogclaw/pkg/types"
)

// AgentTool 用于调用子代理（类似 TS 版本的 AgentTool）
type AgentTool struct {
	agentManager *agent.AgentManager
}

// NewAgentTool 创建新的 AgentTool
func NewAgentTool(agentManager *agent.AgentManager) *AgentTool {
	return &AgentTool{
		agentManager: agentManager,
	}
}

func (t *AgentTool) Name() string      { return "Agent" }
func (t *AgentTool) Aliases() []string { return []string{"agent", "subagent", "fork"} }

func (t *AgentTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"agent_type": map[string]any{
				"type":        "string",
				"description": "Type of agent to spawn (e.g., 'explore', 'plan', 'code-reviewer')",
			},
			"initial_prompt": map[string]any{
				"type":        "string",
				"description": "Initial prompt for the agent",
			},
			"context": map[string]any{
				"type":        "string",
				"description": "Additional context for the agent",
			},
		},
		Required: []string{"agent_type"},
	}
}

func (t *AgentTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Spawn a sub-agent to handle a specialized task. Agents can use a focused set of tools and have their own system prompt."
}

func (t *AgentTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	agentType, ok := input["agent_type"].(string)
	if !ok || agentType == "" {
		return &types.ToolResult{
			Data:    "Error: 'agent_type' parameter is required",
			IsError: true,
		}, nil
	}

	initialPrompt, _ := input["initial_prompt"].(string)
	contextStr, _ := input["context"].(string)

	// 获取当前工作目录作为 session 标识
	sessionID := toolCtx.Cwd
	if sessionID == "" {
		sessionID = "default"
	}

	// 生成代理ID
	agentID := agent.FormatAgentID(agentType, sessionID)

	// 创建子代理上下文
	subagentCtx := agent.NewSubagentContext(
		agentID,
		nil, // parentSessionID - 暂时设为 nil
		agentType,
		true, // 假设为内置代理，实际应该查表
	)

	// 设置调用信息
	invocationKind := "spawn"
	subagentCtx.InvocationKind = &invocationKind

	// 创建带代理上下文的新 context
	agentCtx := context.WithValue(ctx, struct{}{}, subagentCtx)
	_ = agentCtx // TODO: 实现正确的 agent context 传播

	// 准备 Agent 执行参数
	agentDef, err := t.agentManager.GetAgent(agentType)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: failed to get agent definition: %v", err),
			IsError: true,
		}, nil
	}

	// 构建 Agent 的系统提示
	systemPrompt := agentDef.GetSystemPrompt()
	if contextStr != "" {
		systemPrompt += "\n\nAdditional Context:\n" + contextStr
	}
	if initialPrompt != "" {
		systemPrompt += "\n\nInitial Task:\n" + initialPrompt
	}

	// 构造 Agent 执行参数
	permMode := agentDef.GetPermissionMode()
	maxTurns := 20
	if agentDef.GetMaxTurns() != nil {
		maxTurns = *agentDef.GetMaxTurns()
	}
	execParams := agent.AgentExecuteParams{
		AgentType:      agentType,
		AgentID:        agentID,
		SystemPrompt:   systemPrompt,
		Tools:          agentDef.GetTools(),
		ParentCtx:      ctx,
		SessionID:      sessionID,
		PermissionMode: permMode,
		Model:          agentDef.GetModel(),
		MaxTurns:       maxTurns,
	}

	// 执行 Agent
	result, err := t.agentManager.ExecuteAgent(ctx, execParams)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error: agent execution failed: %v", err),
			IsError: true,
		}, nil
	}

	// 返回结果
	return &types.ToolResult{
		Data:    fmt.Sprintf("Agent %s completed.\n\nResult: %s", agentType, result.Output),
		IsError: false,
	}, nil
}

func (t *AgentTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *AgentTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *AgentTool) IsDestructive(input map[string]any) bool     { return false }
func (t *AgentTool) IsEnabled() bool                             { return agent.IsAgentSwarmsEnabled() }
func (t *AgentTool) SearchHint() string                          { return "agent subagent fork delegate task" }
