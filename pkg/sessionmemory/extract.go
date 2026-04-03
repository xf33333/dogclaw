// Package sessionmemory provides session-level memory management.
// It manages SESSION.md files per session, unlike memdir which handles
// persistent typed memory (MEMORY.md).
package sessionmemory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExtractSessionMemory extracts structured information from a session
// transcript and writes it to the session memory file (SESSION.md).
func ExtractSessionMemory(transcriptContent string, sessionDir string, sessionID string) error {
	if transcriptContent == "" || sessionDir == "" || sessionID == "" {
		return nil
	}

	sessionPath := filepath.Join(sessionDir, sessionID)
	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		return fmt.Errorf("ensure session dir: %w", err)
	}

	// Read existing session memory if any
	existing, _ := ReadSessionFile(sessionDir, sessionID)

	// Extract sections from transcript
	sections := extractSections(transcriptContent, existing)

	// Update session file if changes found
	if len(sections) > 0 {
		return updateSessionFile(sessionDir, sessionID, sections)
	}

	return nil
}

// SectionExtraction holds extracted information for a section
type SectionExtraction struct {
	SectionName string
	Content     string
}

// extractSections parses transcript content and returns new/updated sections
func extractSections(transcriptContent string, existingMemory string) []SectionExtraction {
	var sections []SectionExtraction

	// Look for structured XML tags in transcript indicating memory-worthy content
	typeExtractor := func(tagName, sectionName string) {
		startMarker := fmt.Sprintf("<%s>", tagName)
		endMarker := fmt.Sprintf("</%s>", tagName)

		idx := strings.Index(transcriptContent, startMarker)
		if idx == -1 {
			return
		}

		content := transcriptContent[idx+len(startMarker):]
		endIdx := strings.Index(content, endMarker)
		if endIdx == -1 {
			return
		}

		body := strings.TrimSpace(content[:endIdx])
		if body != "" {
			sections = append(sections, SectionExtraction{
				SectionName: sectionName,
				Content:     body,
			})
		}
	}

	// Extract conversation insights
	typeExtractor("session_goal", "Task Specification")
	typeExtractor("session_state", "Current State")
	typeExtractor("key_files", "Files and Functions")
	typeExtractor("commands", "Workflow")
	typeExtractor("errors_fixed", "Errors and Corrections")
	typeExtractor("learnings", "Learnings")
	typeExtractor("key_results", "Key Results")
	typeExtractor("worklog", "Worklog")

	return sections
}

// updateSessionFile updates the SESSION.md with extracted sections
func updateSessionFile(sessionDir, sessionID string, extractions []SectionExtraction) error {
	path := SessionFilePath(sessionDir, sessionID)

	existing, err := os.ReadFile(path)
	if err != nil {
		// Create new file with template
		template := GenerateDefaultTemplate()
		return os.WriteFile(path, []byte(template), 0644)
	}

	content := string(existing)

	// Update sections with new content
	for _, ext := range extractions {
		// Simple replacement: find section header and append content
		sectionHeader := fmt.Sprintf("# %s", ext.SectionName)
		if idx := strings.Index(content, sectionHeader); idx != -1 {
			// Found existing section, append content
			content += fmt.Sprintf("\n## %s (Extracted)\n\n%s\n", ext.SectionName, ext.Content)
		} else {
			// Add as new section
			content += fmt.Sprintf("\n# %s\n\n%s\n", ext.SectionName, ext.Content)
		}
	}

	return os.WriteFile(path, []byte(content), 0644)
}
