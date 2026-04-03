package sessionmemory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Section represents a section in the session memory template
type Section struct {
	Name        string
	Description string
}

// DefaultSections defines the default session memory template sections
var DefaultSections = []Section{
	{
		Name:        "Session Title",
		Description: "5-10 word descriptive title of what this session is about",
	},
	{
		Name:        "Current State",
		Description: "What is currently being worked on right now",
	},
	{
		Name:        "Task Specification",
		Description: "What user is asking to build? Design decisions",
	},
	{
		Name:        "Files and Functions",
		Description: "Important files and their purposes",
	},
	{
		Name:        "Workflow",
		Description: "Common bash commands run and their explanations",
	},
	{
		Name:        "Errors and Corrections",
		Description: "Errors encountered and how they were fixed",
	},
	{
		Name:        "Codebase and System Documentation",
		Description: "Important system components and how they work",
	},
	{
		Name:        "Learnings",
		Description: "What works / what doesn't / what to avoid",
	},
	{
		Name:        "Key Results",
		Description: "Specific outputs the user requested",
	},
	{
		Name:        "Worklog",
		Description: "Step-by-step summary of what was tried and what has been completed",
	},
}

// GenerateDefaultTemplate generates the default session memory template
func GenerateDefaultTemplate() string {
	var sb strings.Builder

	for i, section := range DefaultSections {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("# %s\n", section.Name))
		sb.WriteString(fmt.Sprintf("_%s_\n", section.Description))
	}

	return sb.String()
}

// Template holds the session memory template
type Template struct {
	Sections []Section
	Raw      string
}

// LoadTemplate loads the template from a custom path or uses the default
func LoadTemplate(customPath string) (*Template, error) {
	if customPath != "" {
		data, err := os.ReadFile(customPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read custom template %s: %w", customPath, err)
		}
		return &Template{
			Raw: string(data),
		}, nil
	}

	// Use default template
	defaultContent := GenerateDefaultTemplate()
	return &Template{
		Sections: DefaultSections,
		Raw:      defaultContent,
	}, nil
}

// GetSections returns the sections from the template
func (t *Template) GetSections() []Section {
	if len(t.Sections) > 0 {
		return t.Sections
	}

	// Parse sections from raw template
	var sections []Section
	lines := strings.Split(t.Raw, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			sections = append(sections, Section{
				Name:        strings.TrimPrefix(line, "# "),
				Description: "",
			})
		} else if strings.HasPrefix(line, "_") && strings.HasSuffix(line, "_") {
			if len(sections) > 0 {
				sections[len(sections)-1].Description = strings.Trim(line, "_ ")
			}
		}
	}

	t.Sections = sections
	return sections
}

// EnsureSessionFile ensures the SESSION.md file exists for the given session
func EnsureSessionFile(sessionDir, sessionID string) (string, error) {
	dir := filepath.Join(sessionDir, sessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create session directory: %w", err)
	}

	path := filepath.Join(dir, "SESSION.md")

	// Check if already exists
	if _, err := os.Stat(path); err == nil {
		return path, nil // Already exists
	}

	// Create with default template
	template := GenerateDefaultTemplate()
	if err := os.WriteFile(path, []byte(template), 0644); err != nil {
		return "", fmt.Errorf("failed to create session file: %w", err)
	}

	return path, nil
}

// ReadSessionFile reads the current session memory file
func ReadSessionFile(sessionDir, sessionID string) (string, error) {
	path := filepath.Join(sessionDir, sessionID, "SESSION.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // Not yet created
		}
		return "", fmt.Errorf("failed to read session file: %w", err)
	}
	return string(data), nil
}

// WriteSessionFile writes updated content to the session memory file
func WriteSessionFile(sessionDir, sessionID, content string) error {
	dir := filepath.Join(sessionDir, sessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	path := filepath.Join(dir, "SESSION.md")
	return os.WriteFile(path, []byte(content), 0644)
}

// SessionFilePath returns the path to the session memory file
func SessionFilePath(sessionDir, sessionID string) string {
	return filepath.Join(sessionDir, sessionID, "SESSION.md")
}
