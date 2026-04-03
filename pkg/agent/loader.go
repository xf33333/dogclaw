package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// logDebug 日志输出
var logDebug = debugLogger{}

type debugLogger struct{}

func (d debugLogger) Printf(format string, args ...interface{}) {
	if os.Getenv("CLAUDE_DEBUG") != "" || os.Getenv("GOCLAUDE_DEBUG") != "" {
		fmt.Printf("[DEBUG] "+format+"\n", args...)
	}
}

// ConfigLoader 配置加载器
type ConfigLoader struct {
	homeDir    string
	managedDir string
	cwd        string
	simpleMode bool // 是否使用简单模式（只加载内置 agent）
}

// NewConfigLoader 创建新的配置加载器
func NewConfigLoader(cwd string) *ConfigLoader {
	homeDir, _ := os.UserHomeDir()
	// TODO: 实现 managedDir 确定逻辑（参考 TS 版本的 getManagedFilePath）
	managedDir := filepath.Join(homeDir, ".dogclaw", "managed") // 占位符

	return &ConfigLoader{
		homeDir:    homeDir,
		managedDir: managedDir,
		cwd:        cwd,
		simpleMode: isEnvTruthy(os.Getenv("CLAUDE_CODE_SIMPLE")),
	}
}

// LoadAgents 加载所有 agent 定义
func (cl *ConfigLoader) LoadAgents(ctx context.Context) (*AgentDefinitionsResult, error) {
	// 简单模式：只返回内置 agent
	if cl.simpleMode {
		builtIn := GetBuiltInAgents()
		return &AgentDefinitionsResult{
			ActiveAgents: builtIn,
			AllAgents:    builtIn,
		}, nil
	}

	// 1. 加载 markdown agent 文件
	markdownFiles, err := cl.loadMarkdownAgents(ctx)
	if err != nil {
		// 出错时仍然返回内置 agent
		builtIn := GetBuiltInAgents()
		return &AgentDefinitionsResult{
			ActiveAgents: builtIn,
			AllAgents:    builtIn,
			FailedFiles: []FailedFile{
				{Path: "unknown", Error: err.Error()},
			},
		}, nil
	}

	// 2. 解析 markdown 中的 agent
	customAgents := make([]AgentDefinition, 0, len(markdownFiles))
	var failedFiles []FailedFile

	for _, mf := range markdownFiles {
		agent := parseAgentFromMarkdown(mf)
		if agent == nil {
			// 跳过非 agent 文件（没有 name frontmatter）
			if _, hasName := mf.Frontmatter["name"]; hasName {
				failedFiles = append(failedFiles, FailedFile{
					Path:  mf.FilePath,
					Error: getParseError(mf.Frontmatter),
				})
			}
			continue
		}
		// 设置基础信息
		agent.GetBaseAgentDefinition().Filename = filepath.Base(mf.FilePath)
		agent.GetBaseAgentDefinition().BaseDir = mf.BaseDir
		agent.GetBaseAgentDefinition().Source = mf.Source
		customAgents = append(customAgents, agent)
	}

	// 3. 加载插件 agent（占位符）
	pluginAgents := loadPluginAgents()

	// 4. 加载内置 agent
	builtInAgents := GetBuiltInAgents()

	// 5. 合并所有 agent
	allAgents := make([]AgentDefinition, 0, len(builtInAgents)+len(pluginAgents)+len(customAgents))
	allAgents = append(allAgents, builtInAgents...)
	allAgents = append(allAgents, pluginAgents...)
	allAgents = append(allAgents, customAgents...)

	// 6. 获取 active agents（去重，优先级：built-in > plugin > user > project > policy > flag）
	activeAgents := getActiveAgentsFromList(allAgents)

	// TODO: 初始化 agent 颜色
	// TODO: 初始化 agent 内存快照

	return &AgentDefinitionsResult{
		ActiveAgents: activeAgents,
		AllAgents:    allAgents,
		FailedFiles:  failedFiles,
	}, nil
}

