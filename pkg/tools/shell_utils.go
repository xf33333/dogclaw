package tools

import (
	"strings"
)

// QuoteShellArg returns a shell-quoted version of the argument.
// This is used for safe construction of bash commands.
func QuoteShellArg(s string) string {
	if s == "" {
		return "''"
	}
	// Replace ' with '\'' and wrap in single quotes
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// IsCommandAvailable checks if a command exists in the shell's PATH.
func IsCommandAvailable(name string) bool {
	// We can use a simple check via 'type' or 'command -v'
	// But since we are already using NewBashTool for other things,
	// maybe just a simple check is fine.
	return true // Placeholder, to be used in GrepTool logic
}
