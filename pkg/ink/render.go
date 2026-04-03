// Package ink provides terminal UI rendering utilities.
// Translated from src/ink/
package ink

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// TextStyle represents ANSI text styling
type TextStyle struct {
	ForeGround string // ANSI escape code for foreground
	BackGround string // ANSI escape code for background
	Bold       bool
	Italic     bool
	Underline  bool
}

// DefaultStyle returns the default (reset) text style
func DefaultStyle() TextStyle {
	return TextStyle{}
}

// ToANSI converts a TextStyle to ANSI escape sequence
func (s TextStyle) ToANSI() string {
	var parts []string
	if s.ForeGround != "" {
		parts = append(parts, s.ForeGround)
	}
	if s.BackGround != "" {
		parts = append(parts, s.BackGround)
	}
	if s.Bold {
		parts = append(parts, "\033[1m")
	}
	if s.Italic {
		parts = append(parts, "\033[3m")
	}
	if s.Underline {
		parts = append(parts, "\033[4m")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "")
}

// ResetANSI is the ANSI reset sequence
const ResetANSI = "\033[0m"

// Colorize wraps text with ANSI color codes
func Colorize(text string, fg, bg string) string {
	if fg == "" && bg == "" {
		return text
	}
	var codes []string
	if fg != "" {
		codes = append(codes, fg)
	}
	if bg != "" {
		codes = append(codes, bg)
	}
	return strings.Join(codes, "") + text + ResetANSI
}

// TruncateToWidth truncates text to fit within the given display width
func TruncateToWidth(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}

	width := 0
	result := strings.Builder{}

	for _, r := range text {
		rw := runeWidth(r)
		if width+rw > maxWidth {
			break
		}
		result.WriteRune(r)
		width += rw
	}

	return result.String()
}

// runeWidth returns the display width of a rune
func runeWidth(r rune) int {
	// Simple implementation: CJK characters take 2 columns
	if r >= 0x1100 && (r <= 0x115F || r == 0x2329 || r == 0x232A ||
		(r >= 0x2E80 && r <= 0xA4CF && r != 0x303F) ||
		(r >= 0xAC00 && r <= 0xD7A3) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFE10 && r <= 0xFE19) ||
		(r >= 0xFE30 && r <= 0xFE6F) ||
		(r >= 0xFF00 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6) ||
		(r >= 0x20000 && r <= 0x2FFFD) ||
		(r >= 0x30000 && r <= 0x3FFFD)) {
		return 2
	}
	return 1
}

// StringWidth returns the display width of a string
func StringWidth(s string) int {
	width := 0
	for _, r := range s {
		width += runeWidth(r)
	}
	return width
}

// WrapText wraps text to the given width
func WrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	var result strings.Builder
	lineWidth := 0

	words := strings.Fields(text)
	for i, word := range words {
		wordWidth := StringWidth(word)

		if lineWidth+wordWidth > width {
			result.WriteString("\n")
			lineWidth = 0
		}

		if lineWidth > 0 {
			result.WriteString(" ")
			lineWidth++
		}

		result.WriteString(word)
		lineWidth += wordWidth

		_ = i // avoid unused variable
	}

	return result.String()
}

// PadRight pads text to the right with spaces
func PadRight(text string, width int) string {
	tw := StringWidth(text)
	if tw >= width {
		return text
	}
	return text + strings.Repeat(" ", width-tw)
}

// PadLeft pads text to the left with spaces
func PadLeft(text string, width int) string {
	tw := StringWidth(text)
	if tw >= width {
		return text
	}
	return strings.Repeat(" ", width-tw) + text
}

// Center centers text within the given width
func Center(text string, width int) string {
	tw := StringWidth(text)
	if tw >= width {
		return text
	}
	padding := width - tw
	left := padding / 2
	right := padding - left
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
}

// StripANSI removes all ANSI escape sequences from text
func StripANSI(s string) string {
	var result strings.Builder
	inEscape := false

	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' || r == 'H' || r == 'J' || r == 'K' || r == 'A' || r == 'B' || r == 'C' || r == 'D' {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}

	return result.String()
}

// VisibleLength returns the visible length of text (excluding ANSI codes)
func VisibleLength(s string) int {
	return utf8.RuneCountInString(StripANSI(s))
}

// TextSegment represents a segment of text with styling
type TextSegment struct {
	Text  string
	Style TextStyle
}

// RenderSegments renders text segments with ANSI styling
func RenderSegments(segments []TextSegment) string {
	var sb strings.Builder
	currentStyle := DefaultStyle()

	for _, seg := range segments {
		if seg.Style != currentStyle {
			if currentStyle != DefaultStyle() {
				sb.WriteString(ResetANSI)
			}
			if ansi := seg.Style.ToANSI(); ansi != "" {
				sb.WriteString(ansi)
			}
			currentStyle = seg.Style
		}
		sb.WriteString(seg.Text)
	}

	if currentStyle != DefaultStyle() {
		sb.WriteString(ResetANSI)
	}

	return sb.String()
}

// Screen represents a terminal screen buffer
type Screen struct {
	Width  int
	Height int
	Lines  []string
}

// NewScreen creates a new screen buffer
func NewScreen(width, height int) *Screen {
	return &Screen{
		Width:  width,
		Height: height,
		Lines:  make([]string, 0, height),
	}
}

// WriteLine writes a line to the screen
func (s *Screen) WriteLine(line string) {
	truncated := TruncateToWidth(line, s.Width)
	s.Lines = append(s.Lines, truncated)
}

// Render renders the screen to a string
func (s *Screen) Render() string {
	return strings.Join(s.Lines, "\n")
}

// Clear clears the screen
func (s *Screen) Clear() {
	s.Lines = s.Lines[:0]
}

// Cursor movements
const (
	CursorUp    = "\033[%dA"
	CursorDown  = "\033[%dB"
	CursorRight = "\033[%dC"
	CursorLeft  = "\033[%dD"
	CursorHome  = "\033[H"
	ClearScreen = "\033[2J"
	ClearLine   = "\033[2K"
)

// MoveCursor returns a cursor movement sequence
func MoveCursor(direction string, count int) string {
	return fmt.Sprintf("\033[%d%s", count, direction)
}

// ClearScreenCode returns the clear screen escape sequence
func ClearScreenCode() string {
	return ClearScreen + CursorHome
}

// ClearLineCode returns the clear line escape sequence
func ClearLineCode() string {
	return ClearLine
}

// HideCursor hides the cursor
func HideCursor() string {
	return "\033[?25l"
}

// ShowCursor shows the cursor
func ShowCursor() string {
	return "\033[?25h"
}