// loadMarkdownAgents 从各个位置加载 markdown agent 文件
func (cl *ConfigLoader) loadMarkdownAgents(ctx context.Context) ([]MarkdownFile, error) {
	// 收集搜索目录
	userDir := filepath.Join(cl.homeDir, ".dogclaw", "agents")
	managedDir := filepath.Join(cl.managedDir, ".dogclaw", "agents")
	projectDirs := getProjectDirsUpToHome("agents", cl.cwd)

	// TODO: worktree fallback 逻辑

	// 并行加载
	managedChan := make(chan []MarkdownFile, 1)
	userChan := make(chan []MarkdownFile, 1)
	projectChan := make(chan []MarkdownFile, 1)
	errChan := make(chan error, 3)

	// 加载 managed
	go func() {
		files, err := LoadMarkdownFiles(managedDir)
		if err != nil {
			errChan <- fmt.Errorf("failed to load managed agents: %w", err)
			return
		}
		marked := make([]MarkdownFile, len(files))
		for i, f := range files {
			marked[i] = MarkdownFile{
				FilePath:    f.FilePath,
				BaseDir:     managedDir,
				Frontmatter: f.Frontmatter,
				Content:     f.Content,
				Source:      SourcePolicy,
			}
		}
		managedChan <- marked
	}()

	// 加载 user
	go func() {
		// TODO: 检查是否启用 userSettings
		files, err := LoadMarkdownFiles(userDir)
		if err != nil {
			if !os.IsNotExist(err) {
				errChan <- fmt.Errorf("failed to load user agents: %w", err)
			}
			userChan <- []MarkdownFile{}
			return
		}
		marked := make([]MarkdownFile, len(files))
		for i, f := range files {
			marked[i] = MarkdownFile{
				FilePath:    f.FilePath,
				BaseDir:     userDir,
				Frontmatter: f.Frontmatter,
				Content:     f.Content,
				Source:      SourceUser,
			}
		}
		userChan <- marked
	}()

	// 加载 project
	go func() {
		// TODO: 检查是否启用 projectSettings
		allProject := make([]MarkdownFile, 0)
		for _, dir := range projectDirs {
			files, err := LoadMarkdownFiles(dir)
			if err != nil {
				if !os.IsNotExist(err) {
					errChan <- fmt.Errorf("failed to load project agents from %s: %w", dir, err)
					continue
				}
			}
			for _, f := range files {
				allProject = append(allProject, MarkdownFile{
					FilePath:    f.FilePath,
					BaseDir:     dir,
					Frontmatter: f.Frontmatter,
					Content:     f.Content,
					Source:      SourceProject,
				})
			}
		}
		projectChan <- allProject
	}()

	// 收集结果
	var allFiles []MarkdownFile
	for i := 0; i < 3; i++ {
		select {
		case files := <-managedChan:
			allFiles = append(allFiles, files...)
		case files := <-userChan:
			allFiles = append(allFiles, files...)
		case files := <-projectChan:
			allFiles = append(allFiles, files...)
		case err := <-errChan:
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// 去重（基于设备+inode）
	deduped := DeduplicateFiles(allFiles)
	logDebug.Printf("Deduplicated %d files (removed %d duplicates)", len(deduped), len(allFiles)-len(deduped))

	return deduped, nil
}

// ParseAgentFromJSON 从 JSON 数据解析 agent（来自 settings）
func ParseAgentFromJSON(name string, definition map[string]interface{}, source SettingSource) *CustomAgentDefinition {
	// TODO: 实现完整的 Zod schema 验证
	// 这里使用简单的类型断言

	agent := &CustomAgentDefinition{
		BaseAgentDefinition: BaseAgentDefinition{
			AgentType: name,
		},
		Source: source,
	}

	if desc, ok := definition["description"].(string); ok {
		agent.WhenToUse = desc
	}
	if tools, ok := definition["tools"].([]interface{}); ok {
		agent.Tools = make([]string, len(tools))
		for i, t := range tools {
			if s, ok := t.(string); ok {
				agent.Tools[i] = s
			}
		}
	}
	if disallowed, ok := definition["disallowedTools"].([]interface{}); ok {
		agent.DisallowedTools = make([]string, len(disallowed))
		for i, t := range disallowed {
			if s, ok := t.(string); ok {
				agent.DisallowedTools[i] = s
			}
		}
	}
	if prompt, ok := definition["prompt"].(string); ok {
		agent.SystemPrompt = prompt
	}
	if model, ok := definition["model"].(string); ok {
		agent.Model = strings.ToLower(strings.TrimSpace(model))
	}
	if effort, ok := definition["effort"]; ok {
		agent.Effort = effort // 可以是字符串或整数
	}
	if perm, ok := definition["permissionMode"].(string); ok && isValidPermissionMode(perm) {
		agent.PermissionMode = PermissionMode(perm)
	}
	if servers, ok := definition["mcpServers"].([]interface{}); ok {
		agent.McpServers = servers
	}
	if hooks, ok := definition["hooks"].(map[interface{}]interface{}); ok {
		// TODO: 解析 HooksSettings
		_ = hooks
	}
	if maxTurns, ok := definition["maxTurns"].(int); ok && maxTurns > 0 {
		agent.MaxTurns = &maxTurns
	}
	if skills, ok := definition["skills"].([]interface{}); ok {
		agent.Skills = make([]string, len(skills))
		for i, s := range skills {
			if skill, ok := s.(string); ok {
				agent.Skills[i] = skill
			}
		}
	}
	if initPrompt, ok := definition["initialPrompt"].(string); ok {
		agent.InitialPrompt = initPrompt
	}
	if memory, ok := definition["memory"].(string); ok && isValidMemoryScope(memory) {
		agent.Memory = AgentMemoryScope(memory)
	}
	if background, ok := definition["background"].(bool); ok {
		agent.Background = background
	}
	if isolation, ok := definition["isolation"].(string); ok && isValidIsolation(isolation) {
		agent.Isolation = isolation
	}

	// 验证必需字段
	if agent.AgentType == "" || agent.WhenToUse == "" || agent.SystemPrompt == "" {
		return nil
	}

	return agent
}

// ParseAgentsFromJSON 从 JSON 对象解析多个 agents
func ParseAgentsFromJSON(agentsJSON map[string]interface{}) []AgentDefinition {
	result := make([]AgentDefinition, 0, len(agentsJSON))
	for name, def := range agentsJSON {
		if defMap, ok := def.(map[interface{}]interface{}); ok {
			// 转换 map[interface{}]interface{} 为 map[string]interface{}
			mapped := make(map[string]interface{})
			for k, v := range defMap {
				if keyStr, ok := k.(string); ok {
					mapped[keyStr] = v
				}
			}
			agent := ParseAgentFromJSON(name, mapped, SourceFlag)
			if agent != nil {
				result = append(result, agent)
			}
		}
	}
	return result
}

// getActiveAgentsFromList 从列表中提取 active agents，去重并应用优先级
func getActiveAgentsFromList(allAgents []AgentDefinition) []AgentDefinition {
	// 优先级顺序（低→高）：built-in, plugin, user, project, policy, flag
	priority := map[SettingSource]int{
		SourceBuiltIn: 0,
		SourcePlugin:  1,
		SourceUser:    2,
		SourceProject: 3,
		SourcePolicy:  4,
		SourceFlag:    5,
	}

	agentMap := make(map[string]AgentDefinition)

	// 按优先级排序处理
	agentsWithPriority := make([]struct {
		agent AgentDefinition
		prio  int
	}, len(allAgents))
	for i, a := range allAgents {
		src := SettingSource(a.GetSource())
		p := priority[src]
		if p == 0 {
			// 默认优先级
			p = 3
		}
		agentsWithPriority[i] = struct {
			agent AgentDefinition
			prio  int
		}{a, p}
	}

	// 按优先级排序（稳定排序）
	for i := 0; i < len(agentsWithPriority); i++ {
		for j := i + 1; j < len(agentsWithPriority); j++ {
			if agentsWithPriority[i].prio > agentsWithPriority[j].prio {
				agentsWithPriority[i], agentsWithPriority[j] = agentsWithPriority[j], agentsWithPriority[i]
			}
		}
	}

	// 以最低优先级（最后处理）的为准（更高优先级会覆盖）
	for _, p := range agentsWithPriority {
		a := p.agent
		agentMap[a.GetAgentType()] = a
	}

	// 返回所有唯一的 active agents
	result := make([]AgentDefinition, 0, len(agentMap))
	for _, a := range agentMap {
		result = append(result, a)
	}

	// 按字母顺序排序（可选）
	// sort.Slice(result, func(i, j int) bool {
	//     return result[i].GetAgentType() < result[j].GetAgentType()
	// })

	return result
}

// 辅助函数

func isValidPermissionMode(mode string) bool {
	switch PermissionMode(mode) {
	case PermissionModeDefault, PermissionModeReadOnly, PermissionModeFull:
		return true
	}
	return false
}

func isValidMemoryScope(scope string) bool {
	switch AgentMemoryScope(scope) {
	case MemoryUser, MemoryProject, MemoryLocal:
		return true
	}
	return false
}

func isValidIsolation(mode string) bool {
	return mode == "worktree" || mode == "remote"
}

func parseEffortValue(raw interface{}) interface{} {
	// 如果是字符串，检查是否在有效级别中
	if s, ok := raw.(string); ok {
		switch EffortLevel(s) {
		case EffortLow, EffortMedium, EffortHigh, EffortAuto:
			return s
		}
	}
	// 如果是整数，直接返回
	if i, ok := raw.(int); ok && i > 0 {
		return i
	}
	// 如果是浮点数，转换为整数
	if f, ok := raw.(float64); ok && f > 0 {
		return int(f)
	}
	return nil
}

func getParseError(frontmatter map[string]interface{}) string {
	if agentType, hasName := frontmatter["name"]; !hasName || agentType == "" {
		return "Missing required 'name' field in frontmatter"
	}
	if desc, hasDesc := frontmatter["description"]; !hasDesc || desc == "" {
		return "Missing required 'description' field in frontmatter"
	}
	return "Unknown parsing error"
}

// getProjectDirsUpToHome 从 cwd 向上遍历到 git root 或 home，收集所有 .dogclaw/agents 目录
func getProjectDirsUpToHome(subdir string, cwd string) []string {
	home, _ := os.UserHomeDir()
	home = filepath.Clean(home)

	gitRoot := findGitRoot(cwd)
	current := filepath.Clean(cwd)
	dirs := []string{}

	for {
		// 到达 home 停止
		if filepath.Clean(current) == home {
			break
		}

		dogclawDir := filepath.Join(current, ".dogclaw", subdir)
		// 检查目录是否存在
		if _, err := os.Stat(dogclawDir); err == nil {
			dirs = append(dirs, dogclawDir)
		}

		// 到达 git root 停止
		if gitRoot != "" && filepath.Clean(current) == filepath.Clean(gitRoot) {
			break
		}

		parent := filepath.Dir(current)
		if parent == current { // 已到根目录
			break
		}
		current = parent
	}

	return dirs
}

func findGitRoot(dir string) string {
	// 简单实现：向上查找 .git 目录
	current := filepath.Clean(dir)
	for {
		gitPath := filepath.Join(current, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

// isEnvTruthy 检查环境变量是否为真值
func isEnvTruthy(val string) bool {
	switch strings.ToLower(val) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// loadPluginAgents 加载插件 agent（占位符）
func loadPluginAgents() []AgentDefinition {
	// TODO: 实现插件系统
	return []AgentDefinition{}
}

// clearAgentDefinitionsCache 清除 agent 定义缓存（占位符）
func clearAgentDefinitionsCache() {
	// TODO: 实现缓存清除
}

// parseAgentFromMarkdown 从 markdown 文件数据解析 agent
func parseAgentFromMarkdown(mf MarkdownFile) AgentDefinition {
	// 检查必需字段
	agentType, hasName := mf.Frontmatter["name"]
	if !hasName || agentType == "" {
		return nil
	}
	nameStr, ok := agentType.(string)
	if !ok || nameStr == "" {
		return nil
	}

	whenToUse, hasDesc := mf.Frontmatter["description"]
	if !hasDesc || whenToUse == "" {
		return nil
	}
	descStr, ok := whenToUse.(string)
	if !ok || descStr == "" {
		return nil
	}

	// 创建 custom agent
	agent := &CustomAgentDefinition{
		BaseAgentDefinition: BaseAgentDefinition{
			AgentType: nameStr,
			WhenToUse: descStr,
		},
		Source: mf.Source,
	}

	// 解析颜色
	if colorVal, ok := mf.Frontmatter["color"]; ok {
		if colorStr, ok := colorVal.(string); ok && isValidAgentColor(AgentColorName(colorStr)) {
			agent.Color = AgentColorName(colorStr)
		}
	}

	// 解析模型
	if modelVal, ok := mf.Frontmatter["model"]; ok {
		if modelStr, ok := modelVal.(string); ok && strings.TrimSpace(modelStr) != "" {
			agent.Model = strings.ToLower(strings.TrimSpace(modelStr))
		}
	}

	// 解析工具
	agent.Tools = ParseAgentToolsFromFrontmatter(mf.Frontmatter["tools"])
	agent.DisallowedTools = ParseAgentToolsFromFrontmatter(mf.Frontmatter["disallowedTools"])

	// 解析技能
	if skillsVal, ok := mf.Frontmatter["skills"]; ok {
		agent.Skills = ParseSlashCommandToolsFromFrontmatter(skillsVal)
	}

	// 解析初始提示
	if initPromptVal, ok := mf.Frontmatter["initialPrompt"]; ok {
		if initStr, ok := initPromptVal.(string); ok && strings.TrimSpace(initStr) != "" {
			agent.InitialPrompt = strings.TrimSpace(initStr)
		}
	}

	// 解析背景标志
	if bgVal, ok := mf.Frontmatter["background"]; ok {
		if bgBool, ok := bgVal.(bool); ok {
			agent.Background = bgBool
		} else if bgStr, ok := bgVal.(string); ok {
			agent.Background = strings.ToLower(bgStr) == "true"
		}
	}

	// 解析记忆
	if memVal, ok := mf.Frontmatter["memory"]; ok {
		if memStr, ok := memVal.(string); ok && isValidMemoryScope(memStr) {
			agent.Memory = AgentMemoryScope(memStr)
		}
	}

	// 解析隔离
	if isoVal, ok := mf.Frontmatter["isolation"]; ok {
		if isoStr, ok := isoVal.(string); ok && isValidIsolation(isoStr) {
			agent.Isolation = isoStr
		}
	}

	// 解析努力值
	if effortVal, ok := mf.Frontmatter["effort"]; ok {
		if effort := parseEffortValue(effortVal); effort != nil {
			agent.Effort = effort
		}
	}

	// 解析权限模式
	if permVal, ok := mf.Frontmatter["permissionMode"]; ok {
		if permStr, ok := permVal.(string); ok && isValidPermissionMode(permStr) {
			agent.PermissionMode = PermissionMode(permStr)
		}
	}

	// 解析最大回合数
	if maxTurnsVal, ok := mf.Frontmatter["maxTurns"]; ok {
		if maxTurnsInt, ok := maxTurnsVal.(int); ok && maxTurnsInt > 0 {
			agent.MaxTurns = &maxTurnsInt
		}
	}

	// 解析 MCP 服务器
	if mcpVal, ok := mf.Frontmatter["mcpServers"]; ok {
		if mcpList, ok := mcpVal.([]interface{}); ok {
			agent.McpServers = mcpList
		}
	}

	// TODO: 解析 hooks

	// 设置系统提示（内容主体）
	agent.SystemPrompt = strings.TrimSpace(mf.Content)

	// 如果启用了自动记忆，注入读写工具
	if IsAutoMemoryEnabled() && agent.Memory != "" && agent.Tools != nil {
		toolSet := make(map[string]bool)
		for _, tool := range agent.Tools {
			toolSet[tool] = true
		}
		// 添加文件工具
		requiredTools := []string{"FileWrite", "FileEdit", "FileRead"}
		for _, tool := range requiredTools {
			if !toolSet[tool] {
				agent.Tools = append(agent.Tools, tool)
			}
		}
	}

	// 验证必需字段
	if agent.AgentType == "" || agent.WhenToUse == "" || agent.SystemPrompt == "" {
		return nil
	}

	return agent
}

func isValidAgentColor(color AgentColorName) bool {
	for _, c := range ValidAgentColors {
		if c == color {
			return true
		}
	}
	return false
}
