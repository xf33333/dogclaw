package core

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dogclaw/pkg/memory"
)

// Scanner walks the memory directory, reads frontmatter from each
// `.md` file, and returns a list of MemoryHeaders sorted by mtime
// ascending.
//
// It enforces the memory package limits (max files, depth, symlinks).
// The memoryDir path MUST exist and be accessible.
func Scanner(memoryDir string) ([]memory.MemoryHeader, error) {
	// Resolve to absolute
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

	// Collect candidate paths up to MaxMemoryFiles
	var fileInfos []fileEntry

	walkDir := abs
	// Limit traversal depth to 5 as per spec to avoid deep recursion
	maxDepth := 5
	relStart := walkDir

	err = filepath.Walk(walkDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		// Enforce depth
		rel, _ := filepath.Rel(relStart, path)
		depth := strings.Count(rel, string(filepath.Separator))
		if depth > maxDepth {
			return filepath.SkipDir
		}
		if info.IsDir() {
			if info.Mode()&os.ModeSymlink != 0 {
				return filepath.SkipDir // skip symlinked dirs
			}
			// Check for .git or .svn to skip VCS dirs
			base := filepath.Base(path)
			if base == ".git" || base == ".svn" {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil // skip non-md
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return nil // skip symlinked files
		}

		if len(fileInfos) >= memory.MaxMemoryFiles {
			return nil // cap reached
		}

		mtimeMs := info.ModTime().UnixMilli()
		fileInfos = append(fileInfos, fileEntry{Path: path, MtimeMs: mtimeMs})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking memory dir: %w", err)
	}

	// Sort by mtime ascending
	sort.Slice(fileInfos, func(i, j int) bool {
		if fileInfos[i].MtimeMs == fileInfos[j].MtimeMs {
			return fileInfos[i].Path < fileInfos[j].Path
		}
		return fileInfos[i].MtimeMs < fileInfos[j].MtimeMs
	})

	// Read frontmatter for each file
	results := make([]memory.MemoryHeader, 0, len(fileInfos))
	for _, fe := range fileInfos {
		h, err := readFrontmatter(fe.Path, fe.MtimeMs, abs)
		if err == nil {
			results = append(results, h)
		}
		// Silently skip files with read errors
	}

	return results, nil
}

type fileEntry struct {
	Path    string
	MtimeMs int64
}

// readFrontmatter opens a file, reads up to FrontmatterMaxLines looking
// for a YAML block between `---` markers, and extracts `description`
// and `type` fields. Returns a MemoryHeader.
func readFrontmatter(path string, mtimeMs int64, memoryDir string) (memory.MemoryHeader, error) {
	f, err := os.Open(path)
	if err != nil {
		return memory.MemoryHeader{}, err
	}
	defer f.Close()

	fm := parseFrontmatter(f)
	rel, _ := filepath.Rel(memoryDir, path)

	header := memory.MemoryHeader{
		Filename:    filepath.ToSlash(rel),
		FilePath:    path,
		MtimeMs:     mtimeMs,
		Description: fm.description,
		Type:        memory.ParseMemoryType(fm.typ),
	}
	return header, nil
}

type frontmatter struct {
	description string
	typ         string
	query       string
}

// parseFrontmatter reads from r looking for a YAML block delimited by
// `---`. It returns the first `description:` and `type:` values found
// within the block.
func parseFrontmatter(r io.Reader) frontmatter {
	var fm frontmatter
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)

	// Find opening ---
	foundOpen := false
	for i := 0; i < memory.FrontmatterMaxLines; i++ {
		if !scanner.Scan() {
			return fm
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			if !foundOpen {
				foundOpen = true
			} else {
				// Found closing ---
				return fm
			}
		} else if foundOpen {
			// Parse key: value
			if k, v, ok := parseKV(line); ok {
				switch strings.ToLower(k) {
				case "description":
					fm.description = v
				case "type":
					fm.typ = v
				case "query":
					fm.query = v
				}
			}
		}
	}
	return fm
}

// parseKV splits "key: value" or "key:value" returning trimmed parts.
func parseKV(line string) (key, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])

	// Strip surrounding quotes
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return key, value, true
}

// Ingest reads the full content of the files whose paths appear in
// headers, filtering for .md files only, and returns RelevantMemory
// entries sorted by mtime ascending.
//
// Files that fail to open are silently skipped. Content is returned
// without frontmatter.
func Ingest(headers []memory.MemoryHeader) ([]memory.RelevantMemory, error) {
	results := make([]memory.RelevantMemory, 0, len(headers))

	for _, h := range headers {
		if !strings.HasSuffix(strings.ToLower(h.Filename), ".md") {
			continue
		}

		content, err := ReadContentWithoutFrontmatter(h.FilePath)
		if err != nil {
			continue // skip unreadable
		}

		results = append(results, memory.RelevantMemory{
			Path:    h.FilePath,
			MtimeMs: h.MtimeMs,
			Content: content,
		})
	}

	return results, nil
}

// ReadContentWithoutFrontmatter reads the entire file, skipping the
// frontmatter block if present, and returns the remaining text.
func ReadContentWithoutFrontmatter(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 4096), 10_000*1024)

	// Skip frontmatter
	inFM := false
	fmClosed := false
	for {
		if !scanner.Scan() {
			// Entire file was frontmatter or empty
			return "", nil
		}
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			if !inFM {
				inFM = true
			} else {
				fmClosed = true
				break
			}
		} else if inFM {
			// inside frontmatter, keep scanning
			continue
		} else {
			// no frontmatter found — content starts immediately
			// rewind: we already consumed the first line
			var sb strings.Builder
			sb.WriteString(line)
			sb.WriteString("\n")
			for scanner.Scan() {
				sb.WriteString(scanner.Text())
				sb.WriteString("\n")
			}
			return strings.TrimSuffix(sb.String(), "\n"), scanner.Err()
		}
	}

	if !fmClosed {
		// Frontmatter started but never closed — treat whole file as FM
		return "", nil
	}

	// Read rest of content
	var sb strings.Builder
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
		sb.WriteString("\n")
	}
	return strings.TrimSuffix(sb.String(), "\n"), scanner.Err()
}

// FormatMemories produces a formatted markdown string of RelevantMemory
// entries with timestamp, path, type, and content sections.
//
// Output format:
//
//	## timestamp: 1748169875000 | path: memory/example.md | type: user
//
//	<content>
//
// If content is empty, the block shows "(memory file is empty)".
func FormatMemories(memories []memory.RelevantMemory) string {
	var sb strings.Builder
	for _, m := range memories {
		// Determine type from path (best effort)
		typ := memory.MemoryTypeUser // fallback
		lower := strings.ToLower(m.Path)
		for _, mt := range memory.ValidMemoryTypes() {
			if strings.Contains(lower, string(mt)) {
				typ = mt
				break
			}
		}

		sb.WriteString(fmt.Sprintf("## timestamp: %d | path: %s | type: %s\n\n",
			m.MtimeMs, m.Path, typ))

		content := strings.TrimSpace(m.Content)
		if content == "" {
			sb.WriteString("(memory file is empty)\n\n")
		} else {
			sb.WriteString(content)
			sb.WriteString("\n\n")
		}
	}
	return sb.String()
}

// TimeInfo holds formatted current date/time and timezone info.
type TimeInfo struct {
	ISO8601  string
	Timezone string
}

// GetNowInfo returns the current date, time, and timezone formatted
// per the spec.
func GetNowInfo() TimeInfo {
	now := time.Now()
	return TimeInfo{
		ISO8601:  now.Format("2006-01-02T15:04:05-07:00"),
		Timezone: now.Location().String(),
	}
}
