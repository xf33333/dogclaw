package skills

import (
	"regexp"
	"strings"
)

var (
	// Regex to match ${name} or $name
	argRegex = regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)\}`)
)

// SubstituteVariables replaces variables in the markdown content
func SubstituteVariables(content string, args map[string]string, skillDir string, sessionID string) string {
	// 1. Substitute session ID and skill dir
	result := content
	result = strings.ReplaceAll(result, "${CLAUDE_SESSION_ID}", sessionID)
	
	// Normalize skillDir for shell consistency (forward slashes)
	normalizedSkillDir := strings.ReplaceAll(skillDir, "\\", "/")
	result = strings.ReplaceAll(result, "${CLAUDE_SKILL_DIR}", normalizedSkillDir)

	// 2. Substitute arguments
	result = argRegex.ReplaceAllStringFunc(result, func(match string) string {
		name := argRegex.FindStringSubmatch(match)[1]
		if val, ok := args[name]; ok {
			return val
		}
		return match // Keep original if not found
	})

	return result
}

// SubstituteArguments is a legacy wrapper or specific for arguments
func SubstituteArguments(content string, args map[string]string) string {
	return argRegex.ReplaceAllStringFunc(content, func(match string) string {
		name := argRegex.FindStringSubmatch(match)[1]
		if val, ok := args[name]; ok {
			return val
		}
		return match
	})
}
