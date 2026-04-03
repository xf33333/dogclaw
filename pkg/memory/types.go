// Package memory provides a file-based memory system for persisting
// context across conversations.
//
// Memories are stored as markdown files with frontmatter metadata
// in a dedicated memory directory. The system supports four types
// of memory: user, feedback, project, and reference.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MemoryType represents the category of a memory.
// Memories are constrained to these four types capturing context
// NOT derivable from the current project state.
type MemoryType string

const (
	// MemoryTypeUser contains information about the user's role,
	// goals, responsibilities, and knowledge.
	MemoryTypeUser MemoryType = "user"

	// MemoryTypeFeedback is guidance the user has given about how
	// to approach work — what to avoid and what to keep doing.
	MemoryTypeFeedback MemoryType = "feedback"

	// MemoryTypeProject is information about ongoing work, goals,
	// initiatives, bugs, or incidents within the project.
	MemoryTypeProject MemoryType = "project"

	// MemoryTypeReference stores pointers to where information can
	// be found in external systems.
	MemoryTypeReference MemoryType = "reference"
)

// ValidMemoryTypes returns all valid memory types.
func ValidMemoryTypes() []MemoryType {
	return []MemoryType{
		MemoryTypeUser,
		MemoryTypeFeedback,
		MemoryTypeProject,
		MemoryTypeReference,
	}
}

// ParseMemoryType parses a raw string into a MemoryType.
// Invalid or missing values return empty — legacy files without a
// `type:` field keep working, files with unknown types degrade gracefully.
func ParseMemoryType(raw string) MemoryType {
	for _, t := range ValidMemoryTypes() {
		if strings.EqualFold(string(t), strings.TrimSpace(raw)) {
			return t
		}
	}
	return ""
}

const (
	// MemoryDirName is the name of the memory directory.
	MemoryDirName = "memory"

	// EntrypointName is the index file name.
	EntrypointName = "MEMORY.md"

	// MaxEntrypointLines is the line cap for the index file.
	MaxEntrypointLines = 200

	// MaxEntrypointBytes is the byte cap for the index file.
	MaxEntrypointBytes = 25_000

	// MaxMemoryFiles is the cap on scanned memory files.
	MaxMemoryFiles = 200

	// FrontmatterMaxLines is the max lines to read for frontmatter.
	FrontmatterMaxLines = 30

	// MaxMemoryChars is the recommended max chars for a memory file.
	MaxMemoryChars = 40_000
)

// MemoryHeader holds metadata about a memory file (frontmatter + stat).
type MemoryHeader struct {
	// Filename is the relative path from the memory directory.
	Filename string
	// FilePath is the absolute path to the memory file.
	FilePath string
	// MtimeMs is the file modification time in milliseconds.
	MtimeMs int64
	// Description is the frontmatter description field.
	Description string
	// Type is the parsed memory type (empty if invalid/missing).
	Type MemoryType
}

// RelevantMemory is a memory selected as relevant to a query.
type RelevantMemory struct {
	// Path is the absolute file path.
	Path string
	// MtimeMs is the file modification time in milliseconds.
	MtimeMs int64
	// Content is the full file content (without frontmatter).
	Content string
}

// MemoryAgeCategory represents the age category of a memory
type MemoryAgeCategory string

const (
	AgeFresh    MemoryAgeCategory = "fresh"    // < 1 day
	AgeRecent   MemoryAgeCategory = "recent"   // < 1 week
	AgeModerate MemoryAgeCategory = "moderate" // < 1 month
	AgeStale    MemoryAgeCategory = "stale"    // > 1 month
)

// GetMemoryAgeCategory categorizes how old a memory is
func GetMemoryAgeCategory(modTime time.Time) MemoryAgeCategory {
	age := time.Since(modTime)

	switch {
	case age < 24*time.Hour:
		return AgeFresh
	case age < 7*24*time.Hour:
		return AgeRecent
	case age < 30*24*time.Hour:
		return AgeModerate
	default:
		return AgeStale
	}
}

// ShouldCompactMemory returns true if memory should be compacted based on age
func ShouldCompactMemory(modTime time.Time) bool {
	return GetMemoryAgeCategory(modTime) == AgeStale
}

