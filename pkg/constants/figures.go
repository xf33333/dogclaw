package constants

import "runtime"

// Unicode figures for UI display.
// Platform-specific: some characters aren't supported on Windows/Linux.
var (
	// BlackCircle — better vertically aligned, but usually not supported on Windows/Linux
	BlackCircle = func() string {
		if runtime.GOOS == "darwin" {
			return "\u23fa" // ⏺
		}
		return "\u25cf" // ●
	}()

	BulletOperator   = "\u2219" // ∙
	TeardropAsterisk = "\u273b" // ✻
	UpArrow          = "\u2191" // ↑ - used for opus 1m merge notice
	DownArrow        = "\u2193" // ↓ - used for scroll hint
	LightningBolt    = "\u21af" // ↯ - used for fast mode indicator
	EffortLow        = "\u25cb" // ○ - effort level: low
	EffortMedium     = "\u25d0" // ◐ - effort level: medium
	EffortHigh       = "\u25cf" // ● - effort level: high
	EffortMax        = "\u25c9" // ◉ - effort level: max (Opus 4.6 only)

	// Media/trigger status indicators
	PlayIcon  = "\u25b6" // ▶
	PauseIcon = "\u23f8" // ⏸

	// MCP subscription indicators
	RefreshArrow  = "\u21bb" // ↻ - used for resource update indicator
	ChannelArrow  = "\u2190" // ← - inbound channel message indicator
	InjectedArrow = "\u2192" // → - cross-session injected message indicator
	ForkGlyph     = "\u2442" // ⑂ - fork directive indicator

	// Review status indicators (ultrareview diamond states)
	DiamondOpen   = "\u25c7" // ◇ - running
	DiamondFilled = "\u25c6" // ◆ - completed/failed
	ReferenceMark = "\u203b" // ※ - komejirushi, away-summary recap marker

	// Issue flag indicator
	FlagIcon = "\u2691" // ⚑ - used for issue flag banner

	// Blockquote indicator
	BlockquoteBar   = "\u258e" // ▎ - left one-quarter block
	HeavyHorizontal = "\u2501" // ━ - heavy box-drawing horizontal

	// Bridge status indicators
	BridgeSpinnerFrames = [4]string{
		"\u00b7|\u00b7",      // ·|·
		"\u00b7/\u00b7",      // ·/·
		"\u00b7\u2014\u00b7", // ·—·
		"\u00b7\\\u00b7",     // ·\·
	}
	BridgeReadyIndicator  = "\u00b7\u2714\ufe0e\u00b7" // ·✔︎·
	BridgeFailedIndicator = "\u00d7"                   // ×
)
