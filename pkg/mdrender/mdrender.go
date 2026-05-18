
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
// It disables background color detection to fix macOS zsh escape code issues.
func getRenderer() (*glamour.TermRenderer, error) {
	rendererOnce.Do(func() {
		// Force set GLAMOUR_STYLE to dark before initializing to prevent
		// background color detection issues on macOS zsh
		originalGlamourStyle := os.Getenv("GLAMOUR_STYLE")
		os.Setenv("GLAMOUR_STYLE", "dark")
		defer func() {
			if originalGlamourStyle != "" {
				os.Setenv("GLAMOUR_STYLE", originalGlamourStyle)
			} else {
				os.Unsetenv("GLAMOUR_STYLE")
			}
		}()

		// Detect terminal width for word wrapping
		width := getTerminalWidth()

		// Explicitly use "dark" style and disable background color detection
		// to prevent issues like "1;rgb:fae0/fae0/fae0" on macOS zsh
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(width),
		)
		if err != nil {
			// Fallback to no style at all
			r, err = glamour.NewTermRenderer(
				glamour.WithWordWrap(width),
			)
			if err != nil {
				rendererErr = err
				return
			}
		}
		renderer = r
	})
	return renderer, rendererErr
}

// Render converts Markdown text to ANSI-formatted terminal output.
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
