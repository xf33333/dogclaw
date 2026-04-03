package constants

import (
	"fmt"
	"os"
	"time"
)

// GetLocalISODate returns the local date in ISO format.
// Checks for CLAUDE_CODE_OVERRIDE_DATE env var first.
func GetLocalISODate() string {
	if override := os.Getenv("CLAUDE_CODE_OVERRIDE_DATE"); override != "" {
		return override
	}
	now := time.Now()
	return now.Format("2006-01-02")
}

// SessionStartDate is the session start date, captured once at init.
// In TS this is memoized; in Go we capture it at package init.
var SessionStartDate = GetLocalISODate()

// GetLocalMonthYear returns "Month YYYY" (e.g. "February 2026") in the user's local timezone.
// Changes monthly, not daily — used in tool prompts to minimize cache busting.
func GetLocalMonthYear() string {
	now := time.Now()
	if override := os.Getenv("CLAUDE_CODE_OVERRIDE_DATE"); override != "" {
		if t, err := time.Parse("2006-01-02", override); err == nil {
			now = t
		}
	}
	return now.Format("January 2006")
}

// FormatAttributionHeader returns the attribution header string.
// When NATIVE_CLIENT_ATTESTATION is enabled, includes a cch=00000 placeholder.
func FormatAttributionHeader(fingerprint, version, entrypoint, workload string) string {
	cch := ""
	if os.Getenv("NATIVE_CLIENT_ATTESTATION") != "" {
		cch = " cch=00000;"
	}
	workloadPair := ""
	if workload != "" {
		workloadPair = fmt.Sprintf(" cc_workload=%s;", workload)
	}
	return fmt.Sprintf("x-anthropic-billing-header: cc_version=%s.%s; cc_entrypoint=%s;%s%s",
		version, fingerprint, entrypoint, cch, workloadPair)
}
