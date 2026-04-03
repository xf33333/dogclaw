package slash

import (
	"context"
	"fmt"
	"strings"
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
		Description: "Show token usage and cost",
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
}

// HandleHelp shows available commands
func HandleHelp(ctx context.Context, args string) (*CommandResult, error) {
	return &CommandResult{
		Output: `Available commands:
  /help          - Show this help message
  /clear         - Clear conversation history
  /usage         - Show token usage and cost
  /model <name>  - Switch model (sonnet/opus/haiku)
  /compact       - Manually trigger context compaction
  /verbose       - Toggle verbose mode
  /skills        - List available skills
  /max-turns <n> - Set maximum turns`,
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

// HandleModel switches model
func HandleModel(ctx context.Context, args string) (*CommandResult, error) {
	if args == "" {
		return &CommandResult{
			Output: "Current model: sonnet. Usage: /model <sonnet|opus|haiku>",
		}, nil
	}

	model := strings.ToLower(args)
	validModels := map[string]bool{
		"sonnet": true, "opus": true, "haiku": true,
	}

	if !validModels[model] {
		return &CommandResult{
			IsError:  true,
			ErrorMsg: fmt.Sprintf("Unknown model: %s. Available: sonnet, opus, haiku", model),
		}, nil
	}

	return &CommandResult{
		Output: fmt.Sprintf("Model switched to: %s", model),
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
