// Package memdir provides the memory directory system for persistent agent memory.
// Translated from src/memdir/
package memdir

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MemoryType represents the type of memory
type MemoryType string

const (
	MemoryTypeUser      MemoryType = "user"
	MemoryTypeFeedback  MemoryType = "feedback"
	MemoryTypeProject   MemoryType = "project"
	MemoryTypeReference MemoryType = "reference"
)

// ValidMemoryTypes lists all valid memory types
var ValidMemoryTypes = []MemoryType{
	MemoryTypeUser, MemoryTypeFeedback, MemoryTypeProject, MemoryTypeReference,
}

// ParseMemoryType parses a raw string into a MemoryType
func ParseMemoryType(raw string) (MemoryType, bool) {
	for _, t := range ValidMemoryTypes {
		if strings.EqualFold(raw, string(t)) {
			return t, true
		}
	}
	return "", false
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
	path := filepath.Join(ms.Dir, "MEMORY.md")
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

	path := filepath.Join(ms.Dir, "MEMORY.md")
	var sb strings.Builder

	sb.WriteString("---\n")
	sb.WriteString("# Memory\n\n")
	sb.WriteString("---\n\n")

	for _, entry := range entries {
		sb.WriteString(entry.Format())
		sb.WriteString("\n\n")
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

// Clear removes all memories
func (ms *MemoryStore) Clear() error {
	path := filepath.Join(ms.Dir, "MEMORY.md")
	return os.Remove(path)
}

// Format formats a memory entry as markdown
func (e *MemoryEntry) Format() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("---\nname: %s\ndescription: %s\ntype: %s\n---\n\n",
		e.Name, e.Description, e.Type))
	sb.WriteString(e.Content)
	return sb.String()
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
				t, _ := ParseMemoryType(strings.TrimSpace(strings.TrimPrefix(line, "type:")))
				entry.Type = t
			}
		}
		entry.Content = body
		entries = append(entries, entry)
	}

	return entries, nil
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
func WhatNotToSave() []string {
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
