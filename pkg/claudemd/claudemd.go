package claudemd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// MemoryType represents the type of memory file
type MemoryType string

const (
	Managed MemoryType = "Managed"
	User    MemoryType = "User"
	Project MemoryType = "Project"
	Local   MemoryType = "Local"
	AutoMem MemoryType = "AutoMem"
)

// MemoryFileInfo holds information about a loaded memory file
type MemoryFileInfo struct {
	Path    string
	Type    MemoryType
	Content string
	Parent  string // Path of the file that included this one via @include
}

// MAX_MEMORY_CHARACTER_COUNT is the recommended max character count for a memory file
const MAX_MEMORY_CHARACTER_COUNT = 40000

// MEMORY_INSTRUCTION_PROMPT is prepended to all memory content
const MEMORY_INSTRUCTION_PROMPT = "Codebase and user instructions are shown below. Be sure to adhere to these instructions. IMPORTANT: These instructions OVERRIDE any default behavior and you MUST follow them exactly as written."

// MaxIncludeDepth prevents infinite @include loops
const MaxIncludeDepth = 5

// GetMemoryFiles discovers and loads all memory files from:
// 1. Managed memory (/etc/dowclaw/AGENT.md) - Global for all users
// 2. User memory (~/.dogclaw/AGENT.md) - Private global instructions
// 3. Project memory (AGENT.md, .dogclaw/AGENT.md, .dogclaw/rules/*.md) - Checked into codebase
// 4. Local memory (AGENT.local.md) - Private project-specific instructions
//
// Files loaded in reverse priority order (latest = highest priority)
func GetMemoryFiles(cwd string) ([]MemoryFileInfo, error) {
	var result []MemoryFileInfo
	processedPaths := make(map[string]bool)

	// 1. Managed memory
	managedPath := "/etc/dowclaw/AGENT.md"
	files, err := processMemoryFile(managedPath, Managed, processedPaths, 0, "")
	if err == nil {
		result = append(result, files...)
	}

	// 2. User memory (~/.dogclaw/AGENT.md)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		userPath := filepath.Join(homeDir, ".dogclaw", "AGENT.md")
		files, err = processMemoryFile(userPath, User, processedPaths, 0, "")
		if err == nil {
			result = append(result, files...)
		}

		// User rules (~/.dogclaw/rules/*.md)
		userRulesDir := filepath.Join(homeDir, ".dogclaw", "rules")
		ruleFiles, err := processMdRules(userRulesDir, User, processedPaths)
		if err == nil {
			result = append(result, ruleFiles...)
		}
	}

	// 3 & 4. Project and Local files - walk from CWD up to root
	dirs := []string{}
	currentDir, err := filepath.Abs(cwd)
	if err != nil {
		return result, err
	}

	for {
		dirs = append(dirs, currentDir)
		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			break // reached root
		}
		currentDir = parent
	}

	// Process from root downward (root has lowest priority, loaded first)
	for i := len(dirs) - 1; i >= 0; i-- {
		dir := dirs[i]

		// Project: AGENT.md
		projectPath := filepath.Join(dir, "AGENT.md")
		files, err = processMemoryFile(projectPath, Project, processedPaths, 0, "")
		if err == nil {
			result = append(result, files...)
		}

		// Project: .dogclaw/AGENT.md
		dogclawPath := filepath.Join(dir, ".dogclaw", "AGENT.md")
		files, err = processMemoryFile(dogclawPath, Project, processedPaths, 0, "")
		if err == nil {
			result = append(result, files...)
		}

		// Project: .dogclaw/rules/*.md
		rulesDir := filepath.Join(dir, ".dogclaw", "rules")
		ruleFiles, err := processMdRules(rulesDir, Project, processedPaths)
		if err == nil {
			result = append(result, ruleFiles...)
		}

		// Local: AGENT.local.md
		localPath := filepath.Join(dir, "AGENT.local.md")
		files, err = processMemoryFile(localPath, Local, processedPaths, 0, "")
		if err == nil {
			result = append(result, files...)
		}
	}

	return result, nil
}

// processMemoryFile reads a memory file and recursively processes @include references
func processMemoryFile(filePath string, memType MemoryType, processedPaths map[string]bool, depth int, parent string) ([]MemoryFileInfo, error) {
	// Normalize path for dedup
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}

	// Skip if already processed or max depth exceeded
	if processedPaths[absPath] || depth >= MaxIncludeDepth {
		return nil, nil
	}

	processedPaths[absPath] = true

	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		// ENOENT or EISDIR are expected, silently ignore
		if os.IsNotExist(err) || isDirectoryError(err) {
			return nil, nil
		}
		return nil, err
	}

	contentStr := strings.TrimSpace(string(content))
	if contentStr == "" {
		return nil, nil
	}

	var result []MemoryFileInfo

	// Add the main file
	result = append(result, MemoryFileInfo{
		Path:    absPath,
		Type:    memType,
		Content: contentStr,
		Parent:  parent,
	})

	// Extract and process @include references
	includePaths := extractIncludePaths(contentStr, filepath.Dir(absPath))
	for _, includePath := range includePaths {
		includedFiles, err := processMemoryFile(includePath, memType, processedPaths, depth+1, absPath)
		if err == nil {
			result = append(result, includedFiles...)
		}
	}

	return result, nil
}

