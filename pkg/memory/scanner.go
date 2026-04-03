package memory

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// scanResult holds the result of scanning a single memory file.
type scanResult struct {
	header MemoryHeader
	err    error
}

// ScanMemoryFiles scans a memory directory for .md files, reads their
// frontmatter, and returns a header list sorted newest-first (capped at
// MaxMemoryFiles). Excludes MEMORY.md from the list.
//
// Single-pass: os.Stat is called internally, so we stat-then-sort rather
// than stat-sort-read. For the common case (N ≤ 200) this halves syscalls
// vs a separate stat round.
func ScanMemoryFiles(memoryDir string) ([]MemoryHeader, error) {
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read memory directory: %w", err)
	}

	var mdFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		if name == EntrypointName {
			continue
		}
		mdFiles = append(mdFiles, name)
	}

	if len(mdFiles) == 0 {
		return nil, nil
	}

	results := make(chan scanResult, len(mdFiles))
	for _, name := range mdFiles {
		go func(fname string) {
			header, err := scanSingleFile(memoryDir, fname)
			results <- scanResult{header: header, err: err}
		}(name)
	}

	var headers []MemoryHeader
	for i := 0; i < len(mdFiles); i++ {
		r := <-results
		if r.err == nil {
			headers = append(headers, r.header)
		}
	}

	// Sort newest-first
	sort.Slice(headers, func(a, b int) bool {
		return headers[a].MtimeMs > headers[b].MtimeMs
	})

	// Cap at MaxMemoryFiles
	if len(headers) > MaxMemoryFiles {
		headers = headers[:MaxMemoryFiles]
	}

	return headers, nil
}

// scanSingleFile reads and parses a single memory file's frontmatter.
func scanSingleFile(memoryDir, filename string) (MemoryHeader, error) {
	filePath := filepath.Join(memoryDir, filename)

	info, err := os.Stat(filePath)
	if err != nil {
		return MemoryHeader{}, err
	}

	mtimeMs := info.ModTime().UnixMilli()

	// Read first N lines for frontmatter
	description, memType := readFrontmatter(filePath)

	return MemoryHeader{
		Filename:    filename,
		FilePath:    filePath,
		MtimeMs:     mtimeMs,
		Description: description,
		Type:        memType,
	}, nil
}

// readFrontmatter reads the frontmatter from a file and extracts
// description and type fields.
func readFrontmatter(filePath string) (description string, memType MemoryType) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineCount := 0
	inFrontmatter := false
	frontmatterEnded := false

	for scanner.Scan() {
		if lineCount >= FrontmatterMaxLines {
			break
		}
		line := scanner.Text()
		lineCount++

		if !inFrontmatter {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = true
			}
			continue
		}

		if frontmatterEnded {
			break
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			frontmatterEnded = true
			continue
		}

		// Parse key: value
		if idx := strings.Index(trimmed, ":"); idx > 0 {
			key := strings.TrimSpace(trimmed[:idx])
			value := strings.TrimSpace(trimmed[idx+1:])

			// Strip quotes
			if len(value) >= 2 {
				if (value[0] == '"' && value[len(value)-1] == '"') ||
					(value[0] == '\'' && value[len(value)-1] == '\'') {
					value = value[1 : len(value)-1]
				}
			}

			switch key {
			case "description":
				description = value
			case "type":
				memType = ParseMemoryType(value)
			}
		}
	}

	return description, memType
}

// FormatMemoryManifest formats memory headers as a text manifest:
// one line per file with [type] filename (timestamp): description.
func FormatMemoryManifest(memories []MemoryHeader) string {
	var lines []string
	for _, m := range memories {
		tag := ""
		if m.Type != "" {
			tag = fmt.Sprintf("[%s] ", m.Type)
		}
		ts := time.UnixMilli(m.MtimeMs).Format(time.RFC3339)
		line := fmt.Sprintf("- %s%s (%s)", tag, m.Filename, ts)
		if m.Description != "" {
			line = fmt.Sprintf("- %s%s (%s): %s", tag, m.Filename, ts, m.Description)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// ScanMemoryDir walks the memory directory with depth and symlink guards,
// returning MemoryHeaders sorted by mtime newest-first.
// This is the comprehensive scanner used by the core package.
func ScanMemoryDir(memoryDir string) ([]MemoryHeader, error) {
	abs, err := filepath.Abs(memoryDir)
	if err != nil {
		return nil, fmt.Errorf("resolving memory dir: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat memory dir: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("memory path is not a directory: %s", abs)
	}

	type fileEntry struct {
		Path    string
		MtimeMs int64
	}

	var fileInfos []fileEntry
	maxDepth := 5

	err = filepath.Walk(abs, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		rel, _ := filepath.Rel(abs, path)
		depth := strings.Count(rel, string(filepath.Separator))
		if depth > maxDepth {
			return filepath.SkipDir
		}
		if info.IsDir() {
			if info.Mode()&os.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			base := filepath.Base(path)
			if base == ".git" || base == ".svn" {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if strings.EqualFold(info.Name(), EntrypointName) {
			return nil
		}

		if len(fileInfos) >= MaxMemoryFiles {
			return nil
		}

		fileInfos = append(fileInfos, fileEntry{Path: path, MtimeMs: info.ModTime().UnixMilli()})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking memory dir: %w", err)
	}

	// Sort by mtime descending (newest first)
	sort.Slice(fileInfos, func(i, j int) bool {
		if fileInfos[i].MtimeMs == fileInfos[j].MtimeMs {
			return fileInfos[i].Path < fileInfos[j].Path
		}
		return fileInfos[i].MtimeMs > fileInfos[j].MtimeMs
	})

	// Read frontmatter for each file
	results := make([]MemoryHeader, 0, len(fileInfos))
	for _, fe := range fileInfos {
		h, err := readFrontmatterFromPath(fe.Path, fe.MtimeMs, abs)
		if err == nil {
			results = append(results, h)
		}
	}

	return results, nil
}

// readFrontmatterFromPath opens a file, reads up to FrontmatterMaxLines looking
// for a YAML block between `---` markers, and extracts `description`
// and `type` fields. Returns a MemoryHeader.
func readFrontmatterFromPath(path string, mtimeMs int64, memoryDir string) (MemoryHeader, error) {
	f, err := os.Open(path)
	if err != nil {
		return MemoryHeader{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFM := false
	var description string
	var memType MemoryType

	lineCount := 0
	for scanner.Scan() && lineCount < FrontmatterMaxLines {
		line := strings.TrimSpace(scanner.Text())
		lineCount++

		if line == "---" {
			if !inFM {
				inFM = true
			} else {
				break // closing ---
			}
		} else if inFM {
			if idx := strings.Index(line, ":"); idx > 0 {
				key := strings.TrimSpace(line[:idx])
				value := strings.TrimSpace(line[idx+1:])
				if len(value) >= 2 &&
					((value[0] == '"' && value[len(value)-1] == '"') ||
						(value[0] == '\'' && value[len(value)-1] == '\'')) {
					value = value[1 : len(value)-1]
				}
				switch strings.ToLower(key) {
				case "description":
					description = value
				case "type":
					memType = ParseMemoryType(value)
				}
			}
		}
	}

	rel, _ := filepath.Rel(memoryDir, path)
	return MemoryHeader{
		Filename:    filepath.ToSlash(rel),
		FilePath:    path,
		MtimeMs:     mtimeMs,
		Description: description,
		Type:        memType,
	}, nil
}
