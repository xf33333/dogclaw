package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSkill(t *testing.T) {
	content := `---
name: test-skill
description: A test skill
arguments:
  - name
  - value
---
Hello ${name}, your value is ${value}.
Your dir is ${CLAUDE_SKILL_DIR} and session is ${CLAUDE_SESSION_ID}.
`
	skill, err := ParseSkill(content, "test/SKILL.md", "test", SourceUser)
	if err != nil {
		t.Fatalf("ParseSkill failed: %v", err)
	}

	if skill.Name != "test" {
		t.Errorf("Expected Name 'test', got '%s'", skill.Name)
	}
	if skill.DisplayName != "test-skill" {
		t.Errorf("Expected DisplayName 'test-skill', got '%s'", skill.DisplayName)
	}
	if skill.Description != "A test skill" {
		t.Errorf("Expected Description 'A test skill', got '%s'", skill.Description)
	}

	argNames := skill.GetArgumentNames()
	if len(argNames) != 2 || argNames[0] != "name" || argNames[1] != "value" {
		t.Errorf("Expected arguments [name value], got %v", argNames)
	}

	// Test substitution
	args := map[string]string{
		"name":  "Alice",
		"value": "42",
	}
	result := SubstituteVariables(skill.MarkdownContent, args, "/path/to/skill", "session-123")
	
	expected := "Hello Alice, your value is 42.\nYour dir is /path/to/skill and session is session-123."
	if result != expected {
		t.Errorf("Substitution failed.\nExpected: %s\nGot:      %s", expected, result)
	}
}

func TestLoadSkillsFromDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "skilltest")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", tmpDir)
	}
	defer os.RemoveAll(tmpDir)

	skillDir := filepath.Join(tmpDir, "hello")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill dir: %v", err)
	}

	skillContent := `---
name: hello-skill
---
Hello world
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	skills, err := LoadSkillsFromDir(tmpDir, SourceUser)
	if err != nil {
		t.Fatalf("LoadSkillsFromDir failed: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("Expected 1 skill, got %d", len(skills))
	}

	if skills[0].Name != "hello" {
		t.Errorf("Expected skill name 'hello', got '%s'", skills[0].Name)
	}
}

func TestAllowedTools(t *testing.T) {
	content := `---
name: test-skill
allowed-tools: tool1, tool2, tool3
---
Content`
	skill, _ := ParseSkill(content, "test.md", "test", SourceUser)
	tools := skill.GetAllowedTools()
	if len(tools) != 3 || tools[0] != "tool1" || tools[1] != "tool2" || tools[2] != "tool3" {
		t.Errorf("Expected [tool1 tool2 tool3], got %v", tools)
	}

	contentList := `---
name: test-skill
allowed-tools:
  - t1
  - t2
---
Content`
	skill2, _ := ParseSkill(contentList, "test.md", "test", SourceUser)
	tools2 := skill2.GetAllowedTools()
	if len(tools2) != 2 || tools2[0] != "t1" || tools2[1] != "t2" {
		t.Errorf("Expected [t1 t2], got %v", tools2)
	}
}
