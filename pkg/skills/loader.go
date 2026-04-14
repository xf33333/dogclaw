package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadSkillsFromDir loads all skills from a directory.
// Each skill must be in a subdirectory containing a SKILL.md file.
func LoadSkillsFromDir(basePath string, source SkillSource) ([]*Skill, error) {
	entries, err := os.ReadDir(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Silently skip if directory doesn't exist
		}
		return nil, fmt.Errorf("failed to read skills directory %s: %w", basePath, err)
	}

	var skills []*Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillName := entry.Name()
		skillDirPath := filepath.Join(basePath, skillName)
		skillFilePath := filepath.Join(skillDirPath, "SKILL.md")

		skill, err := LoadSkillFromFile(skillFilePath, skillName, source)
		if err != nil {
			// For now, we print a warning. In a real app we might want to log this properly.
			fmt.Fprintf(os.Stderr, "Warning: failed to load skill from %s: %v\n", skillFilePath, err)
			continue
		}
		if skill != nil {
			skill.BaseDir = skillDirPath
			skills = append(skills, skill)
		}
	}

	return skills, nil
}

// LoadSkillFromFile loads a skill from a specific SKILL.md file
func LoadSkillFromFile(filePath string, name string, source SkillSource) (*Skill, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read skill file: %w", err)
	}

	return ParseSkill(string(content), filePath, name, source)
}

// ParseSkill parses the content of a SKILL.md file (YAML frontmatter + Markdown)
func ParseSkill(content string, filePath string, name string, source SkillSource) (*Skill, error) {
	const separator = "---"

	trimmedContent := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmedContent, separator) {
		return &Skill{
			Name:            name,
			DisplayName:     name,
			Description:     "",
			MarkdownContent: content,
			Source:          source,
			FilePath:        filePath,
		}, nil
	}

	// Find the second separator
	parts := strings.SplitN(trimmedContent, separator, 3)
	// strings.SplitN with separator "---" on "---yaml---markdown"
	// parts[0] is empty (before the first ---)
	// parts[1] is the yaml
	// parts[2] is the markdown

	if len(parts) < 3 {
		return &Skill{
			Name:            name,
			DisplayName:     name,
			Description:     "",
			MarkdownContent: content,
			Source:          source,
			FilePath:        filePath,
		}, nil
	}

	yamlContent := parts[1]
	markdownContent := strings.TrimSpace(parts[2])

	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	skill := &Skill{
		Name:            name,
		DisplayName:     fm.Name,
		Description:     fm.Description,
		MarkdownContent: markdownContent,
		Frontmatter:     fm,
		Source:          source,
		FilePath:        filePath,
	}

	if skill.DisplayName == "" {
		skill.DisplayName = name
	}

	return skill, nil
}

// DiscoverSkills looks for skills in standard locations:
// 1. Current directory skills directory (./skills) - HIGHEST PRIORITY
// 2. User skills ( ~/.dogclaw/skills)
// 3. Project skills (.dogclaw/skills in project root and parent dirs)
func DiscoverSkills(cwd string) ([]*Skill, error) {
	var allSkills []*Skill

	//Project skills (walking up from cwd)
	curr := cwd
	//for {
	projectSkillsDir := filepath.Join(curr, ".dogclaw", "skills")
	skills, _ := LoadSkillsFromDir(projectSkillsDir, SourceProject)
	allSkills = append(allSkills, skills...)

	//parent := filepath.Dir(curr)
	//if parent == curr {
	//	break
	//}
	//curr = parent
	//}

	// 1. Current directory skills directory (./skills) - HIGHEST PRIORITY
	currSkillsDir := filepath.Join(cwd, "skills")
	skills, _ = LoadSkillsFromDir(currSkillsDir, SourceProject)
	allSkills = append(allSkills, skills...)

	// 2. User skills
	home, err := os.UserHomeDir()
	if err == nil {
		userSkillsDir := filepath.Join(home, ".dogclaw", "skills")
		skills, _ := LoadSkillsFromDir(userSkillsDir, SourceUser)
		allSkills = append(allSkills, skills...)

		agentSkillsDir := filepath.Join(home, ".agents", "skills")
		agentSkills, _ := LoadSkillsFromDir(agentSkillsDir, SourceUser)
		allSkills = append(allSkills, agentSkills...)

	}

	// Deduplicate by name, keeping the first occurrence (priority order)
	seen := make(map[string]bool)
	dedupedSkills := []*Skill{}
	for _, skill := range allSkills {
		if !seen[skill.Name] {
			seen[skill.Name] = true
			dedupedSkills = append(dedupedSkills, skill)
		}
	}

	return dedupedSkills, nil
}
 
