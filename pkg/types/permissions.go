package types

// PermissionMode represents the permission mode for tool execution
type PermissionMode string

const (
	PermissionModeDefault     PermissionMode = "default"
	PermissionModeBypass      PermissionMode = "bypassPermissions"
	PermissionModePlan        PermissionMode = "plan"
	PermissionModeAcceptEdits PermissionMode = "acceptEdits"
	PermissionModeDontAsk     PermissionMode = "dontAsk"
	PermissionModeAuto        PermissionMode = "auto"
	PermissionModeBubble      PermissionMode = "bubble"
)

// PermissionBehavior represents the result of a permission check
type PermissionBehavior string

const (
	PermissionAllow PermissionBehavior = "allow"
	PermissionDeny  PermissionBehavior = "deny"
	PermissionAsk   PermissionBehavior = "ask"
)

// PermissionResult is the result of a permission check
type PermissionResult struct {
	Behavior       PermissionBehavior `json:"behavior"`
	UpdatedInput   map[string]any     `json:"updatedInput,omitempty"`
	Message        string             `json:"message,omitempty"`
	UserModified   bool               `json:"userModified,omitempty"`
	DecisionReason *DecisionReason    `json:"decisionReason,omitempty"`
}

// DecisionReason explains why a permission decision was made
type DecisionReason struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}
