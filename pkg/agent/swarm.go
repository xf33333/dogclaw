package agent

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"dogclaw/pkg/types"
)

// ========== Agent Swarm / Teammate 系统（参考 teammate.ts） ==========

// Teammate 队友代理配置
type Teammate struct {
	// 唯一ID (格式: agentName@teamName)
	ID string
	// 显示名称
	Name string
	// 所属团队
	Team string
	// UI颜色
	Color string
	// 是否必须在实施前进入计划模式
	PlanModeRequired bool
	// 是否是团队领导
	IsLead bool
	// 父会话ID（用于转录关联）
	ParentSessionID string
	// 最大回合数（0=无限制）
	MaxTurns int
	// 是否后台运行
	Background bool
	// Memory 内存作用域
	Memory types.AgentMemoryScope
	// 隔离模式
	Isolation string
	// 模型设置
	Model string
	// 初始提示
	InitialPrompt string
	// 是否允许使用所有工具
	Tools []string
	// 禁止的工具
	DisallowedTools []string
	// 技能列表
	Skills []string
	// MCP服务器
	McpServers []interface{}
	// 首次创建时间
	CreatedAt int64
	// 最后活跃时间
	LastActiveAt int64
	// 会话次数
	SessionCount int
}

// Team 团队配置
type Team struct {
	// 团队名称
	Name string
	// 团队成员ID列表
	Members []string
	// 团队领导的ID
	LeaderID string
	// 创建时间
	CreatedAt int64
	// 是否允许新成员
	AllowJoins bool
	// 最小成员数（用于团队恢复）
	MinMembers int
}

// SwarmManager 管理 swarm 和 teammate 生命周期
type SwarmManager struct {
	mu          sync.RWMutex
	teammates   map[string]*Teammate // ID -> Teammate
	teams       map[string]*Team     // Name -> Team
	agentColorM *AgentColorManager
}

// NewSwarmManager 创建新的 swarm 管理器
func NewSwarmManager() *SwarmManager {
	return &SwarmManager{
		teammates:   make(map[string]*Teammate),
		teams:       make(map[string]*Team),
		agentColorM: NewAgentColorManager(),
	}
}

// RegisterTeammate 注册一个 teammate
func (s *SwarmManager) RegisterTeammate(t *Teammate) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if t.ID == "" {
		return fmt.Errorf("teammate ID cannot be empty")
	}

	// 设置时间戳
	if t.CreatedAt == 0 {
		t.CreatedAt = time.Now().UnixNano() / 1e6
	}
	t.LastActiveAt = time.Now().UnixNano() / 1e6

	// 确保颜色已分配
	if t.Color == "" {
		t.Color = string(s.agentColorM.GetAgentColor(t.Name, nil))
	}

	s.teammates[t.ID] = t
	return nil
}

// GetTeammate 获取 teammate
func (s *SwarmManager) GetTeammate(id string) (*Teammate, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.teammates[id]
	return t, ok
}

// UpdateTeammateActivity 更新 teammate 活跃状态
func (s *SwarmManager) UpdateTeammateActivity(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if t, ok := s.teammates[id]; ok {
		t.LastActiveAt = time.Now().UnixNano() / 1e6
		t.SessionCount++
	}
}

// RemoveTeammate 移除 teammate
func (s *SwarmManager) RemoveTeammate(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.teammates, id)
}

// ListTeammates 列出所有 teammate
func (s *SwarmManager) ListTeammates() []*Teammate {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Teammate, 0, len(s.teammates))
	for _, t := range s.teammates {
		result = append(result, t)
	}
	return result
}

// GetTeammatesByTeam 获取指定团队的所有 teammate
func (s *SwarmManager) GetTeammatesByTeam(teamName string) []*Teammate {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Teammate
	for _, t := range s.teammates {
		if t.Team == teamName {
			result = append(result, t)
		}
	}
	return result
}

// ========== Team 管理 ==========

// CreateTeam 创建团队
func (s *SwarmManager) CreateTeam(teamName string, leaderID string) (*Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if teamName == "" {
		return nil, fmt.Errorf("team name cannot be empty")
	}

	if _, exists := s.teams[teamName]; exists {
		return nil, fmt.Errorf("team '%s' already exists", teamName)
	}

	team := &Team{
		Name:       teamName,
		Members:    []string{leaderID},
		LeaderID:   leaderID,
		CreatedAt:  time.Now().UnixNano() / 1e6,
		AllowJoins: true,
		MinMembers: 1,
	}

	s.teams[teamName] = team
	return team, nil
}

