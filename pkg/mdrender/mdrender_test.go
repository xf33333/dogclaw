package mdrender

import (
	"strings"
	"sync"
	"testing"
)

func TestRenderPlainText(t *testing.T) {
	// Test that plain text is returned as-is when not a TTY and force not enabled
	input := "Hello, world!"
	output := Render(input)
	if output != input {
		t.Errorf("Render() for non-TTY should return original text, got: %q", output)
	}
}

func TestRenderWithForceRender(t *testing.T) {
	// Enable force render and test that Markdown gets rendered with ANSI codes
	EnableForceRender()
	// Reset the renderer so it picks up the new forceRender setting
	rendererOnce = sync.Once{}
	renderer = nil
	rendererErr = nil
	defer func() {
		forceRender = false
		rendererOnce = sync.Once{}
		renderer = nil
		rendererErr = nil
	}()

	input := "Hello, **world**!"
	output := Render(input)
	// With force render enabled, the output should contain ANSI escape codes
	if output == "" {
		t.Error("Render() with force enabled should return non-empty output")
	}
	// The rendered output should NOT contain raw ** markers
	if strings.Contains(output, "**") {
		t.Errorf("Render() with force enabled should not contain raw ** markers, got: %q", output)
	}
	// Should contain ANSI escape sequences (indicating formatting)
	if !strings.Contains(output, "\x1b[") {
		t.Errorf("Render() with force enabled should contain ANSI escape codes, got: %q", output)
	}
}

func TestRenderCodeBlockWithForce(t *testing.T) {
	EnableForceRender()
	rendererOnce = sync.Once{}
	renderer = nil
	rendererErr = nil
	defer func() {
		forceRender = false
		rendererOnce = sync.Once{}
		renderer = nil
		rendererErr = nil
	}()

	input := "```go\nfmt.Println(\"hello\")\n```"
	output := Render(input)
	if output == "" {
		t.Error("Render() should return non-empty output for code block")
	}
	// Should contain syntax-highlighted code (ANSI sequences)
	if !strings.Contains(output, "\x1b[") {
		t.Errorf("Render() should contain ANSI escape codes for code highlighting, got: %q", output)
	}
}

func TestRenderHeadingWithForce(t *testing.T) {
	EnableForceRender()
	rendererOnce = sync.Once{}
	renderer = nil
	rendererErr = nil
	defer func() {
		forceRender = false
		rendererOnce = sync.Once{}
		renderer = nil
		rendererErr = nil
	}()

	input := "# Hello World\n\nSome content here."
	output := Render(input)
	if output == "" {
		t.Error("Render() should return non-empty output for heading")
	}
	// Should NOT contain raw # heading marker
	if strings.Contains(output, "# Hello World") {
		t.Errorf("Render() should not contain raw heading marker, got: %q", output)
	}
}

func TestIsTerminal(t *testing.T) {
	// In test environment, stdout is not a terminal
	if isTerminal() {
		t.Error("isTerminal() should return false in test environment")
	}
}

func TestGetTerminalWidth(t *testing.T) {
	width := getTerminalWidth()
	if width <= 0 {
		t.Errorf("getTerminalWidth() should return positive value, got: %d", width)
	}
}

func TestIsLightTerminal(t *testing.T) {
	// Without env vars set, should return false
	result := isLightTerminal()
	if result {
		t.Error("isLightTerminal() should return false without env vars")
	}
}

func TestRenderEmptyString(t *testing.T) {
	output := Render("")
	if output != "" {
		t.Errorf("Render(empty) should return empty, got: %q", output)
	}
}

func TestRenderChineseTextWithForce(t *testing.T) {
	EnableForceRender()
	rendererOnce = sync.Once{}
	renderer = nil
	rendererErr = nil
	defer func() {
		forceRender = false
		rendererOnce = sync.Once{}
		renderer = nil
		rendererErr = nil
	}()

	input := "# 标题\n\n这是**粗体**和*斜体*文本。\n\n- 项目1\n- 项目2"
	output := Render(input)
	if output == "" {
		t.Error("Render() should return non-empty output for Chinese text")
	}
}

func TestRenderTableWithForce(t *testing.T) {
	EnableForceRender()
	rendererOnce = sync.Once{}
	renderer = nil
	rendererErr = nil
	defer func() {
		forceRender = false
		rendererOnce = sync.Once{}
		renderer = nil
		rendererErr = nil
	}()

	input := "| Name | Age |\n|------|-----|\n| Alice | 30 |\n| Bob | 25 |"
	output := Render(input)
	if output == "" {
		t.Error("Render() should return non-empty output for table")
	}
}

// TestRenderWithForcePassthrough tests that the rendering logic doesn't crash
// on various Markdown inputs when force render is enabled.
func TestRenderWithForcePassthrough(t *testing.T) {
	EnableForceRender()
	rendererOnce = sync.Once{}
	renderer = nil
	rendererErr = nil
	defer func() {
		forceRender = false
		rendererOnce = sync.Once{}
		renderer = nil
		rendererErr = nil
	}()

	testCases := []string{
		"# Heading 1\n## Heading 2\n### Heading 3",
		"**bold** *italic* ~~strikethrough~~",
		"> blockquote text",
		"1. Ordered list\n2. Second item",
		"- [ ] Task list\n- [x] Done task",
		"---\n***\n___",
		"`inline code`",
		"```javascript\nconsole.log('hi');\n```",
		strings.Repeat("Long paragraph ", 100),
	}

	for i, input := range testCases {
		output := Render(input)
		if output == "" && input != "" {
			t.Errorf("Test case %d: Render() returned empty for non-empty input", i)
		}
	}
}
