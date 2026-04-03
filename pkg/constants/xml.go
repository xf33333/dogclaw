package constants

// XML tag names used to mark skill/command metadata in messages
const (
	CommandNameTag    = "command-name"
	CommandMessageTag = "command-message"
	CommandArgsTag    = "command-args"
)

// XML tag names for terminal/bash command input and output in user messages
const (
	BashInputTag          = "bash-input"
	BashStdoutTag         = "bash-stdout"
	BashStderrTag         = "bash-stderr"
	LocalCommandStdoutTag = "local-command-stdout"
	LocalCommandStderrTag = "local-command-stderr"
	LocalCommandCaveatTag = "local-command-caveat"
)

// All terminal-related tags that indicate a message is terminal output, not a user prompt
var TerminalOutputTags = []string{
	BashInputTag,
	BashStdoutTag,
	BashStderrTag,
	LocalCommandStdoutTag,
	LocalCommandStderrTag,
	LocalCommandCaveatTag,
}

const TickTag = "tick"

// XML tag names for task notifications (background task completions)
const (
	TaskNotificationTag = "task-notification"
	TaskIDTag           = "task-id"
	ToolUseIDTag        = "tool-use-id"
	TaskTypeTag         = "task-type"
	OutputFileTag       = "output-file"
	StatusTag           = "status"
	SummaryTag          = "summary"
	ReasonTag           = "reason"
	WorktreeTag         = "worktree"
	WorktreePathTag     = "worktreePath"
	WorktreeBranchTag   = "worktreeBranch"
)

// XML tag names for ultraplan mode (remote parallel planning sessions)
const UltraplanTag = "ultraplan"

// XML tag name for remote /review results
const RemoteReviewTag = "remote-review"

// Remote review progress tag
const RemoteReviewProgressTag = "remote-review-progress"

// XML tag name for teammate messages (swarm inter-agent communication)
const TeammateMessageTag = "teammate-message"

// XML tag name for external channel messages
const (
	ChannelMessageTag = "channel-message"
	ChannelTag        = "channel"
)

// XML tag name for cross-session UDS messages
const CrossSessionMessageTag = "cross-session-message"

// XML tag wrapping the rules/format boilerplate in a fork child's first message
const (
	ForkBoilerplateTag  = "fork-boilerplate"
	ForkDirectivePrefix = "Your directive: "
)

// Common argument patterns for slash commands that request help
var CommonHelpArgs = []string{"help", "-h", "--help"}

// Common argument patterns for slash commands that request current state/info
var CommonInfoArgs = []string{
	"list", "show", "display", "current", "view", "get",
	"check", "describe", "print", "version", "about", "status", "?",
}
