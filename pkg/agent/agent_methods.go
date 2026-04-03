package agent

// CustomAgentDefinition 的方法
func (c *CustomAgentDefinition) GetAgentType() string {
	return c.AgentType
}

func (c *CustomAgentDefinition) GetWhenToUse() string {
	return c.WhenToUse
}

func (c *CustomAgentDefinition) GetSystemPrompt() string {
	return c.SystemPrompt
}

func (c *CustomAgentDefinition) GetSource() string {
	return string(c.Source)
}

func (c *CustomAgentDefinition) GetBaseDir() string {
	return c.BaseDir
}

func (c *CustomAgentDefinition) GetFilename() string {
	return c.Filename
}

func (c *CustomAgentDefinition) GetTools() []string {
	return c.Tools
}

func (c *CustomAgentDefinition) GetDisallowedTools() []string {
	return c.DisallowedTools
}

func (c *CustomAgentDefinition) GetSkills() []string {
	return c.Skills
}

func (c *CustomAgentDefinition) GetMcpServers() []any {
	return c.McpServers
}

func (c *CustomAgentDefinition) GetHooks() *HooksSettings {
	return c.Hooks
}

func (c *CustomAgentDefinition) GetColor() AgentColorName {
	return c.Color
}

func (c *CustomAgentDefinition) GetModel() string {
	return c.Model
}

func (c *CustomAgentDefinition) GetEffort() interface{} {
	return c.Effort
}

func (c *CustomAgentDefinition) GetPermissionMode() PermissionMode {
	return c.PermissionMode
}

func (c *CustomAgentDefinition) GetMaxTurns() *int {
	return c.MaxTurns
}

func (c *CustomAgentDefinition) GetBackground() bool {
	return c.Background
}

func (c *CustomAgentDefinition) GetMemory() AgentMemoryScope {
	return c.Memory
}

func (c *CustomAgentDefinition) GetIsolation() string {
	return c.Isolation
}

func (c *CustomAgentDefinition) GetInitialPrompt() string {
	return c.InitialPrompt
}

func (c *CustomAgentDefinition) GetOmitClaudeMd() bool {
	return c.OmitClaudeMd
}

func (c *CustomAgentDefinition) GetBaseAgentDefinition() *BaseAgentDefinition {
	return &c.BaseAgentDefinition
}

// BuiltInAgentDefinition 的方法
func (b *BuiltInAgentDefinition) GetAgentType() string {
	return b.AgentType
}

func (b *BuiltInAgentDefinition) GetWhenToUse() string {
	return b.WhenToUse
}

func (b *BuiltInAgentDefinition) GetSystemPrompt() string {
	if b.SystemPromptFunc != nil {
		return b.SystemPromptFunc(BuiltInPromptParams{})
	}
	return b.BaseAgentDefinition.SystemPrompt
}

func (b *BuiltInAgentDefinition) GetSource() string {
	return string(b.Source)
}

func (b *BuiltInAgentDefinition) GetBaseDir() string {
	return b.BaseDir
}

func (b *BuiltInAgentDefinition) GetFilename() string {
	return b.Filename
}

func (b *BuiltInAgentDefinition) GetTools() []string {
	return b.Tools
}

func (b *BuiltInAgentDefinition) GetDisallowedTools() []string {
	return b.DisallowedTools
}

func (b *BuiltInAgentDefinition) GetSkills() []string {
	return b.Skills
}

func (b *BuiltInAgentDefinition) GetMcpServers() []any {
	return b.McpServers
}

func (b *BuiltInAgentDefinition) GetHooks() *HooksSettings {
	return b.Hooks
}

func (b *BuiltInAgentDefinition) GetColor() AgentColorName {
	return b.Color
}

func (b *BuiltInAgentDefinition) GetModel() string {
	return b.Model
}

func (b *BuiltInAgentDefinition) GetEffort() interface{} {
	return b.Effort
}

func (b *BuiltInAgentDefinition) GetPermissionMode() PermissionMode {
	return b.PermissionMode
}

func (b *BuiltInAgentDefinition) GetMaxTurns() *int {
	return b.MaxTurns
}

func (b *BuiltInAgentDefinition) GetBackground() bool {
	return b.Background
}

func (b *BuiltInAgentDefinition) GetMemory() AgentMemoryScope {
	return b.Memory
}

func (b *BuiltInAgentDefinition) GetIsolation() string {
	return b.Isolation
}

func (b *BuiltInAgentDefinition) GetInitialPrompt() string {
	return b.InitialPrompt
}

func (b *BuiltInAgentDefinition) GetOmitClaudeMd() bool {
	return b.OmitClaudeMd
}

func (b *BuiltInAgentDefinition) GetBaseAgentDefinition() *BaseAgentDefinition {
	return &b.BaseAgentDefinition
}

// PluginAgentDefinition 的方法
func (p *PluginAgentDefinition) GetAgentType() string {
	return p.AgentType
}

func (p *PluginAgentDefinition) GetWhenToUse() string {
	return p.WhenToUse
}

func (p *PluginAgentDefinition) GetSystemPrompt() string {
	return p.SystemPrompt
}

func (p *PluginAgentDefinition) GetSource() string {
	return string(p.Source)
}

func (p *PluginAgentDefinition) GetBaseDir() string {
	return p.BaseDir
}

func (p *PluginAgentDefinition) GetFilename() string {
	return p.Filename
}

func (p *PluginAgentDefinition) GetTools() []string {
	return p.Tools
}

func (p *PluginAgentDefinition) GetDisallowedTools() []string {
	return p.DisallowedTools
}

func (p *PluginAgentDefinition) GetSkills() []string {
	return p.Skills
}

func (p *PluginAgentDefinition) GetMcpServers() []any {
	return p.McpServers
}

func (p *PluginAgentDefinition) GetHooks() *HooksSettings {
	return p.Hooks
}

func (p *PluginAgentDefinition) GetColor() AgentColorName {
	return p.Color
}

func (p *PluginAgentDefinition) GetModel() string {
	return p.Model
}

func (p *PluginAgentDefinition) GetEffort() interface{} {
	return p.Effort
}

func (p *PluginAgentDefinition) GetPermissionMode() PermissionMode {
	return p.PermissionMode
}

func (p *PluginAgentDefinition) GetMaxTurns() *int {
	return p.MaxTurns
}

func (p *PluginAgentDefinition) GetBackground() bool {
	return p.Background
}

func (p *PluginAgentDefinition) GetMemory() AgentMemoryScope {
	return p.Memory
}

func (p *PluginAgentDefinition) GetIsolation() string {
	return p.Isolation
}

func (p *PluginAgentDefinition) GetInitialPrompt() string {
	return p.InitialPrompt
}

func (p *PluginAgentDefinition) GetOmitClaudeMd() bool {
	return p.OmitClaudeMd
}

func (p *PluginAgentDefinition) GetBaseAgentDefinition() *BaseAgentDefinition {
	return &p.BaseAgentDefinition
}
