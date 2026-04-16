package experience

import (
	"context"
	"fmt"
	"strings"

	"dogclaw/pkg/types"
)

type ExperienceTool struct {
	manager *Manager
}

func NewExperienceTool(manager *Manager) *ExperienceTool {
	return &ExperienceTool{manager: manager}
}

func (t *ExperienceTool) Name() string {
	return "Experience"
}

func (t *ExperienceTool) Aliases() []string {
	return []string{"experience", "exp", "经验"}
}

func (t *ExperienceTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "操作类型: list(列出所有经验), read(读取指定日期的经验), summary(读取经验汇总)",
				"enum":        []string{"list", "read", "summary"},
			},
			"date": map[string]any{
				"type":        "string",
				"description": "日期，格式为 yyyy-mm-dd，用于read操作",
			},
		},
		Required: []string{"action"},
	}
}

func (t *ExperienceTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return `经验工具，用于查看用户的每日经验总结，方便你了解过去的用户工作总结以及用户信息总结。`
}

func (t *ExperienceTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	action, _ := input["action"].(string)
	if action == "" {
		action = "list"
	}

	switch action {
	case "list":
		return t.handleList()
	case "read":
		date, _ := input["date"].(string)
		return t.handleRead(date)
	case "summary":
		return t.handleSummary()
	default:
		return &types.ToolResult{
			Data:    fmt.Sprintf("Unknown action: %s. Supported actions: list, read, summary", action),
			IsError: true,
		}, nil
	}
}

func (t *ExperienceTool) handleList() (*types.ToolResult, error) {
	files, err := t.manager.GetExperienceList()
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Failed to get experience list: %v", err),
			IsError: true,
		}, nil
	}

	if len(files) == 0 {
		return &types.ToolResult{
			Data: "暂无经验记录。经验会在每天自动总结生成。",
		}, nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("共找到 %d 条经验记录：\n\n", len(files)))

	for i, f := range files {
		sizeKB := float64(f.Size) / 1024
		result.WriteString(fmt.Sprintf("%d. **%s** - %.1f KB (修改时间: %s)\n",
			i+1, f.Date, sizeKB, f.ModTime.Format("2006-01-02 15:04")))
	}

	result.WriteString("\n使用 `Experience` 工具，action=\"read\", date=\"yyyy-mm-dd\" 来查看具体某天的经验。")

	return &types.ToolResult{
		Data: result.String(),
	}, nil
}

func (t *ExperienceTool) handleRead(date string) (*types.ToolResult, error) {
	if date == "" {
		return &types.ToolResult{
			Data:    "请提供date参数，格式为yyyy-mm-dd",
			IsError: true,
		}, nil
	}

	content, err := t.manager.GetExperience(date)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Failed to read experience for %s: %v", date, err),
			IsError: true,
		}, nil
	}

	return &types.ToolResult{
		Data: content,
	}, nil
}

func (t *ExperienceTool) handleSummary() (*types.ToolResult, error) {
	content, err := t.manager.GetSummary()
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Failed to read experience summary: %v", err),
			IsError: true,
		}, nil
	}

	return &types.ToolResult{
		Data: content,
	}, nil
}

func (t *ExperienceTool) IsConcurrencySafe(input map[string]any) bool {
	return true
}

func (t *ExperienceTool) IsReadOnly(input map[string]any) bool {
	return true
}

func (t *ExperienceTool) IsDestructive(input map[string]any) bool {
	return false
}

func (t *ExperienceTool) IsEnabled() bool {
	return true
}

func (t *ExperienceTool) SearchHint() string {
	return "experience summary user profile 经验总结"
}
