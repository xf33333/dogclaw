package skills

import (
)

// SkillSource defines where a skill was loaded from
type SkillSource string

const (
	SourceManaged SkillSource = "managed"
	SourceUser    SkillSource = "user"
	SourceProject SkillSource = "project"
	SourceBundled SkillSource = "bundled"
)

// Frontmatter represents the YAML frontmatter of a SKILL.md file
type Frontmatter struct {
	Name                   string   `yaml:"name"`
	Description            string   `yaml:"description"`
	Arguments              any      `yaml:"arguments"` // can be string or []string
	WhenToUse              string   `yaml:"when_to_use"`
	Version                string   `yaml:"version"`
	Model                  string   `yaml:"model"`
	DisableModelInvocation bool     `yaml:"disable-model-invocation"`
	UserInvocable          *bool    `yaml:"user-invocable"`
	AllowedTools           []string `yaml:"allowed-tools"`
	Paths                  any      `yaml:"paths"` // can be string or []string
	Effort                 string   `yaml:"effort"`
	Context                string   `yaml:"context"`
	Agent                  string   `yaml:"agent"`
}

// Skill represents a loaded skill
type Skill struct {
	Name            string
	DisplayName     string
	Description     string
	MarkdownContent string
	Frontmatter     Frontmatter
	Source          SkillSource
	BaseDir         string
	FilePath        string
	LoadedFrom      string
}

// ToToolDescription converts a skill to a tool description string
func (s *Skill) ToToolDescription() string {
	if s.Frontmatter.Description != "" {
		return s.Frontmatter.Description
	}
	return s.Description
}

// GetArgumentNames returns the argument names for the skill
func (s *Skill) GetArgumentNames() []string {
	switch v := s.Frontmatter.Arguments.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []string:
		return v
	case []any:
		var result []string
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	default:
		return nil
	}
}

// GetPaths returns the path patterns for the skill
func (s *Skill) GetPaths() []string {
	switch v := s.Frontmatter.Paths.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []string:
		return v
	case []any:
		var result []string
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	default:
		return nil
	}
}

// IsUserInvocable returns true if the skill can be invoked by the user
func (s *Skill) IsUserInvocable() bool {
	if s.Frontmatter.UserInvocable == nil {
		return true
	}
	return *s.Frontmatter.UserInvocable
}
