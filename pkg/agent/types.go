package agent

import (
	"context"
)

// ========== 常量和类型定义 ==========

// SettingSource 定义配置来源
type SettingSource string

const (
	SourceBuiltIn SettingSource = "built-in"
	SourcePlugin  SettingSource = "plugin"
	SourceUser    SettingSource = "userSettings"
	SourceProject SettingSource = "projectSettings"
	SourcePolicy  SettingSource = "policySettings"
	SourceFlag    SettingSource = "flagSettings"
)

// AgentColorName 表示有效的颜色名称
type AgentColorName string

const (
	ColorBlue    AgentColorName = "blue"
	ColorGreen   AgentColorName = "green"
	ColorRed     AgentColorName = "red"
	ColorYellow  AgentColorName = "yellow"
	ColorMagenta AgentColorName = "magenta"
	ColorCyan    AgentColorName = "cyan"
	ColorGray    AgentColorName = "gray"
	ColorDefault AgentColorName = "default"
)

var ValidAgentColors = []AgentColorName{
	ColorBlue, ColorGreen, ColorRed, ColorYellow,
	ColorMagenta, ColorCyan, ColorGray, ColorDefault,
}

// AgentMemoryScope 定义 agent 记忆作用域
type AgentMemoryScope string

const (
	MemoryUser    AgentMemoryScope = "user"
	MemoryProject AgentMemoryScope = "project"
	MemoryLocal   AgentMemoryScope = "local"
)

// PermissionMode 定义权限模式
type PermissionMode string

const (
	PermissionModeDefault  PermissionMode = "default"
	PermissionModeReadOnly PermissionMode = "read-only"
	PermissionModeFull     PermissionMode = "full"
)

// EffortLevel 定义努力级别
type EffortLevel string

const (
	EffortLow    EffortLevel = "low"
	EffortMedium EffortLevel = "medium"
	EffortHigh   EffortLevel = "high"
	EffortAuto   EffortLevel = "auto"
)

// ========== Agent 定义相关类型 ==========

// BaseAgentDefinition 包含所有 agent 共有的字段
type BaseAgentDefinition struct {
	AgentType       string           `yaml:"name" json:"name"`
	WhenToUse       string           `yaml:"description" json:"description"`
	Tools           []string         `yaml:"tools,omitempty" json:"tools,omitempty"`
	DisallowedTools []string         `yaml:"disallowedTools,omitempty" json:"disallowedTools,omitempty"`
	Skills          []string         `yaml:"skills,omitempty" json:"skills,omitempty"`
	McpServers      []any            `yaml:"mcpServers,omitempty" json:"mcpServers,omitempty"`
	Hooks           *HooksSettings   `yaml:"hooks,omitempty" json:"hooks,omitempty"`
	Color           AgentColorName   `yaml:"color,omitempty" json:"color,omitempty"`
	Model           string           `yaml:"model,omitempty" json:"model,omitempty"`
	Effort          interface{}      `yaml:"effort,omitempty" json:"effort,omitempty"` // string or int
	PermissionMode  PermissionMode   `yaml:"permissionMode,omitempty" json:"permissionMode,omitempty"`
	MaxTurns        *int             `yaml:"maxTurns,omitempty" json:"maxTurns,omitempty"`
	Background      bool             `yaml:"background,omitempty" json:"background,omitempty"`
	Memory          AgentMemoryScope `yaml:"memory,omitempty" json:"memory,omitempty"`
	Isolation       string           `yaml:"isolation,omitempty" json:"isolation,omitempty"` // "worktree" or "remote"
	InitialPrompt   string           `yaml:"initialPrompt,omitempty" json:"initialPrompt,omitempty"`
	OmitClaudeMd    bool             `yaml:"omitClaudeMd,omitempty" json:"omitClaudeMd,omitempty"`

	// 内部字段
	Filename     string        `json:"filename,omitempty"` // 原始文件名（不含路径）
	BaseDir      string        `json:"baseDir,omitempty"`  // 基础目录
	Source       SettingSource `json:"source,omitempty"`   // 来源：built-in, plugin, userSettings, projectSettings, policySettings
	SystemPrompt string        `json:"-"`                  // 完整的系统提示（由内容生成）
}

// CustomAgentDefinition 用户/项目/策略定义的 agent
type CustomAgentDefinition struct {
	BaseAgentDefinition
	Source SettingSource `json:"source"` // userSettings, projectSettings, policySettings, flagSettings
}

// BuiltInAgentDefinition 内置 agent
type BuiltInAgentDefinition struct {
	BaseAgentDefinition
	Source           SettingSource
	Callback         func()
	SystemPromptFunc func(params BuiltInPromptParams) string
}

// BuiltInPromptParams 获取内置 agent 系统提示的参数
type BuiltInPromptParams struct {
	ToolUseContext ToolUseContext
}

// PluginAgentDefinition 插件 agent
type PluginAgentDefinition struct {
	BaseAgentDefinition
	Source string `json:"source"` // "plugin"
	Plugin string `json:"plugin"` // 插件名称
}

// AgentDefinition 所有 agent 定义类型的联合接口
type AgentDefinition interface {
	GetAgentType() string
	GetWhenToUse() string
	GetSystemPrompt() string
	GetSource() string
	GetBaseDir() string
	GetFilename() string
	GetTools() []string
	GetDisallowedTools() []string
	GetSkills() []string
	GetMcpServers() []any
	GetHooks() *HooksSettings
	GetColor() AgentColorName
	GetModel() string
	GetEffort() interface{}
	GetPermissionMode() PermissionMode
	GetMaxTurns() *int
	GetBackground() bool
	GetMemory() AgentMemoryScope
	GetIsolation() string
	GetInitialPrompt() string
	GetOmitClaudeMd() bool
	GetBaseAgentDefinition() *BaseAgentDefinition
}

// AgentDefinitionsResult 返回 agent 定义的结果
type AgentDefinitionsResult struct {
	ActiveAgents      []AgentDefinition `json:"activeAgents"`
	AllAgents         []AgentDefinition `json:"allAgents"`
	FailedFiles       []FailedFile      `json:"failedFiles,omitempty"`
	AllowedAgentTypes []string          `json:"allowedAgentTypes,omitempty"`
}

type FailedFile struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// HooksSettings 定义 agent 的钩子设置
type HooksSettings struct {
	PostStart   *HookConfig `yaml:"postStart,omitempty" json:"postStart,omitempty"`
	PreToolUse  *HookConfig `yaml:"preToolUse,omitempty" json:"preToolUse,omitempty"`
	PostToolUse *HookConfig `yaml:"postToolUse,omitempty" json:"postToolUse,omitempty"`
	PostTurn    *HookConfig `yaml:"postTurn,omitempty" json:"postTurn,omitempty"`
}

type HookConfig struct {
	Command string   `yaml:"command" json:"command"`
	Args    []string `yaml:"args,omitempty" json:"args,omitempty"`
}

// ========== 文件解析相关类型 ==========

// MarkdownFile 解析后的 markdown 文件
type MarkdownFile struct {
	FilePath    string
	BaseDir     string
	Frontmatter map[string]interface{}
	Content     string
	Source      SettingSource
}

// FrontmatterResult 解析后的 frontmatter 结果
type FrontmatterResult struct {
	Data           map[string]interface{}
	Content        string
	HasFrontmatter bool
}

// ========== 工具调用相关类型 ==========

// ToolUseContext 工具调用上下文（简化版本）
type ToolUseContext struct {
	Cwd                     string
	AbortController         context.Context
	Tools                   []interface{} // 实际使用 []Tool
	IsNonInteractiveSession bool
}
