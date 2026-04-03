package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	maxHistoryItems = 100
	flushInterval   = 500 * time.Millisecond
)

// HistoryEntry represents a single history item
type HistoryEntry struct {
	Display        string                `json:"display"`
	PastedContents map[int]PastedContent `json:"pasted_contents,omitempty"`
}

// PastedContent represents pasted text content
type PastedContent struct {
	ID        int    `json:"id"`
	Type      string `json:"type"` // "text" or "image"
	Content   string `json:"content,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Filename  string `json:"filename,omitempty"`
}

// LogEntry is the disk format for history entries
type LogEntry struct {
	Display        string                      `json:"display"`
	PastedContents map[int]StoredPastedContent `json:"pasted_contents,omitempty"`
	Timestamp      int64                       `json:"timestamp"`
	Project        string                      `json:"project"`
	SessionID      string                      `json:"session_id,omitempty"`
}

// StoredPastedContent is the disk format (may use hash reference for large content)
type StoredPastedContent struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	Content     string `json:"content,omitempty"`      // Inline for small content
	ContentHash string `json:"content_hash,omitempty"` // Hash reference for large content
	MediaType   string `json:"media_type,omitempty"`
	Filename    string `json:"filename,omitempty"`
}

var (
	instance     *HistoryManager
	instanceOnce sync.Once
)

// HistoryManager manages command history persistence
type HistoryManager struct {
	mu          sync.Mutex
	pending     []LogEntry
	isWriting   bool
	historyPath string
	projectRoot string
	sessionID   string
	flushTimer  *time.Timer
}

// GetHistoryManager returns the singleton history manager
func GetHistoryManager() *HistoryManager {
	instanceOnce.Do(func() {
		instance = &HistoryManager{}
	})
	return instance
}

// Init initializes the history manager with project and session info
func (h *HistoryManager) Init(projectRoot, sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.projectRoot = projectRoot
	h.sessionID = sessionID

	// Set history file path (~/.dogclaw/history.jsonl)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		claudeDir := filepath.Join(homeDir, ".dogclaw")
		os.MkdirAll(claudeDir, 0700)
		h.historyPath = filepath.Join(claudeDir, "history.jsonl")
	}
}

// AddToHistory adds a command to history (non-blocking)
func (h *HistoryManager) AddToHistory(entry HistoryEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.historyPath == "" {
		return // Not initialized
	}

	// Convert to log format
	logEntry := LogEntry{
		Display:   entry.Display,
		Timestamp: time.Now().UnixMilli(),
		Project:   h.projectRoot,
		SessionID: h.sessionID,
	}

	// Handle pasted content (store inline for small, hash for large)
	if entry.PastedContents != nil {
		logEntry.PastedContents = make(map[int]StoredPastedContent)
		for id, pc := range entry.PastedContents {
			if pc.Type == "image" {
				continue // Images stored separately
			}
			if len(pc.Content) <= 1024 {
				// Small content: store inline
				logEntry.PastedContents[id] = StoredPastedContent{
					ID:        pc.ID,
					Type:      pc.Type,
					Content:   pc.Content,
					MediaType: pc.MediaType,
					Filename:  pc.Filename,
				}
			} else {
				// Large content: compute hash, store reference
				hash := hashContent(pc.Content)
				logEntry.PastedContents[id] = StoredPastedContent{
					ID:          pc.ID,
					Type:        pc.Type,
					ContentHash: hash,
					MediaType:   pc.MediaType,
					Filename:    pc.Filename,
				}
				// Fire-and-forget: store large content to disk
				go storePastedContent(hash, pc.Content)
			}
		}
	}

	h.pending = append(h.pending, logEntry)

	// Schedule flush
	if h.flushTimer != nil {
		h.flushTimer.Stop()
	}
	h.flushTimer = time.AfterFunc(flushInterval, func() {
		h.flush()
	})
}

// AddSimpleHistory adds a simple string to history
func (h *HistoryManager) AddSimpleHistory(command string) {
	h.AddToHistory(HistoryEntry{Display: command})
}

// GetHistory returns history entries for the current project
// Current session entries first, then other sessions (newest first)
func (h *HistoryManager) GetHistory() ([]HistoryEntry, error) {
	h.mu.Lock()
	projectRoot := h.projectRoot
	sessionID := h.sessionID
	historyPath := h.historyPath
	h.mu.Unlock()

	if historyPath == "" {
		return nil, nil
	}

	var entries []HistoryEntry
	var otherSessionEntries []LogEntry

	// Read pending first (current session, newest first)
	h.mu.Lock()
	for i := len(h.pending) - 1; i >= 0; i-- {
		pending := h.pending[i]
		if pending.Project == projectRoot {
			if pending.SessionID == sessionID {
				entries = append(entries, logEntryToHistoryEntry(pending))
			} else {
				otherSessionEntries = append(otherSessionEntries, pending)
			}
		}
	}
	h.mu.Unlock()

	// Then read from disk (reverse order - newest first)
	file, err := os.Open(historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return entries, nil
		}
		return nil, err
	}
	defer file.Close()

	// Read all lines, then reverse
	var diskLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		diskLines = append(diskLines, scanner.Text())
	}

	// Process in reverse (newest first)
	for i := len(diskLines) - 1; i >= 0 && len(entries) < maxHistoryItems; i-- {
		var logEntry LogEntry
		if err := json.Unmarshal([]byte(diskLines[i]), &logEntry); err != nil {
			continue // Skip malformed
		}
		if logEntry.Project != projectRoot {
			continue
		}
		if logEntry.SessionID == sessionID {
			entries = append(entries, logEntryToHistoryEntry(logEntry))
		} else {
			otherSessionEntries = append(otherSessionEntries, logEntry)
		}
	}

	// Add other session entries
	for _, entry := range otherSessionEntries {
		if len(entries) >= maxHistoryItems {
			break
		}
		entries = append(entries, logEntryToHistoryEntry(entry))
	}

	return entries, nil
}

// Flush forces an immediate flush of pending entries
func (h *HistoryManager) Flush() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.flush()
}

// flush writes pending entries to disk (caller must hold lock)
func (h *HistoryManager) flush() {
	if len(h.pending) == 0 {
		return
	}
	if h.isWriting {
		return
	}

	h.isWriting = true
	pending := h.pending
	h.pending = nil

	go func() {
		defer func() {
			h.mu.Lock()
			h.isWriting = false
			h.mu.Unlock()
		}()

		// Ensure file exists
		f, err := os.OpenFile(h.historyPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return
		}
		defer f.Close()

		writer := bufio.NewWriter(f)
		for _, entry := range pending {
			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			writer.Write(data)
			writer.WriteByte('\n')
		}
		writer.Flush()
	}()
}

func logEntryToHistoryEntry(entry LogEntry) HistoryEntry {
	result := HistoryEntry{
		Display: entry.Display,
	}

	if entry.PastedContents != nil {
		result.PastedContents = make(map[int]PastedContent)
		for id, stored := range entry.PastedContents {
			if stored.Content != "" {
				result.PastedContents[id] = PastedContent{
					ID:        stored.ID,
					Type:      stored.Type,
					Content:   stored.Content,
					MediaType: stored.MediaType,
					Filename:  stored.Filename,
				}
			} else if stored.ContentHash != "" {
				// Try to retrieve from paste store
				if content := retrievePastedContent(stored.ContentHash); content != "" {
					result.PastedContents[id] = PastedContent{
						ID:        stored.ID,
						Type:      stored.Type,
						Content:   content,
						MediaType: stored.MediaType,
						Filename:  stored.Filename,
					}
				}
			}
		}
	}

	return result
}

// Simple hash function for content (not cryptographic, just for dedup)
func hashContent(content string) string {
	var hash uint32
	for i := 0; i < len(content); i++ {
		hash = hash*31 + uint32(content[i])
	}
	return fmt.Sprintf("%08x", hash)
}

// storePastedContent saves large content to disk (fire-and-forget)
func storePastedContent(hash string, content string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	pasteDir := filepath.Join(homeDir, ".dogclaw", "pastes")
	os.MkdirAll(pasteDir, 0700)

	pastePath := filepath.Join(pasteDir, hash)
	os.WriteFile(pastePath, []byte(content), 0600)
}

// retrievePastedContent retrieves content from disk store
func retrievePastedContent(hash string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	pastePath := filepath.Join(homeDir, ".dogclaw", "pastes", hash)
	data, err := os.ReadFile(pastePath)
	if err != nil {
		return ""
	}
	return string(data)
}
