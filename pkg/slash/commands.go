package slash

import (
	"context"
	"fmt"
	"strings"

	"dogclaw/pkg/version"
)

// CommandType represents the type of slash command
type CommandType string

const (
	// PromptCommand expands to a prompt sent to the LLM
	PromptCommand CommandType = "prompt"
	// LocalCommand executes locally and returns text
	LocalCommand CommandType = "local"
	// LocalJSXCommand renders interactive UI (blocked in headless mode)
	LocalJSXCommand CommandType = "local-jsx"
)

// Command represents a slash command definition
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Type        CommandType
	Source      string // "builtin", "skill", "plugin", "bundled"
	Handler     CommandHandler
}

// CommandHandler processes a command and returns a result
type CommandHandler func(ctx context.Context, args string) (*CommandResult, error)

// CommandResult represents the outcome of executing a command
type CommandResult struct {
	// For prompt commands: the expanded prompt text
	Prompt string
	// For local commands: the text output
	Output string
	// Whether this should halt further processing
	Halt bool
	// Whether an error occurred
	IsError bool
	// Error message if applicable
	ErrorMsg string
}

// CommandRegistry manages all available slash commands
type CommandRegistry struct {
	commands map[string]*Command
}

// NewCommandRegistry creates a new command registry
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		commands: make(map[string]*Command),
	}
}

// Register adds a command to the registry
func (r *CommandRegistry) Register(cmd *Command) {
	r.commands[cmd.Name] = cmd
	for _, alias := range cmd.Aliases {
		r.commands[alias] = cmd
	}
}

// Find looks up a command by name or alias
func (r *CommandRegistry) Find(name string) *Command {
	return r.commands[name]
}

// IsSlashCommand checks if input starts with /
func IsSlashCommand(input string) bool {
	return strings.HasPrefix(strings.TrimSpace(input), "/")
}

// ParseCommand extracts command name and arguments from input
func ParseCommand(input string) (name string, args string) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return "", ""
	}

	trimmed = strings.TrimPrefix(trimmed, "/")
	parts := strings.SplitN(trimmed, " ", 2)

	name = parts[0]
	if len(parts) > 1 {
		args = parts[1]
	}

	return name, args
}

// Execute processes a slash command
func (r *CommandRegistry) Execute(ctx context.Context, input string) (*CommandResult, error) {
	name, args := ParseCommand(input)
	if name == "" {
		return nil, nil // Not a slash command
	}

	cmd := r.Find(name)
	if cmd == nil {
		return &CommandResult{
			IsError:  true,
			ErrorMsg: fmt.Sprintf("Unknown command: /%s. Use /help for available commands.", name),
		}, nil
	}

	if cmd.Handler == nil {
		return &CommandResult{
			IsError:  true,
			ErrorMsg: fmt.Sprintf("Command /%s has no handler implemented", name),
		}, nil
	}

	return cmd.Handler(ctx, args)
}

// ListCommands returns all registered commands
func (r *CommandRegistry) ListCommands() []*Command {
	seen := make(map[string]bool)
	var result []*Command

	for _, cmd := range r.commands {
		if !seen[cmd.Name] {
			seen[cmd.Name] = true
			result = append(result, cmd)
		}
	}

	return result
}

// RegisterBuiltinCommands adds all built-in slash commands
func RegisterBuiltinCommands(registry *CommandRegistry) {
	registry.Register(&Command{
		Name:        "help",
		Aliases:     []string{"h"},
		Description: "Show available commands",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleHelp,
	})

	registry.Register(&Command{
		Name:        "clear",
		Aliases:     []string{"cls"},
		Description: "Clear conversation history",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleClear,
	})

	registry.Register(&Command{
		Name:        "usage",
		Aliases:     []string{"cost"},
		Description: "Show token usage",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleUsage,
	})

	registry.Register(&Command{
		Name:        "model",
		Aliases:     []string{"m"},
		Description: "Switch the current model",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleModel,
	})

	registry.Register(&Command{
		Name:        "compact",
		Description: "Manually trigger context compaction",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleCompact,
	})

	registry.Register(&Command{
		Name:        "verbose",
		Aliases:     []string{"v"},
		Description: "Toggle verbose mode",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleVerbose,
	})

	registry.Register(&Command{
		Name:        "skills",
		Description: "List available skills",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleSkills,
	})

	registry.Register(&Command{
		Name:        "max-turns",
		Description: "Set maximum number of turns",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleMaxTurns,
	})

	registry.Register(&Command{
		Name:        "sessions",
		Aliases:     []string{"ls", "session"},
		Description: "List or search sessions",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleSessions,
	})

	registry.Register(&Command{
		Name:        "resume",
		Aliases:     []string{"r"},
		Description: "Resume a previous session",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleResume,
	})

	registry.Register(&Command{
		Name:        "new",
		Aliases:     []string{"n"},
		Description: "Start a new session",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler: func(ctx context.Context, args string) (*CommandResult, error) {
			return &CommandResult{Output: ""}, nil // handled in engine
		},
	})

	registry.Register(&Command{
		Name:        "restart",
		Aliases:     []string{"reboot"},
		Description: "Restart the program",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleRestart,
	})

	registry.Register(&Command{
		Name:        "status",
		Aliases:     []string{"stat"},
		Description: "Show current session status",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleStatus,
	})

	registry.Register(&Command{
		Name:        "setting",
		Aliases:     []string{"settings", "config"},
		Description: "Show current active configuration",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleSetting,
	})

	registry.Register(&Command{
		Name:        "reset",
		Description: "Clear conversation history (alias of clear)",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleClear, // reuses clear logic but we'll add it to engine switch too
	})

	registry.Register(&Command{
		Name:        "version",
		Aliases:     []string{"v"},
		Description: "Show version information",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleVersion,
	})

	registry.Register(&Command{
		Name:        "shell",
		Aliases:     []string{"sh"},
		Description: "Execute a shell command",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleShell,
	})

	registry.Register(&Command{
		Name:        "mcp",
		Description: "List all MCP tools and their details",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler:     HandleMCP,
	})
}

