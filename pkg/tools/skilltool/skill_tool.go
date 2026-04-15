package skilltool

import (
	"context"
	"fmt"
	"os"
	"strings"

	"dogclaw/pkg/skills"
	"dogclaw/pkg/types"
)

// SkillTool implements the tool for discovering and reading skills
type SkillTool struct{}

func NewSkillTool() *SkillTool {
	return &SkillTool{}
}

func (t *SkillTool) Name() string {
	return "Skill"
}

func (t *SkillTool) Aliases() []string {
	return []string{"skill"}
}

func (t *SkillTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "The action to perform: 'list' to see all available skills, 'read' to read a skill's SKILL.md content",
				"enum":        []string{"list", "read"},
			},
			"name": map[string]any{
				"type":        "string",
				"description": "The skill name to read (required for 'read' action)",
			},
		},
		Required: []string{"action"},
	}
}

func (t *SkillTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Discover and read custom prompt-based skills. " +
		"Use 'list' to see all available skills and their descriptions, " +
		"'read' to get the full SKILL.md content of a specific skill."
}

func (t *SkillTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	action, _ := input["action"].(string)

	cwd := toolCtx.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	loadedSkills, err := skills.DiscoverSkills(cwd)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error discovering skills: %v", err),
			IsError: true,
		}, nil
	}

	switch action {
	case "list":
		return t.handleList(loadedSkills)
	case "read":
		name, _ := input["name"].(string)
		return t.handleRead(loadedSkills, name)
	default:
		return &types.ToolResult{
			Data:    fmt.Sprintf("Invalid action '%s'. Use 'list' or 'read'.", action),
			IsError: true,
		}, nil
	}
}

func (t *SkillTool) handleList(allSkills []*skills.Skill) (*types.ToolResult, error) {
	if len(allSkills) == 0 {
		return &types.ToolResult{
			Data: "No skills found.",
		}, nil
	}

	var sb strings.Builder
	sb.WriteString("Available Skills:\n\n")
	for _, s := range allSkills {
		sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", s.Name, s.FilePath, s.Description))
	}
	return &types.ToolResult{
		Data: sb.String(),
	}, nil
}

func (t *SkillTool) handleRead(allSkills []*skills.Skill, name string) (*types.ToolResult, error) {
	if name == "" {
		return &types.ToolResult{
			Data:    "Action 'read' requires a 'name' parameter (skill name).",
			IsError: true,
		}, nil
	}

	for _, s := range allSkills {
		if s.Name == name {
			content, err := os.ReadFile(s.FilePath)
			if err != nil {
				return &types.ToolResult{
					Data:    fmt.Sprintf("Error reading skill file %s: %v", s.FilePath, err),
					IsError: true,
				}, nil
			}
			return &types.ToolResult{
				Data: fmt.Sprintf("# Skill: %s\n\nSource: %s\n\n%s", s.Name, s.FilePath, string(content)),
			}, nil
		}
	}

	return &types.ToolResult{
		Data:    fmt.Sprintf("Skill '%s' not found. Use 'list' to see available skills.", name),
		IsError: true,
	}, nil
}

func (t *SkillTool) IsConcurrencySafe(input map[string]any) bool {
	return true
}

func (t *SkillTool) IsReadOnly(input map[string]any) bool {
	return true
}

func (t *SkillTool) IsDestructive(input map[string]any) bool {
	return false
}

func (t *SkillTool) IsEnabled() bool {
	return true
}

func (t *SkillTool) SearchHint() string {
	return "discover and read custom prompt-based skills"
}
