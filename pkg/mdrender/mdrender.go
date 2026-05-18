
// Package mdrender provides Markdown-to-terminal rendering for CLI output.
// It uses glamour to convert Markdown text into ANSI-formatted terminal output,
// supporting headings, bold, italic, code blocks, lists, links, etc.
package mdrender

import (
	"os"
)

var (
	forceRender bool // when true, always render regardless of TTY check
)

// EnableForceRender enables rendering even when stdout is not detected as a TTY.
// This is useful for CLI agent mode where readline handles the output directly.
func EnableForceRender() {
	forceRender = true
}

// Render converts Markdown text to ANSI-formatted terminal output.
// For now, it just returns the text as-is to avoid escape code issues.
func Render(text string) string {
	return text
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
