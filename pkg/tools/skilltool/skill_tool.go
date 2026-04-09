package skilltool

import (
	"context"
	"fmt"
	"os"
	"strings"

	"dogclaw/internal/config"
	"dogclaw/pkg/bootstrap" // Assuming we need session ID or CWD from here
	"dogclaw/pkg/skills"
	"dogclaw/pkg/types"
	"path/filepath"
)

// SkillTool implements the tool for discovering and using skills
type SkillTool struct {
	// Cache or reference to skills
}

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
				"description": "The action to perform: 'list', 'search', 'run', or 'install'",
				"enum":        []string{"list", "search", "run", "install"},
			},
			"query": map[string]any{
				"type":        "string",
				"description": "The search query or skill name to run",
			},
			"arguments": map[string]any{
				"type":        "object",
				"description": "Arguments for the skill (key-value pairs)",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The markdown content for the skill (required for 'install' action)",
			},
		},
		Required: []string{"action"},
	}
}

func (t *SkillTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Discover custom prompt-based skills. " +
		"Use 'list' to see all skills, 'search' to find relevant skills, " +
		"if user want to install skill,you should manually create the directory in ~/.dogclaw/skills/<skill-name>/ and write all necessary files (SKILL.md, scripts, etc.) using file tools."
}

func (t *SkillTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	action, _ := input["action"].(string)
	query, _ := input["query"].(string)

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
	case "search":
		return t.handleSearch(loadedSkills, query)
	case "run":
		args, _ := input["arguments"].(map[string]any)
		return t.handleRun(loadedSkills, query, args, cwd)

	default:
		return &types.ToolResult{
			Data:    "Invalid action",
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
		if s.IsUserInvocable() {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", s.Name, s.Description))
		}
	}
	return &types.ToolResult{
		Data: sb.String(),
	}, nil
}

func (t *SkillTool) handleSearch(allSkills []*skills.Skill, query string) (*types.ToolResult, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for '%s':\n\n", query))

	count := 0
	queryLower := strings.ToLower(query)
	for _, s := range allSkills {
		if strings.Contains(strings.ToLower(s.Name), queryLower) ||
			strings.Contains(strings.ToLower(s.Description), queryLower) {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", s.Name, s.Description))
			count++
		}
	}

	if count == 0 {
		return &types.ToolResult{
			Data: "No matching skills found.",
		}, nil
	}

	return &types.ToolResult{
		Data: sb.String(),
	}, nil
}

func (t *SkillTool) handleRun(allSkills []*skills.Skill, skillName string, inputArgs map[string]any, cwd string) (*types.ToolResult, error) {
	var target *skills.Skill
	for _, s := range allSkills {
		if s.Name == skillName {
			target = s
			break
		}
	}

	if target == nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Skill '%s' not found.", skillName),
			IsError: true,
		}, nil
	}

	// Prepare string arguments
	args := make(map[string]string)
	for k, v := range inputArgs {
		args[k] = fmt.Sprintf("%v", v)
	}

	// Session ID from bootstrap
	sessionID := string(bootstrap.GetSessionID())

	content := skills.SubstituteVariables(target.MarkdownContent, args, target.BaseDir, sessionID)

	return &types.ToolResult{
		Data: fmt.Sprintf("Executed skill '%s'. Resulting prompt:\n\n%s", skillName, content),
	}, nil
}

func (t *SkillTool) handleInstall(name string, content string) (*types.ToolResult, error) {
	if name == "" {
		return &types.ToolResult{
			Data:    "Action 'install' requires a 'query' (skill name).",
			IsError: true,
		}, nil
	}
	if content == "" {
		return &types.ToolResult{
			Data:    "Action 'install' requires 'content'.",
			IsError: true,
		}, nil
	}

	settingsDir, err := config.GetSettingsDir()
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error getting settings directory: %v", err),
			IsError: true,
		}, nil
	}

	skillDir := filepath.Join(settingsDir, "skills", name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error creating skill directory: %v", err),
			IsError: true,
		}, nil
	}

	skillFilePath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFilePath, []byte(content), 0644); err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error writing skill file: %v", err),
			IsError: true,
		}, nil
	}

	return &types.ToolResult{
		Data: fmt.Sprintf("Successfully installed skill '%s' to %s", name, skillFilePath),
	}, nil
}

func (t *SkillTool) IsConcurrencySafe(input map[string]any) bool {
	return true
}

func (t *SkillTool) IsReadOnly(input map[string]any) bool {
	action, _ := input["action"].(string)
	return action != "run" // 'run' might technically lead to non-readonly actions if the prompt asks for it
}

func (t *SkillTool) IsDestructive(input map[string]any) bool {
	return false
}

func (t *SkillTool) IsEnabled() bool {
	return false
}

func (t *SkillTool) SearchHint() string {
	return "discover and run custom prompt-based skills"
}
