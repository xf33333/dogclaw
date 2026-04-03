package memory

import (
	"fmt"
	"math"
	"time"
)

// MemoryAgeDays returns the number of whole days since mtime.
// Floor-rounded: 0 for today, 1 for yesterday, 2+ for older.
// Negative inputs (future mtime, clock skew) clamp to 0.
func MemoryAgeDays(mtimeMs int64) int {
	nowMs := time.Now().UnixMilli()
	diff := nowMs - mtimeMs
	if diff < 0 {
		return 0
	}
	return int(math.Floor(float64(diff) / 86_400_000))
}

// MemoryAge returns a human-readable age string.
// Models are poor at date arithmetic — a raw ISO timestamp doesn't
// trigger staleness reasoning the way "47 days ago" does.
func MemoryAge(mtimeMs int64) string {
	d := MemoryAgeDays(mtimeMs)
	if d == 0 {
		return "today"
	}
	if d == 1 {
		return "yesterday"
	}
	return fmt.Sprintf("%d days ago", d)
}

// MemoryFreshnessText returns a plain-text staleness caveat for
// memories older than 1 day. Returns empty string for fresh memories.
//
// Use this when the consumer already provides its own wrapping
// (e.g. messages.ts relevant_memories → wrapMessagesInSystemReminder).
//
// Motivated by user reports of stale code-state memories (file:line
// citations to code that has since changed) being asserted as fact —
// the citation makes the stale claim sound more authoritative, not less.
func MemoryFreshnessText(mtimeMs int64) string {
	d := MemoryAgeDays(mtimeMs)
	if d <= 1 {
		return ""
	}
	return fmt.Sprintf(
		"This memory is %d days old. "+
			"Memories are point-in-time observations, not live state — "+
			"claims about code behavior or file:line citations may be outdated. "+
			"Verify against current code before asserting as fact.",
		d,
	)
}

// MemoryFreshnessNote returns a per-memory staleness note wrapped in
// <system-reminder> tags. Returns empty string for memories ≤ 1 day old.
// Use this for callers that don't add their own system-reminder wrapper
// (e.g. FileReadTool output).
func MemoryFreshnessNote(mtimeMs int64) string {
	text := MemoryFreshnessText(mtimeMs)
	if text == "" {
		return ""
	}
	return "<system-reminder>" + text + "</system-reminder>\n"
}

// MtimeMsFromFileInfo returns the modification time in milliseconds
// from a time.Time value.
func MtimeMsFromFileInfo(t time.Time) int64 {
	return t.UnixMilli()
}

// MtimeMsNow returns the current time in milliseconds.
func MtimeMsNow() int64 {
	return time.Now().UnixMilli()
}