// DeleteTeam 删除团队
func (s *SwarmManager) DeleteTeam(teamName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.teams[teamName]; !exists {
		return fmt.Errorf("team '%s' not found", teamName)
	}

	delete(s.teams, teamName)
	return nil
}

// GetTeam 获取团队信息
func (s *SwarmManager) GetTeam(teamName string) (*Team, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	team, ok := s.teams[teamName]
	return team, ok
}

// ListTeams 列出所有团队
func (s *SwarmManager) ListTeams() []*Team {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Team, 0, len(s.teams))
	for _, t := range s.teams {
		result = append(result, t)
	}
	return result
}

// JoinTeam 让 teammate 加入团队
func (s *SwarmManager) JoinTeam(teammateID, teamName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查 teammate 是否存在
	t, ok := s.teammates[teammateID]
	if !ok {
		return fmt.Errorf("teammate '%s' not found", teammateID)
	}

	// 检查团队是否存在
	team, ok := s.teams[teamName]
	if !ok {
		return fmt.Errorf("team '%s' not found", teamName)
	}

	// 检查是否允许加入
	if !team.AllowJoins {
		return fmt.Errorf("team '%s' does not allow new members", teamName)
	}

	// 更新 teammate 的团队
	t.Team = teamName

	// 添加到团队成员列表（如果不存在）
	for _, memberID := range team.Members {
		if memberID == teammateID {
			return nil // 已经是成员
		}
	}
	team.Members = append(team.Members, teammateID)

	return nil
}

// LeaveTeam 让 teammate 离开团队
func (s *SwarmManager) LeaveTeam(teammateID, teamName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	team, ok := s.teams[teamName]
	if !ok {
		return fmt.Errorf("team '%s' not found", teamName)
	}

	// 从团队成员列表中移除
	newMembers := make([]string, 0, len(team.Members))
	for _, memberID := range team.Members {
		if memberID != teammateID {
			newMembers = append(newMembers, memberID)
		}
	}
	team.Members = newMembers

	// 如果离开的是领导，需要指定新领导或解散团队
	if team.LeaderID == teammateID {
		if len(team.Members) > 0 {
			// 选择第一个成员作为新领导
			team.LeaderID = team.Members[0]
		} else {
			// 没有成员了，删除团队
			delete(s.teams, teamName)
		}
	}

	return nil
}

// ========== 工具函数 ==========

// SanitizeAgentName 清理代理名称（移除非法字符）
func SanitizeAgentName(name string) string {
	// 移除 @ 符号（用于ID分隔）
	name = strings.ReplaceAll(name, "@", "")
	// 移除空白字符
	name = strings.TrimSpace(name)
	return name
}

// ValidateAgentID 验证代理ID格式（agentName@teamName）
func ValidateAgentID(id string) error {
	if id == "" {
		return fmt.Errorf("agent ID cannot be empty")
	}

	parsed := ParseAgentID(id)
	if parsed == nil {
		return fmt.Errorf("invalid agent ID format: must be 'agentName@teamName'")
	}

	if parsed.AgentName == "" {
		return fmt.Errorf("agent name cannot be empty")
	}
	if parsed.TeamName == "" {
		return fmt.Errorf("team name cannot be empty")
	}

	return nil
}

// GetActiveTeammates 获取活跃的 teammate（按最后活动时间过滤）
func (s *SwarmManager) GetActiveTeammates(thresholdMinutes int) []*Teammate {
	s.mu.RLock()
	defer s.mu.RUnlock()

	threshold := time.Now().UnixNano()/1e6 - int64(thresholdMinutes*60)

	var active []*Teammate
	for _, t := range s.teammates {
		if t.LastActiveAt >= threshold {
			active = append(active, t)
		}
	}
	return active
}

// ========== 环境变量检查 ==========

// GetParentSessionID 从环境变量获取父会话ID
func GetParentSessionID() string {
	return os.Getenv("CLAUDE_CODE_PARENT_SESSION_ID")
}

// GetAgentIDFromEnv 从环境变量获取当前代理ID
func GetAgentIDFromEnv() string {
	return os.Getenv("CLAUDE_CODE_AGENT_ID")
}

// SetAgentEnvVars 设置代理相关的环境变量
func SetAgentEnvVars(agentID, parentSessionID string) {
	os.Setenv("CLAUDE_CODE_AGENT_ID", agentID)
	if parentSessionID != "" {
		os.Setenv("CLAUDE_CODE_PARENT_SESSION_ID", parentSessionID)
	}
}

// ClearAgentEnvVars 清除代理环境变量
func ClearAgentEnvVars() {
	os.Unsetenv("CLAUDE_CODE_AGENT_ID")
	os.Unsetenv("CLAUDE_CODE_PARENT_SESSION_ID")
}
