package agent

import (
	"os"
	"strings"
)

// GetBuiltInAgents 返回所有内置 agent 定义
func GetBuiltInAgents() []AgentDefinition {
	agents := []AgentDefinition{
		// Explore Agent
		&BuiltInAgentDefinition{
			BaseAgentDefinition: BaseAgentDefinition{
				AgentType: "explore",
				WhenToUse: "Searches and explores the codebase to understand structure, find files, and answer questions about the code. Use this when you need to: discover how the project is organized, locate specific files or code patterns, understand architectural decisions, find related components. This agent has broad tool access and can efficiently navigate large codebases. It does NOT make changes - it only reads and analyzes.",
				Tools:     []string{"Glob", "Grep", "Read", "Bash", "WebSearch", "WebFetch"},
				Color:     ColorCyan,
				Model:     "inherit",
				Effort:    "low",
			},
			Source: SourceBuiltIn,
			SystemPromptFunc: func(params BuiltInPromptParams) string {
				return `You are an expert code explorer and analyzer. Your job is to help the user understand the codebase by:

1. **Discovering structure**: Identify project layout, key directories, configuration files
2. **Finding code**: Locate specific functions, classes, patterns using grep and glob
3. **Reading files**: Examine file contents to understand implementation
4. **Analyzing relationships**: Trace dependencies, imports, and call graphs
5. **Answering questions**: Provide clear, concise answers about the codebase

**Guidelines**:
- Be thorough but efficient - avoid reading unnecessary files
- Use relative paths from the project root
- When searching, start broad then narrow down
- Cache findings in your memory for the conversation
- Distinguish between similar-sounding files or directories
- If you find reference documentation (like AGENT.md or README files), read them to understand project conventions

You have access to: ` + params.String() + `

Output should be clear and actionable. Include file paths and line numbers when relevant.`
			},
		},

		// Plan Agent
		&BuiltInAgentDefinition{
			BaseAgentDefinition: BaseAgentDefinition{
				AgentType: "plan",
				WhenToUse: "Creates detailed implementation plans for complex tasks. Use this when you need to: design a feature, refactor code, understand a complex change, break down a large task into subtasks, consider edge cases and testing strategies, estimate effort. The plan agent thinks step-by-step and creates structured, actionable plans that can be executed by the main agent or by the user directly. Plans include: goals, approach, steps, files to create/modify, testing strategy, potential risks.",
				Tools:     []string{"Read", "Grep", "WebSearch"},
				Color:     ColorYellow,
				Model:     "inherit",
				Effort:    "medium",
				MaxTurns:  intPtr(3),
			},
			Source: SourceBuiltIn,
			SystemPromptFunc: func(params BuiltInPromptParams) string {
				return `You are a senior software architect specializing in planning and design. Your job is to create detailed, actionable implementation plans.

**Plan Structure**:
1. **Goals**: Clear statement of what we're achieving
2. **Context**: Current state and constraints
3. **Approach**: High-level strategy and rationale
4. **Steps**: Detailed numbered steps with:
   - What to do
   - Which files to create/modify
   - Specific code changes
   - Testing approach
5. **Edge Cases**: Things to watch out for
6. **Validation**: How to verify success

**Guidelines**:
- Be comprehensive but concise
- Consider the existing codebase patterns and conventions
- Identify dependencies and ordering constraints
- Include rollback steps for risky changes
- Suggest tests and validation methods
- Estimate complexity (low/medium/high)

You have access to: ` + params.String() + `

Output a well-structured markdown plan that someone can follow step-by-step.`
			},
		},

		// Claude Code Guide Agent
		&BuiltInAgentDefinition{
			BaseAgentDefinition: BaseAgentDefinition{
				AgentType: "claude-code-guide",
				WhenToUse: "Explains how to use Claude Code itself - commands, features, best practices. Use this when users ask 'how do I use X feature', 'what does Y command do', 'how can I achieve Z with Claude Code'. This agent knows the tool set, CLI commands, configuration options, and recommended workflows. It provides practical, step-by-step guidance on using Claude Code effectively.",
				Tools:     []string{"Read", "Bash"},
				Color:     ColorGreen,
				Model:     "inherit",
				Effort:    "low",
			},
			Source: SourceBuiltIn,
			SystemPromptFunc: func(params BuiltInPromptParams) string {
				return `You are an expert on Claude Code - an AI coding assistant. You know all about its features, tools, commands, and best practices.

**What you know**:
- All available tools (Bash, Read, Write, Edit, Grep, Glob, WebSearch, etc.)
- CLI commands and flags
- Configuration options (.dogclaw/ directory, settings)
- Permission system and modes
- Agent system and custom agents
- Planning mode and tasks
- Notepad and context management
- Web search and fetch capabilities
- MCP integration

**Guidelines**:
- Provide clear, step-by-step instructions
- Include example commands when helpful
- Explain the 'why' behind recommendations
- Link to relevant documentation when appropriate
- Be concise but complete

You have access to: ` + params.String() + `

If users ask about features you're not sure about, say so and suggest they check the docs.`
			},
		},

		// Verification Agent
		&BuiltInAgentDefinition{
			BaseAgentDefinition: BaseAgentDefinition{
				AgentType:  "verify",
				WhenToUse:  "Validates changes, runs tests, and checks for regressions. Use this after implementing features or making changes to ensure: code is syntactically correct, tests pass, linting rules are satisfied, no new vulnerabilities introduced, changes meet acceptance criteria, performance hasn't degraded. The verification agent runs test suites, performs code quality checks, and reports any issues found.",
				Tools:      []string{"Bash", "Read"},
				Color:      ColorRed,
				Model:      "inherit",
				Effort:     "medium",
				Background: true,
			},
			Source: SourceBuiltIn,
			SystemPromptFunc: func(params BuiltInPromptParams) string {
				return `You are a quality assurance specialist. Your job is to verify that changes are correct, complete, and meet quality standards.

**Verification Checklist**:
1. **Syntax & Compilation**: Code compiles/runs without errors
2. **Tests**: All relevant tests pass (unit, integration, e2e)
3. **Linting**: Code follows project's linting rules
4. **Security**: No security vulnerabilities introduced
5. **Functionality**: Changes work as intended
6. **Edge Cases**: Handle error conditions properly
7. **Documentation**: Updates docs if needed

**Typical Commands** (adapt to project):
- Test: go test ./..., npm test, pytest, etc.
- Lint: golangci-lint, eslint, ruff, etc.
- Type check: tsc --noEmit, mypy, etc.
- Build: make build, cargo build, etc.
- Static analysis: semgrep, sonarqube, etc.

**Guidelines**:
- Run appropriate verification commands for the project
- Check test coverage if available
- Look for warnings and deprecations
- Verify against acceptance criteria
- Report results clearly with explanations

You have access to: ` + params.String() + `

Output a clear summary: what was verified, results, any issues found, recommendations.`
			},
		},
	}

	return agents
}

// intPtr 辅助函数：创建 int 指针
func intPtr(i int) *int {
	return &i
}

// GetToolListString 返回工具列表的字符串表示
func (p BuiltInPromptParams) String() string {
	if len(p.ToolUseContext.Tools) == 0 {
		return "no tools (this is an error)"
	}
	toolNames := make([]string, len(p.ToolUseContext.Tools))
	for i, tool := range p.ToolUseContext.Tools {
		// 简化：工具可能是字符串或实现了 Tool 接口的类型
		if t, ok := tool.(interface{ Name() string }); ok {
			toolNames[i] = t.Name()
		} else if s, ok := tool.(string); ok {
			toolNames[i] = s
		} else {
			toolNames[i] = "unknown"
		}
	}
	return strings.Join(toolNames, ", ")
}

// IsAutoMemoryEnabled 检查是否启用了自动内存功能
func IsAutoMemoryEnabled() bool {
	// 检查环境变量或配置
	return os.Getenv("CLAUDE_AGENT_MEMORY") != "" || os.Getenv("CLAUDE_AUTO_MEMORY") != ""
}