// HandleHelp shows available commands
func HandleHelp(ctx context.Context, args string) (*CommandResult, error) {
	return &CommandResult{
		Output: `🦞 DogClaw 命令帮助

📋 基本命令:
  /help, /h              - 显示此帮助信息
  /version, /v           - 显示版本信息
  /status, /stat         - 显示当前会话状态
  /setting, /settings, /config - 显示当前生效的配置
  /exit, /quit, /q       - 退出程序

💬 会话管理:
  /new, /n               - 开始新会话
  /reset, /clear, /cls   - 清除当前会话历史
  /sessions, /ls         - 列出所有会话
  /resume [id], /r [id]  - 恢复指定会话

🔧 工具命令:
  /usage, /cost          - 显示 token 使用统计
  /model [alias], /m [a] - 列出或切换模型别名
  /compact               - 手动触发上下文压缩
  /verbose, /v           - 切换详细模式
  /skills                - 列出可用技能
  /max-turns <n>         - 设置最大对话轮数
  /restart, /reboot      - 重启程序
  /shell <command>, /sh  - 执行 shell 命令

命令行选项 (启动时使用):
  --config <path>, -c <path>  - 使用自定义配置文件
  --version                    - 显示版本信息
  --help, -h                   - 显示命令行帮助`,
	}, nil
}

// HandleClear clears conversation
func HandleClear(ctx context.Context, args string) (*CommandResult, error) {
	return &CommandResult{
		Output: "Conversation history cleared.",
		Halt:   true,
	}, nil
}

// HandleUsage shows usage info
func HandleUsage(ctx context.Context, args string) (*CommandResult, error) {
	return &CommandResult{
		Output: "Usage tracking: use /usage stats for detailed breakdown",
	}, nil
}

// HandleModel switches model - actual logic in engine.handleSlashCommand
func HandleModel(ctx context.Context, args string) (*CommandResult, error) {
	return &CommandResult{
		Output: "",
	}, nil
}

// HandleCompact triggers compaction
func HandleCompact(ctx context.Context, args string) (*CommandResult, error) {
	return &CommandResult{
		Output: "Context compaction triggered.",
	}, nil
}

// HandleVerbose toggles verbose mode
func HandleVerbose(ctx context.Context, args string) (*CommandResult, error) {
	return &CommandResult{
		Output: "Verbose mode toggled.",
	}, nil
}

// HandleSkills lists skills
func HandleSkills(ctx context.Context, args string) (*CommandResult, error) {
	return &CommandResult{
		Output: "No skills installed. Use /skills install <name> to add skills.",
	}, nil
}

// HandleMaxTurns sets max turns
func HandleMaxTurns(ctx context.Context, args string) (*CommandResult, error) {
	if args == "" {
		return &CommandResult{
			Output: "Usage: /max-turns <number>",
		}, nil
	}

	return &CommandResult{
		Output: fmt.Sprintf("Max turns set to: %s", args),
	}, nil
}

// HandleSessions handles /sessions command - actual logic in engine.handleSessionsCommand
func HandleSessions(ctx context.Context, args string) (*CommandResult, error) {
	// Just acknowledge - engine.handleSlashCommand will do the actual work
	return &CommandResult{
		Output: "", // empty output, engine will write logs
	}, nil
}

// HandleResume handles /resume command - actual logic in engine.handleResumeCommand
func HandleResume(ctx context.Context, args string) (*CommandResult, error) {
	// Just acknowledge - engine.handleSlashCommand will do the actual work
	return &CommandResult{
		Output: "", // empty output, engine will write logs
	}, nil
}

// HandleStatus handles /status command - actual logic in engine.handleStatusCommand
func HandleStatus(ctx context.Context, args string) (*CommandResult, error) {
	// Just acknowledge - engine.handleSlashCommand will do the actual work
	return &CommandResult{
		Output: "", // empty output, engine will write logs
	}, nil
}

// HandleSetting handles /setting command - actual logic in engine.handleSettingCommand
func HandleSetting(ctx context.Context, args string) (*CommandResult, error) {
	// Just acknowledge - engine.handleSlashCommand will do the actual work
	return &CommandResult{
		Output: "", // empty output, engine will write logs
	}, nil
}

// HandleRestart handles /restart command - actual logic in engine.handleSlashCommand
func HandleRestart(ctx context.Context, args string) (*CommandResult, error) {
	// Just acknowledge - engine.handleSlashCommand will do the actual work
	return &CommandResult{
		Output: "", // empty output, engine will write logs
	}, nil
}

// HandleVersion handles /version command
func HandleVersion(ctx context.Context, args string) (*CommandResult, error) {
	return &CommandResult{
		Output: version.GetVersionString(),
		Halt:   true,
	}, nil
}

// HandleShell handles /shell command - actual logic in engine.handleShellCommand
func HandleShell(ctx context.Context, args string) (*CommandResult, error) {
	// Just acknowledge - engine.handleSlashCommand will do the actual work
	return &CommandResult{
		Output: "", // empty output, engine will write logs
	}, nil
}

// HandleMCP handles /mcp command - actual logic in engine.handleSlashCommand
func HandleMCP(ctx context.Context, args string) (*CommandResult, error) {
	// Just acknowledge - engine.handleSlashCommand will do the actual work
	return &CommandResult{
		Output: "", // empty output, engine will write logs
	}, nil
}