// MemoryEntry represents a single memory entry
type MemoryEntry struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Type        MemoryType `json:"type"`
	Content     string     `json:"content"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	Scope       string     `json:"scope,omitempty"`
}

// Format formats a memory entry as markdown
func (e *MemoryEntry) Format() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("---\nname: %s\ndescription: %s\ntype: %s\n---\n\n",
		e.Name, e.Description, e.Type))
	sb.WriteString(e.Content)
	return sb.String()
}

// MemoryStore manages memory entries in a directory
type MemoryStore struct {
	Dir string
}

// NewMemoryStore creates a new memory store
func NewMemoryStore(dir string) *MemoryStore {
	return &MemoryStore{Dir: dir}
}

// EnsureDir ensures the memory directory exists
func (ms *MemoryStore) EnsureDir() error {
	return os.MkdirAll(ms.Dir, 0755)
}

// ReadMemory reads the MEMORY.md file and parses entries
func (ms *MemoryStore) ReadMemory() ([]MemoryEntry, error) {
	path := filepath.Join(ms.Dir, EntrypointName)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read memory: %w", err)
	}
	return parseMemoryFile(string(content))
}

// WriteMemory writes entries to MEMORY.md
func (ms *MemoryStore) WriteMemory(entries []MemoryEntry) error {
	if err := ms.EnsureDir(); err != nil {
		return err
	}

	path := filepath.Join(ms.Dir, EntrypointName)
	var sb strings.Builder

	sb.WriteString(`# Memory Index

Each memory is stored in its own file. This file is an index of pointers.
Format: - [title](file.md) — one-line summary

