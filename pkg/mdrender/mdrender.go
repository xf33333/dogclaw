
// Package mdrender provides Markdown-to-terminal rendering for CLI output.
// It uses glamour to convert Markdown text into ANSI-formatted terminal output,
// supporting headings, bold, italic, code blocks, lists, links, etc.
package mdrender

import (
	"os"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/muesli/termenv"
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
func getRenderer() (*glamour.TermRenderer, error) {
	rendererOnce.Do(func() {
		// Detect terminal width for word wrapping
		width := getTerminalWidth()

		// Determine the style to use - use termenv for SAFE background color detection!
		styleName := "dark"
		if envStyle := os.Getenv("GLAMOUR_STYLE"); envStyle != "" {
			styleName = envStyle
		} else {
			// Use termenv's safe background color detection
			if termenv.HasDarkBackground() {
				styleName = "dark"
			} else {
				styleName = "light"
			}
		}

		// Critical fix: temporarily unset COLORTERM and TERM_PROGRAM
		// to prevent glamour from trying to query the terminal background color,
		// which causes the "1;rgb:fae0/fae0/fae0" issue on macOS zsh
		originalColorterm := os.Getenv("COLORTERM")
		originalTermProgram := os.Getenv("TERM_PROGRAM")
		originalTermProgramVersion := os.Getenv("TERM_PROGRAM_VERSION")
		os.Unsetenv("COLORTERM")
		os.Unsetenv("TERM_PROGRAM")
		os.Unsetenv("TERM_PROGRAM_VERSION")

		defer func() {
			// Restore original env vars after renderer is created
			if originalColorterm != "" {
				os.Setenv("COLORTERM", originalColorterm)
			}
			if originalTermProgram != "" {
				os.Setenv("TERM_PROGRAM", originalTermProgram)
			}
			if originalTermProgramVersion != "" {
				os.Setenv("TERM_PROGRAM_VERSION", originalTermProgramVersion)
			}
		}()

		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle(styleName),
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
