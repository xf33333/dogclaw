package constants

import "maps"

// Tool names - mirrored from TypeScript source.
const (
	AGENT_TOOL_NAME        = "Agent"
	LEGACY_AGENT_TOOL_NAME = "Task"

	BASH_TOOL_NAME              = "Bash"
	POWERSHELL_TOOL_NAME        = "PowerShell"
	FILE_READ_TOOL_NAME         = "Read"
	FILE_EDIT_TOOL_NAME         = "Edit"
	FILE_WRITE_TOOL_NAME        = "Write"
	NOTEBOOK_EDIT_TOOL_NAME     = "NotebookEdit"
	WEB_SEARCH_TOOL_NAME        = "WebSearch"
	WEB_FETCH_TOOL_NAME         = "WebFetch"
	GREP_TOOL_NAME              = "Grep"
	GLOB_TOOL_NAME              = "Glob"
	TODO_WRITE_TOOL_NAME        = "TodoWrite"
	SKILL_TOOL_NAME             = "Skill"
	TOOL_SEARCH_TOOL_NAME       = "ToolSearch"
	ENTER_WORKTREE_TOOL_NAME    = "EnterWorktree"
	EXIT_WORKTREE_TOOL_NAME     = "ExitWorktree"
	ASK_USER_QUESTION_TOOL_NAME = "AskUserQuestion"

	// Agent/task management
	SEND_MESSAGE_TOOL_NAME     = "SendMessage"
	TASK_STOP_TOOL_NAME        = "TaskStop"
	TASK_CREATE_TOOL_NAME      = "TaskCreate"
	TASK_GET_TOOL_NAME         = "TaskGet"
	TASK_LIST_TOOL_NAME        = "TaskList"
	TASK_UPDATE_TOOL_NAME      = "TaskUpdate"
	TASK_OUTPUT_TOOL_NAME      = "TaskOutput"
	TEAM_CREATE_TOOL_NAME      = "TeamCreate"
	TEAM_DELETE_TOOL_NAME      = "TeamDelete"
	SYNTHETIC_OUTPUT_TOOL_NAME = "SyntheticOutput"

	// Plan mode
	ENTER_PLAN_MODE_TOOL_NAME   = "EnterPlanMode"
	EXIT_PLAN_MODE_TOOL_NAME    = "ExitPlanMode"
	EXIT_PLAN_MODE_V2_TOOL_NAME = "ExitPlanMode"

	// Brief / SendUserMessage
	BRIEF_TOOL_NAME        = "SendUserMessage"
	LEGACY_BRIEF_TOOL_NAME = "Brief"

	// Cron / Schedule
	CRON_CREATE_TOOL_NAME = "CronCreate"
	CRON_DELETE_TOOL_NAME = "CronDelete"
	CRON_LIST_TOOL_NAME   = "CronList"

	// Config
	CONFIG_TOOL_NAME = "Config"

	// REPL
	REPL_TOOL_NAME = "REPL"

	// Workflow
	WORKFLOW_TOOL_NAME = "Workflow"
)

// Shell tool names (Bash + PowerShell)
var SHELL_TOOL_NAMES = []string{BASH_TOOL_NAME, POWERSHELL_TOOL_NAME}

// AsyncAgentAllowedTools — tools available to workers spawned via Agent tool.
var AsyncAgentAllowedTools = map[string]bool{
	FILE_READ_TOOL_NAME:        true,
	WEB_SEARCH_TOOL_NAME:       true,
	TODO_WRITE_TOOL_NAME:       true,
	GREP_TOOL_NAME:             true,
	WEB_FETCH_TOOL_NAME:        true,
	GLOB_TOOL_NAME:             true,
	BASH_TOOL_NAME:             true,
	POWERSHELL_TOOL_NAME:       true,
	FILE_EDIT_TOOL_NAME:        true,
	FILE_WRITE_TOOL_NAME:       true,
	NOTEBOOK_EDIT_TOOL_NAME:    true,
	SKILL_TOOL_NAME:            true,
	SYNTHETIC_OUTPUT_TOOL_NAME: true,
	TOOL_SEARCH_TOOL_NAME:      true,
	ENTER_WORKTREE_TOOL_NAME:   true,
	EXIT_WORKTREE_TOOL_NAME:    true,
}

// InternalWorkerTools — filtered out from coordinator's worker tools context.
var InternalWorkerTools = map[string]bool{
	TEAM_CREATE_TOOL_NAME:      true,
	TEAM_DELETE_TOOL_NAME:      true,
	SEND_MESSAGE_TOOL_NAME:     true,
	SYNTHETIC_OUTPUT_TOOL_NAME: true,
}

// CoordinatorModeAllowedTools — tools allowed in coordinator mode.
var CoordinatorModeAllowedTools = map[string]bool{
	AGENT_TOOL_NAME:            true,
	TASK_STOP_TOOL_NAME:        true,
	SEND_MESSAGE_TOOL_NAME:     true,
	SYNTHETIC_OUTPUT_TOOL_NAME: true,
}

// AllAgentDisallowedTools — tools disallowed for all subagents.
func AllAgentDisallowedTools(userType string) map[string]bool {
	disallowed := map[string]bool{
		TASK_OUTPUT_TOOL_NAME:       true,
		EXIT_PLAN_MODE_V2_TOOL_NAME: true,
		ENTER_PLAN_MODE_TOOL_NAME:   true,
		ASK_USER_QUESTION_TOOL_NAME: true,
		TASK_STOP_TOOL_NAME:         true,
	}
	if userType != "ant" {
		disallowed[AGENT_TOOL_NAME] = true
	}
	return disallowed
}

// InProcessTeammateAllowedTools — tools for in-process teammates only.
var InProcessTeammateAllowedTools = map[string]bool{
	TASK_CREATE_TOOL_NAME:  true,
	TASK_GET_TOOL_NAME:     true,
	TASK_LIST_TOOL_NAME:    true,
	TASK_UPDATE_TOOL_NAME:  true,
	SEND_MESSAGE_TOOL_NAME: true,
	CRON_CREATE_TOOL_NAME:  true,
	CRON_DELETE_TOOL_NAME:  true,
	CRON_LIST_TOOL_NAME:    true,
}

// MergeAllowedTools merges multiple tool maps, returning a new set.
func MergeAllowedTools(toolSets ...map[string]bool) map[string]bool {
	merged := make(map[string]bool)
	for _, set := range toolSets {
		maps.Copy(merged, set)
	}
	return merged
}
