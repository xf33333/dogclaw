// Package mdrender provides Markdown-to-terminal rendering for CLI output.
// It uses glamour to convert Markdown text into ANSI-formatted terminal output,
// supporting headings, bold, italic, code blocks, lists, links, etc.
package mdrender

import (
	"os"
	"sync"

	"github.com/charmbracelet/glamour"
)

var (
	renderer     *glamour.TermRenderer
	rendererOnce sync.Once
	rendererErr  error
	forceRender  bool // when true, always render regardless of TTY check
)

// EnableForceRender enables rendering even when stdout is not detected as a TTY.
// This is useful for CLI agent mode where readline handles the output directly.
func EnableForceRender() {
	forceRender = true
}

// getRenderer returns a cached glamour renderer instance.
// It auto-detects whether the terminal supports dark or light mode.
func getRenderer() (*glamour.TermRenderer, error) {
	rendererOnce.Do(func() {
		// Detect terminal width for word wrapping
		width := getTerminalWidth()

		// Determine style: auto-detect dark/light, or use dark as fallback
		// when forceRender is enabled (since auto-detect may fail in non-TTY)
		style := glamour.WithAutoStyle()

		if forceRender && !isTerminal() {
			// When forceRender is on but not a real TTY (rare edge case),
			// default to dark style which is the most common terminal theme
			if isLightTerminal() {
				style = glamour.WithStandardStyle("light")
			} else {
				style = glamour.WithStandardStyle("dark")
			}
		} else if isLightTerminal() {
			// Explicit light background override
			style = glamour.WithStandardStyle("light")
		}

		r, err := glamour.NewTermRenderer(
			style,
			glamour.WithWordWrap(width),
		)
		if err != nil {
			rendererErr = err
			return
		}
		renderer = r
	})
	return renderer, rendererErr
}

// Render converts Markdown text to ANSI-formatted terminal output.
// If rendering fails, it falls back to the original text.
// When stdout is not a TTY (e.g., piped output), it returns plain text
// unless forceRender is enabled.
func Render(text string) string {
	// Skip rendering if not a terminal and force is not enabled
	if !forceRender && !isTerminal() {
		return text
	}

	r, err := getRenderer()
	if err != nil {
		return text
	}

	rendered, err := r.Render(text)
	if err != nil {
		return text
	}

	return rendered
}

// isTerminal checks if stdout is a terminal (TTY).
func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// isLightTerminal checks if the terminal has a light background
// by inspecting environment variables.
func isLightTerminal() bool {
	// Check for explicit color scheme setting
	colorterm := os.Getenv("COLORTERM")
	if colorterm == "light" || colorterm == "Light" {
		return true
	}

	// Some terminals set COLORSCHEME
	colorscheme := os.Getenv("COLORSCHEME")
	if colorscheme == "light" || colorscheme == "Light" {
		return true
	}

	// Check GLAMOUR_STYLE environment variable (glamour's built-in)
	glamourStyle := os.Getenv("GLAMOUR_STYLE")
	if glamourStyle == "light" {
		return true
	}

	return false
}

// getTerminalWidth returns the terminal width for word wrapping.
// Falls back to 80 if detection fails.
func getTerminalWidth() int {
	// Try to get from COLUMNS env var first
	if cols := os.Getenv("COLUMNS"); cols != "" {
		var w int
		for _, c := range cols {
			if c >= '0' && c <= '9' {
				w = w*10 + int(c-'0')
			} else {
				break
			}
		}
		if w > 0 && w <= 1000 {
			return w
		}
	}

	// Default width
	return 80
}
