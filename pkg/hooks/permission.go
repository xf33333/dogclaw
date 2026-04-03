// Package hooks provides the tool permission system, command queue, and main loop model.
// Translated from src/hooks/
package hooks

import (
	"context"
	"sync"
)

// PermissionDecision represents the result of a permission check
type PermissionDecision struct {
	Behavior       string          // "allow", "deny", or "ask"
	UpdatedInput   map[string]any  `json:"updatedInput,omitempty"`
	Message        string          `json:"message,omitempty"`
	UserModified   bool            `json:"userModified,omitempty"`
	ContentBlocks  []ContentBlock  `json:"contentBlocks,omitempty"`
	DecisionReason *DecisionReason `json:"decisionReason,omitempty"`
	AcceptFeedback string          `json:"acceptFeedback,omitempty"`
	Interrupt      bool            `json:"interrupt,omitempty"`
}

// DecisionReason explains why a permission decision was made
type DecisionReason struct {
	Type      string `json:"type"` // "user", "hook", "classifier"
	HookName  string `json:"hookName,omitempty"`
	Permanent bool   `json:"permanent,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// ContentBlock represents a content block in a permission response
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// PermissionUpdate represents a permission rule update
type PermissionUpdate struct {
	ToolName    string `json:"toolName"`
	Permission  string `json:"permission"` // "always_allow", "always_deny", "ask"
	Scope       string `json:"scope"`      // "session", "user", "project"
	Pattern     string `json:"pattern,omitempty"`
	Destination string `json:"destination"`
}

// ToolUseConfirm represents a pending tool use confirmation in the queue
type ToolUseConfirm struct {
	ToolUseID   string              `json:"toolUseID"`
	ToolName    string              `json:"toolName"`
	Input       map[string]any      `json:"input"`
	Description string              `json:"description"`
	Status      string              `json:"status"` // "pending", "approved", "rejected"
	Decision    *PermissionDecision `json:"decision,omitempty"`
}

// PermissionContext manages tool permission checks
type PermissionContext struct {
	mu              sync.Mutex
	toolName        string
	toolUseID       string
	input           map[string]any
	abortController context.Context
	decisionCh      chan PermissionDecision
	resolved        bool
}

// NewPermissionContext creates a new permission context for a tool use
func NewPermissionContext(toolName, toolUseID string, input map[string]any, abortCtx context.Context) *PermissionContext {
	return &PermissionContext{
		toolName:        toolName,
		toolUseID:       toolUseID,
		input:           input,
		abortController: abortCtx,
		decisionCh:      make(chan PermissionDecision, 1),
	}
}

// BuildAllow creates an allow permission decision
func (pc *PermissionContext) BuildAllow(updatedInput map[string]any, opts AllowOpts) PermissionDecision {
	decision := PermissionDecision{
		Behavior:     "allow",
		UpdatedInput: updatedInput,
		UserModified: opts.UserModified,
	}
	if opts.DecisionReason != nil {
		decision.DecisionReason = opts.DecisionReason
	}
	if opts.AcceptFeedback != "" {
		decision.AcceptFeedback = opts.AcceptFeedback
	}
	if len(opts.ContentBlocks) > 0 {
		decision.ContentBlocks = opts.ContentBlocks
	}
	return decision
}

// BuildDeny creates a deny permission decision
func (pc *PermissionContext) BuildDeny(message string, reason DecisionReason) PermissionDecision {
	return PermissionDecision{
		Behavior:       "deny",
		Message:        message,
		DecisionReason: &reason,
	}
}

// AllowOpts contains options for BuildAllow
type AllowOpts struct {
	UserModified   bool
	DecisionReason *DecisionReason
	AcceptFeedback string
	ContentBlocks  []ContentBlock
}

// CancelAndAbort cancels the permission request and aborts the context
func (pc *PermissionContext) CancelAndAbort(feedback string) PermissionDecision {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.resolved {
		return PermissionDecision{}
	}
	pc.resolved = true

	message := REJECT_MESSAGE
	if feedback != "" {
		message = REJECT_MESSAGE_WITH_REASON_PREFIX + feedback
	}

	return pc.BuildDeny(message, DecisionReason{Type: "user_abort"})
}

const (
	REJECT_MESSAGE                             = "The user denied this operation."
	REJECT_MESSAGE_WITH_REASON_PREFIX          = "The user denied this operation and provided feedback: "
	SUBAGENT_REJECT_MESSAGE                    = "The user denied this operation."
	SUBAGENT_REJECT_MESSAGE_WITH_REASON_PREFIX = "The user denied this operation and provided feedback: "
)