// processMdRules processes all .md files in a rules directory
func processMdRules(rulesDir string, memType MemoryType, processedPaths map[string]bool) ([]MemoryFileInfo, error) {
	var result []MemoryFileInfo

	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Recurse into subdirectories
			subDir := filepath.Join(rulesDir, entry.Name())
			subFiles, err := processMdRules(subDir, memType, processedPaths)
			if err == nil {
				result = append(result, subFiles...)
			}
		} else if strings.HasSuffix(entry.Name(), ".md") {
			files, err := processMemoryFile(filepath.Join(rulesDir, entry.Name()), memType, processedPaths, 0, "")
			if err == nil {
				result = append(result, files...)
			}
		}
	}

	return result, nil
}

// extractIncludePaths finds @path references in content and resolves them
func extractIncludePaths(content string, basePath string) []string {
	var paths []string
	// Match @path, @./path, @~/path, @/path
	// Skip @paths inside code blocks
	includeRegex := regexp.MustCompile(`(?:^|\s)@((?:[^\s\\]|\\ )+)`)

	lines := strings.Split(content, "\n")
	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}

		if inCodeBlock {
			continue
		}

		matches := includeRegex.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			path := match[1]

			// Strip fragment identifiers
			if idx := strings.Index(path, "#"); idx != -1 {
				path = path[:idx]
			}
			if path == "" {
				continue
			}

			// Unescape spaces
			path = strings.ReplaceAll(path, "\\ ", " ")

			// Validate path
			isValid := strings.HasPrefix(path, "./") ||
				strings.HasPrefix(path, "~/") ||
				(strings.HasPrefix(path, "/") && len(path) > 1) ||
				(!strings.HasPrefix(path, "@") && len(path) > 0 && isAlphanumericStart(path))

			if isValid {
				resolved := expandPath(path, basePath)
				if resolved != "" {
					paths = append(paths, resolved)
				}
			}
		}
	}

	return paths
}

// expandPath resolves @path, @./path, @~/path, @/path to absolute paths
func expandPath(path string, basePath string) string {
	switch {
	case strings.HasPrefix(path, "~/"):
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(homeDir, path[2:])
	case strings.HasPrefix(path, "/"):
		return filepath.Clean(path)
	default:
		// Relative path
		absPath, err := filepath.Abs(filepath.Join(basePath, path))
		if err != nil {
			return ""
		}
		return absPath
	}
}

// isAlphanumericStart checks if path starts with an alphanumeric character
func isAlphanumericStart(s string) bool {
	if len(s) == 0 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
}

// IsMemoryFilePath checks if a file path is a memory file
func IsMemoryFilePath(filePath string) bool {
	name := filepath.Base(filePath)
	return name == "AGENT.md" || name == "AGENT.local.md" ||
		strings.Contains(filePath, string(filepath.Separator)+".dogclaw"+string(filepath.Separator)+"rules"+string(filepath.Separator))
}

// ProcessMemoryFile is exported for use by the file read tool
func ProcessMemoryFile(filePath string, memType MemoryType, processedPaths map[string]bool, depth int, parent string) ([]MemoryFileInfo, error) {
	return processMemoryFile(filePath, memType, processedPaths, depth, parent)
}

// isDirectoryError checks if an error is a directory error
func isDirectoryError(err error) bool {
	return strings.Contains(err.Error(), "is a directory")
}

// BuildAgentMdContext formats memory files into context string for system prompt
func BuildAgentMdContext(files []MemoryFileInfo) string {
	if len(files) == 0 {
		return ""
	}

	var memories []string
	for _, file := range files {
		if file.Content == "" {
			continue
		}

		description := ""
		switch file.Type {
		case Project:
			description = " (project instructions, checked into the codebase)"
		case Local:
			description = " (user's private project instructions, not checked in)"
		case AutoMem:
			description = " (user's auto-memory, persists across conversations)"
		default:
			description = " (user's private global instructions for all projects)"
		}

		memories = append(memories, fmt.Sprintf("Contents of %s%s:\n\n%s", file.Path, description, file.Content))
	}

	if len(memories) == 0 {
		return ""
	}

	return fmt.Sprintf("%s\n\n%s", MEMORY_INSTRUCTION_PROMPT, strings.Join(memories, "\n\n"))
}
