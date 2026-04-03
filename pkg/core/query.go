package core

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dogclaw/pkg/memory"
)

// QueryEntry represents a single entry in the query results.
type QueryEntry struct {
	ID       string
	Semantic []memory.RelevantMemory
}

// QueryResult holds the output of Query: a list of QueryEntries
// sorted ascending by MtimeMs, and a Formatted string ready for
// injection into the system prompt.
type QueryResult struct {
	Entries   []QueryEntry
	Formatted string
}

// Query loads the entrypoint, parses the `# Query:` list from the
// YAML frontmatter, and for each entry:
//
//  1. Calls Scanner on any `path:` value in that entry's frontmatter
//     to get MemoryHeaders.
//  2. Calls Ingest() to load file contents.
//  3. Sorts memories by mtime ascending and formats them.
//
// If no `path:` is present in an entry, all `.md` files under the
// memory directory for that entry are scanned.
//
// Returns QueryResult sorted by earliest memory first.
func Query(memoryDir string) (QueryResult, error) {
	abs, err := filepath.Abs(memoryDir)
	if err != nil {
		return QueryResult{}, fmt.Errorf("resolving memory dir: %w", err)
	}

	entryPath := filepath.Join(abs, memory.EntrypointName)
	entries, err := parseQueryEntries(entryPath)
	if err != nil {
		return QueryResult{}, fmt.Errorf("parsing query entries: %w", err)
	}

	var queryEntries []QueryEntry

	for _, e := range entries {
		var allMemories []memory.RelevantMemory

		if len(e.Paths) == 0 {
			// No explicit path → scan the whole memory directory
			headers, err := Scanner(abs)
			if err != nil {
				continue
			}
			memories, err := Ingest(headers)
			if err != nil {
				continue
			}
			allMemories = memories
		} else {
			// Scan each specified path
			for _, p := range e.Paths {
				targetPath := p
				if !filepath.IsAbs(targetPath) {
					targetPath = filepath.Join(abs, targetPath)
				}

				info, err := os.Stat(targetPath)
				if err != nil {
					continue // skip non-existent
				}

				var headers []memory.MemoryHeader
				if info.IsDir() {
					h, err := Scanner(targetPath)
					if err != nil {
						continue
					}
					headers = h
				} else {
					// Single file as a header
					mtime := info.ModTime().UnixMilli()
					rel, _ := filepath.Rel(abs, targetPath)
					headers = []memory.MemoryHeader{{
						Filename: filepath.ToSlash(rel),
						FilePath: targetPath,
						MtimeMs:  mtime,
					}}
				}

				mems, err := Ingest(headers)
				if err == nil {
					allMemories = append(allMemories, mems...)
				}
			}
		}

		// Sort by mtime ascending
		sort.Slice(allMemories, func(i, j int) bool {
			return allMemories[i].MtimeMs < allMemories[j].MtimeMs
		})

		queryEntries = append(queryEntries, QueryEntry{
			ID:       e.ID,
			Semantic: allMemories,
		})
	}

	// Sort entries by their earliest memory's mtime
	// (entries with no memories come last)
	sort.SliceStable(queryEntries, func(i, j int) bool {
		a, b := queryEntries[i], queryEntries[j]
		if len(a.Semantic) == 0 && len(b.Semantic) == 0 {
			return a.ID < b.ID
		}
		if len(a.Semantic) == 0 {
			return false
		}
		if len(b.Semantic) == 0 {
			return true
		}
		return a.Semantic[0].MtimeMs < b.Semantic[0].MtimeMs
	})

	return QueryResult{
		Entries:   queryEntries,
		Formatted: formatQueryEntries(queryEntries),
	}, nil
}

type queryEntrySpec struct {
	ID    string
	Paths []string
}

// parseQueryEntries reads the entrypoint file and extracts the
// `# Query:` list items matching the regex pattern from the spec.
func parseQueryEntries(entryPath string) ([]queryEntrySpec, error) {
	f, err := os.Open(entryPath)
	if err != nil {
		return nil, nil // entry file missing is fine
	}
	defer f.Close()

	// Read the frontmatter first to check for `query: false`
	if skip := checkFrontmatterSkip(f); skip {
		return nil, nil
	}

	// Reset file position
	f.Seek(0, 0)

	var entries []queryEntrySpec
	scanner := newLineScanner(f)

	inQuery := false
	var currentID string
	var currentPaths []string

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Detect "# Query:" heading
		if strings.HasPrefix(trimmed, "# Query:") || strings.HasPrefix(trimmed, "# Query :") {
			inQuery = true
			continue
		}

		if !inQuery {
			continue
		}

		// Stop at next heading
		if strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "##") {
			// Flush current entry
			if currentID != "" {
				entries = append(entries, queryEntrySpec{
					ID:    currentID,
					Paths: currentPaths,
				})
				currentID = ""
				currentPaths = nil
			}
			break
		}

		// Match list item: `- [id] text` or `- [id](path) text`
		item := parseQueryItem(trimmed)
		if item == nil {
			continue
		}

		if currentID != "" {
			// Flush previous entry
			entries = append(entries, queryEntrySpec{
				ID:    currentID,
				Paths: currentPaths,
			})
		}

		currentID = item.id
		currentPaths = item.paths
	}

	// Flush last entry
	if currentID != "" {
		entries = append(entries, queryEntrySpec{
			ID:    currentID,
			Paths: currentPaths,
		})
	}

	return entries, scanner.Err()
}

type queryItem struct {
	id    string
	paths []string
}

// parseQueryItem tries to parse a line like:
// `- [example_id] description text`
// `- [example_id](/path/to/dir) description text`
func parseQueryItem(line string) *queryItem {
	if !strings.HasPrefix(line, "- [") {
		return nil
	}

	// Extract ID between first [ and ]
	idStart := 3 // after "- ["
	idEnd := strings.Index(line[idStart:], "]")
	if idEnd == -1 {
		return nil
	}

	id := strings.TrimSpace(line[idStart : idStart+idEnd])
	if id == "" {
		return nil
	}

	rest := line[idStart+idEnd+1:]

	var paths []string

	// Check for (path) immediately after ]
	if strings.HasPrefix(strings.TrimSpace(rest), "(") {
		pathStart := strings.Index(rest, "(")
		pathEnd := strings.Index(rest, ")")
		if pathStart != -1 && pathEnd > pathStart {
			p := strings.TrimSpace(rest[pathStart+1 : pathEnd])
			if p != "" {
				paths = append(paths, p)
			}
		}
	}

	return &queryItem{
		id:    id,
		paths: paths,
	}
}

// checkFrontmatterSkip reads frontmatter and returns true if `query: false`
func checkFrontmatterSkip(f *os.File) bool {
	fm := parseFrontmatter(f)
	return strings.TrimSpace(strings.ToLower(fm.query)) == "false"
}

func newLineScanner(f *os.File) *bufio.Scanner {
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 65536), 10*1024*1024)
	return scanner
}

func formatQueryEntries(entries []QueryEntry) string {
	var sb strings.Builder
	for _, e := range entries {
		if len(e.Semantic) == 0 {
			sb.WriteString(fmt.Sprintf("## %s\n\nNo matching memories.\n\n", e.ID))
			continue
		}
		sb.WriteString(FormatMemories(e.Semantic))
	}
	return sb.String()
}
