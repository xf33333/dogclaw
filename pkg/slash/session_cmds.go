package slash

import (
	"context"
)

// RegisterSessionCommands adds /resume and /sessions stub commands.
// The actual logic lives in QueryEngine.handleSlashCommand which has
// full access to the session manager and engine state.
func RegisterSessionCommands(registry *CommandRegistry) {
	registry.Register(&Command{
		Name:        "resume",
		Aliases:     []string{"r"},
		Description: "Resume a previous session",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler: func(ctx context.Context, args string) (*CommandResult, error) {
			return &CommandResult{
				Output: "session-resume:placeholder",
			}, nil
		},
	})

	registry.Register(&Command{
		Name:        "sessions",
		Aliases:     []string{"ls"},
		Description: "List all sessions",
		Type:        LocalCommand,
		Source:      "builtin",
		Handler: func(ctx context.Context, args string) (*CommandResult, error) {
			return &CommandResult{
				Output: "session-list:placeholder",
			}, nil
		},
	})
}
