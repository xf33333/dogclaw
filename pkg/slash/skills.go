package slash

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"dogclaw/pkg/usage"
)

// Skill represents a loaded skill from a .md file
type Skill struct {
	Name        string
	Description string
	Content     string // Full markdown content
	Source      string // "bundled", "project", "user", "plugin"
	Path        string
}

// SkillRegistry manages skill discovery and loading
type SkillRegistry struct {
	skills map[string]*Skill
	mu     sync.RWMutex
	cached bool
}

// NewSkillRegistry creates a new skill registry
func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{
		skills: make(map[string]*Skill),
	}
}

// Register adds a skill manually
func (r *SkillRegistry) Register(skill *Skill) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[skill.Name] = skill
}

// Get retrieves a skill by name
func (r *SkillRegistry) Get(name string) *Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.skills[name]
}

// List returns all registered skills
func (r *SkillRegistry) List() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Skill
	for _, s := range r.skills {
		result = append(result, s)
	}
	return result
}

// DiscoverBundledSkills loads skills from the bundled skills directory
func (r *SkillRegistry) DiscoverBundledSkills() error {
	// In a real implementation, this would read from embedded FS
	// or a known bundled skills path
	return nil
}

// DiscoverProjectSkills loads skills from .dogclaw/skills/ in the project
func (r *SkillRegistry) DiscoverProjectSkills(cwd string) error {
	skillsDir := filepath.Join(cwd, ".dogclaw", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No skills dir is fine
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			skillPath := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
			if _, err := os.Stat(skillPath); err == nil {
				skill, err := loadSkillFromFile(skillPath, "project")
				if err != nil {
					continue
				}
				r.Register(skill)
			}
		}
	}

	return nil
}

// DiscoverUserSkills loads skills from ~/.dogclaw/skills/
func (r *SkillRegistry) DiscoverUserSkills() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	skillsDir := filepath.Join(homeDir, ".dogclaw", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			skillPath := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
			if _, err := os.Stat(skillPath); err == nil {
				skill, err := loadSkillFromFile(skillPath, "user")
				if err != nil {
					continue
				}
				r.Register(skill)
			}
		}
	}

	return nil
}

// DiscoverAll runs all discovery methods
func (r *SkillRegistry) DiscoverAll(cwd string) error {
	r.cached = false
	if err := r.DiscoverBundledSkills(); err != nil {
		return fmt.Errorf("bundled skills: %w", err)
	}
	if err := r.DiscoverProjectSkills(cwd); err != nil {
		return fmt.Errorf("project skills: %w", err)
	}
	if err := r.DiscoverUserSkills(); err != nil {
		return fmt.Errorf("user skills: %w", err)
	}
	r.cached = true
	return nil
}

// loadSkillFromFile reads a SKILL.md file and extracts metadata
func loadSkillFromFile(path string, source string) (*Skill, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	contentStr := string(content)
	name := filepath.Base(filepath.Dir(path))

	// Extract description from first line or frontmatter
	description := extractDescription(contentStr)

	return &Skill{
		Name:        name,
		Description: description,
		Content:     contentStr,
		Source:      source,
		Path:        path,
	}, nil
}

// extractDescription gets the first meaningful line from markdown
func extractDescription(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "#") {
			continue
		}
		// Limit to first 100 chars
		if len(line) > 100 {
			return line[:100] + "..."
		}
		return line
	}
	return ""
}

// FormatSkillsForPrompt formats all skills for inclusion in the system prompt
func (r *SkillRegistry) FormatSkillsForPrompt() string {
	skills := r.List()
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Available Skills\n\n")
	sb.WriteString("The following specialized skills are available. Use them when relevant:\n\n")

	for _, skill := range skills {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", skill.Name, skill.Description))
	}

	sb.WriteString("\n")
	return sb.String()
}

// HandleSkillsCommand implements the /skills slash command
func HandleSkillsCommand(ctx context.Context, args string, registry *SkillRegistry) (*CommandResult, error) {
	args = strings.TrimSpace(args)

	if args == "" {
		// List all skills
		skills := registry.List()
		if len(skills) == 0 {
			return &CommandResult{
				Output: "No skills installed. Add skills to .dogclaw/skills/ or ~/.dogclaw/skills/",
			}, nil
		}

		var sb strings.Builder
		sb.WriteString("Available skills:\n\n")
		for _, skill := range skills {
			sb.WriteString(fmt.Sprintf("  %-20s %s [%s]\n", skill.Name, skill.Description, skill.Source))
		}

		return &CommandResult{Output: sb.String()}, nil
	}

	if args == "refresh" {
		// Re-discover skills
		cwd, _ := os.Getwd()
		if err := registry.DiscoverAll(cwd); err != nil {
			return &CommandResult{IsError: true, ErrorMsg: err.Error()}, nil
		}
		return &CommandResult{Output: "Skills refreshed successfully."}, nil
	}

	// Show specific skill
	skill := registry.Get(args)
	if skill == nil {
		return &CommandResult{
			IsError:  true,
			ErrorMsg: fmt.Sprintf("Skill not found: %s", args),
		}, nil
	}

	return &CommandResult{
		Output: fmt.Sprintf("Skill: %s\nSource: %s\nPath: %s\nDescription: %s\n\n--- Content ---\n%s",
			skill.Name, skill.Source, skill.Path, skill.Description, skill.Content),
	}, nil
}

// HandleUsageCommand implements the /usage command with the usage tracker
func HandleUsageCommand(ctx context.Context, args string, tracker *usage.AccumulatedUsage) (*CommandResult, error) {
	if tracker == nil {
		return &CommandResult{Output: "No usage data available."}, nil
	}

	pricing := usage.GetPricingForModel("sonnet")

	var sb strings.Builder
	sb.WriteString("=== Session Usage ===\n\n")
	sb.WriteString(fmt.Sprintf("Input tokens:     %d\n", tracker.TotalInput))
	sb.WriteString(fmt.Sprintf("Output tokens:    %d\n", tracker.TotalOutput))
	sb.WriteString(fmt.Sprintf("Cache read:       %d\n", tracker.TotalCacheRead))
	sb.WriteString(fmt.Sprintf("Cache creation:   %d\n", tracker.TotalCacheCreation))
	sb.WriteString(fmt.Sprintf("Total tokens:     %d\n", tracker.TotalTokens()))
	sb.WriteString(fmt.Sprintf("Turns:            %d\n", tracker.Turns))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Estimated cost:   %s\n", tracker.FormatCost(pricing)))

	return &CommandResult{Output: sb.String()}, nil
}