`)

	for _, entry := range entries {
		sb.WriteString(fmt.Sprintf("- [%s](%s.md) — %s\n",
			entry.Name, sanitizeMemoryFilename(entry.Name), entry.Description))
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// AddMemory adds a new memory entry
func (ms *MemoryStore) AddMemory(entry MemoryEntry) error {
	existing, err := ms.ReadMemory()
	if err != nil {
		return err
	}

	entry.CreatedAt = time.Now()
	entry.UpdatedAt = time.Now()
	existing = append(existing, entry)

	return ms.WriteMemory(existing)
}

// Query searches for memories by type and/or keyword
func (ms *MemoryStore) Query(memType MemoryType, keyword string) ([]MemoryEntry, error) {
	entries, err := ms.ReadMemory()
	if err != nil {
		return nil, err
	}

	var results []MemoryEntry
	for _, entry := range entries {
		if memType != "" && entry.Type != memType {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(entry.Content), strings.ToLower(keyword)) &&
			!strings.Contains(strings.ToLower(entry.Description), strings.ToLower(keyword)) {
			continue
		}
		results = append(results, entry)
	}

	return results, nil
}

// Clear removes the memory index file (not individual memory files)
func (ms *MemoryStore) Clear() error {
	path := filepath.Join(ms.Dir, EntrypointName)
	return os.Remove(path)
}

// parseMemoryFile parses a MEMORY.md file into entries
func parseMemoryFile(content string) ([]MemoryEntry, error) {
	var entries []MemoryEntry
	parts := strings.Split(content, "---")
	for i := 1; i < len(parts)-1; i += 2 {
		header := parts[i]
		body := ""
		if i+1 < len(parts) {
			body = strings.TrimSpace(parts[i+1])
		}

		entry := MemoryEntry{}
		for _, line := range strings.Split(header, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "name:") {
				entry.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			} else if strings.HasPrefix(line, "description:") {
				entry.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			} else if strings.HasPrefix(line, "type:") {
				entry.Type = ParseMemoryType(strings.TrimSpace(strings.TrimPrefix(line, "type:")))
			}
		}
		entry.Content = body
		entries = append(entries, entry)
	}

	return entries, nil
}

// MemoryScanResult represents the result of scanning a memory directory
type MemoryScanResult struct {
	Path         string
	Type         MemoryType
	AgeCategory  MemoryAgeCategory
	EntryCount   int
	SizeBytes    int64
	LastModified time.Time
}

// ScanMemoryDirectory scans a memory directory and returns metadata
func ScanMemoryDirectory(dir string) ([]MemoryScanResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var results []MemoryScanResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		memType := detectMemoryType(string(content))

		results = append(results, MemoryScanResult{
			Path:         path,
			Type:         memType,
			AgeCategory:  GetMemoryAgeCategory(info.ModTime()),
			EntryCount:   1,
			SizeBytes:    info.Size(),
			LastModified: info.ModTime(),
		})
	}

	return results, nil
}

// detectMemoryType tries to detect the memory type from content
func detectMemoryType(content string) MemoryType {
	lower := strings.ToLower(content)
	for _, mt := range ValidMemoryTypes() {
		if strings.Contains(lower, "type: "+string(mt)) {
			return mt
		}
	}
	return ""
}

// FindRelevantMemories searches for memories relevant to a query
func FindRelevantMemories(dir string, query string) ([]MemoryEntry, error) {
	results, err := ScanMemoryDirectory(dir)
	if err != nil {
		return nil, err
	}

	var relevant []MemoryEntry
	queryLower := strings.ToLower(query)

	for _, result := range results {
		if result.AgeCategory == AgeStale {
			continue // Skip stale memories
		}

		store := NewMemoryStore(filepath.Dir(result.Path))
		entries, err := store.ReadMemory()
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if isRelevant(entry, queryLower) {
				relevant = append(relevant, entry)
			}
		}
	}

	return relevant, nil
}

// isRelevant checks if a memory entry is relevant to a query
func isRelevant(entry MemoryEntry, queryLower string) bool {
	nameLower := strings.ToLower(entry.Name)
	descLower := strings.ToLower(entry.Description)
	contentLower := strings.ToLower(entry.Content)

	keywords := strings.Fields(queryLower)
	for _, kw := range keywords {
		if len(kw) < 3 {
			continue
		}
		if strings.Contains(nameLower, kw) ||
			strings.Contains(descLower, kw) ||
			strings.Contains(contentLower, kw) {
			return true
		}
	}

	return false
}

// MemoryTypeDescriptions returns the description of each memory type
func MemoryTypeDescriptions() map[MemoryType]string {
	return map[MemoryType]string{
		MemoryTypeUser:      "Information about the user's role, goals, responsibilities, and knowledge.",
		MemoryTypeFeedback:  "Guidance the user has given about how to approach work.",
		MemoryTypeProject:   "Information about ongoing work, goals, initiatives, bugs, or incidents.",
		MemoryTypeReference: "Pointers to where information can be found in external systems.",
	}
}

// WhatNotToSave returns guidelines for what NOT to save in memory
func WhatNotToSaveText() []string {
	return []string{
		"Code patterns, conventions, architecture, file paths, or project structure.",
		"Git history, recent changes, or who-changed-what.",
		"Debugging solutions or fix recipes.",
		"Anything already documented in AGENT.md files.",
		"Ephemeral task details: in-progress work, temporary state.",
	}
}

// MemoryDriftCaveat returns the warning about stale memories
func MemoryDriftCaveat() string {
	return "Memory records can become stale over time. Verify that the memory is still correct and up-to-date by reading the current state of the files or resources. If a recalled memory conflicts with current information, trust what you observe now."
}

// sanitizeMemoryFilename creates a safe filename from a name.
func sanitizeMemoryFilename(name string) string {
	replacer := strings.NewReplacer(
		" ", "-",
		"/", "_",
		"\\", "_",
		"?", "_",
		"*", "_",
		":", "-",
		"<", "_",
		">", "_",
		"|", "_",
	)
	name = replacer.Replace(name)
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	name = strings.Trim(name, "-_")
	if name == "" {
		name = "unnamed"
	}
	return strings.ToLower(name)
}

// Path resolution functions for automatic memory directory.
// These allow memory to be project-specific without manual configuration.

// GetMemoryBaseDir returns the base directory for persistent memory storage.
// Resolution order:
//  1. CLAUDE_CODE_REMOTE_MEMORY_DIR env var
//  2. ~/.dogclaw (default config home)
func GetMemoryBaseDir() string {
	if dir := os.Getenv("CLAUDE_CODE_REMOTE_MEMORY_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".dogclaw")
}

const (
	autoMemDirname    = "memory"
	autoMemEntrypoint = "MEMORY.md"
)

// GetAutoMemPath returns the auto-memory directory path for the current project.
// The path is derived from the CWD, sanitized into a directory name.
var (
	autoMemPathOnce sync.Once
	autoMemPathVal  string
)

func GetAutoMemPath() string {
	autoMemPathOnce.Do(func() {
		if override := os.Getenv("CLAUDE_COWORK_MEMORY_PATH_OVERRIDE"); override != "" {
			autoMemPathVal = filepath.Clean(override) + string(filepath.Separator)
			return
		}

		projectsDir := filepath.Join(GetMemoryBaseDir(), "projects")
		cwd, _ := os.Getwd()
		sanitized := sanitizePath(cwd)
		autoMemPathVal = filepath.Join(projectsDir, sanitized, autoMemDirname) + string(filepath.Separator)
	})
	return autoMemPathVal
}

// ResetAutoMemPath resets the cached auto-memory path (useful for tests).
func ResetAutoMemPath() {
	autoMemPathOnce = sync.Once{}
	autoMemPathVal = ""
}

// GetAutoMemDailyLogPath returns the dailylog file path for the given date.
func GetAutoMemDailyLogPath(date time.Time) string {
	if date.IsZero() {
		date = time.Now()
	}
	yyyy := date.Format("2006")
	mm := date.Format("01")
	dd := date.Format("02")
	return filepath.Join(GetAutoMemPath(), "logs", yyyy, mm, yyyy+"-"+mm+"-"+dd+".md")
}

// GetAutoMemEntrypoint returns the MEMORY.md path inside the auto-memory dir.
func GetAutoMemEntrypoint() string {
	return filepath.Join(GetAutoMemPath(), autoMemEntrypoint)
}

// IsAutoMemPath checks if an absolute path is within the auto-memory directory.
func IsAutoMemPath(absolutePath string) bool {
	normalized := filepath.Clean(absolutePath)
	autoPath := GetAutoMemPath()
	return strings.HasPrefix(normalized, autoPath)
}

// HasAutoMemPathOverride checks if CLAUDE_COWORK_MEMORY_PATH_OVERRIDE is set.
func HasAutoMemPathOverride() bool {
	return os.Getenv("CLAUDE_COWORK_MEMORY_PATH_OVERRIDE") != ""
}

// IsAutoMemoryEnabled checks if auto-memory features are enabled.
func IsAutoMemoryEnabled() bool {
	envVal := os.Getenv("CLAUDE_CODE_DISABLE_AUTO_MEMORY")
	if envVal == "1" || envVal == "true" {
		return false
	}
	if os.Getenv("CLAUDE_CODE_SIMPLE") == "1" || os.Getenv("CLAUDE_CODE_SIMPLE") == "true" {
		return false
	}
	return true
}

// sanitizePath sanitizes a path for use as a directory name.
func sanitizePath(path string) string {
	result := strings.ReplaceAll(path, string(filepath.Separator), "-")
	result = strings.ReplaceAll(result, "/", "-")
	result = strings.ReplaceAll(result, "\\", "-")
	result = strings.Trim(result, "-")
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	if result == "" {
		result = "root"
	}
	return result
}

// EnsureMemoryDir ensures a memory directory exists (idempotent)
func EnsureMemoryDir(memoryDir string) error {
	return os.MkdirAll(memoryDir, 0755)
}

// DirExistsGuidance returns the guidance text for existing directories
func DirExistsGuidance() string {
	return "This directory already exists — write to it directly with the Write tool (do not run mkdir or check for its existence)."
}

// CompactRequest represents a request to compact memories.
type CompactRequest struct {
	// Directory to compact memories in
	Directory string
	// MaxFiles is the target max number of memory files after compaction
	MaxFiles int
	// Force compacts even if not stale
	Force bool
}

// CompactResult holds the result of a memory compaction operation.
type CompactResult struct {
	OriginalCount int
	NewCount      int
	Compacted     []string
	Skipped       []string
	Error         string
}
