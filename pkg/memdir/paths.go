package memdir

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MemoryAge represents the age category of a memory
type MemoryAge string

const (
	AgeFresh    MemoryAge = "fresh"    // < 1 day
	AgeRecent   MemoryAge = "recent"   // < 1 week
	AgeModerate MemoryAge = "moderate" // < 1 month
	AgeStale    MemoryAge = "stale"    // > 1 month
)

// GetMemoryAge categorizes how old a memory is
func GetMemoryAge(modTime time.Time) MemoryAge {
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
	return GetMemoryAge(modTime) == AgeStale
}

// MemoryScanResult represents the result of scanning a memory directory
type MemoryScanResult struct {
	Path         string
	Type         MemoryType
	Age          MemoryAge
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
		parsedEntries, _ := parseMemoryFile(string(content))

		results = append(results, MemoryScanResult{
			Path:         path,
			Type:         memType,
			Age:          GetMemoryAge(info.ModTime()),
			EntryCount:   len(parsedEntries),
			SizeBytes:    info.Size(),
			LastModified: info.ModTime(),
		})
	}

	return results, nil
}

// detectMemoryType tries to detect the memory type from content
func detectMemoryType(content string) MemoryType {
	lower := strings.ToLower(content)
	for _, mt := range ValidMemoryTypes {
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
		if result.Age == AgeStale {
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

// GetAutoMemPath returns the auto-memory directory path.
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

// GetAutoMemDailyLogPath returns the daily log file path for the given date.
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
