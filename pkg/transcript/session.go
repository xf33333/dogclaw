package transcript

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SessionSummary holds the display info for a stored session.
// Extracted by scanning the transcript JSONL (similar to claude-code's loadLiteMetadata).
type SessionSummary struct {
	SessionID    string    `json:"sessionId"`
	FilePath     string    `json:"filePath"`
	ProjectDir   string    `json:"projectDir"` // sanitized project path
	Title        string    `json:"title,omitempty"`
	LastPrompt   string    `json:"lastPrompt,omitempty"`
	FirstPrompt  string    `json:"firstPrompt,omitempty"`
	Modified     time.Time `json:"modified"`
	MessageCount int       `json:"messageCount"`
	UserMessages int       `json:"userMessages"`
	IsSidechain  bool      `json:"isSidechain,omitempty"`
	AgentName    string    `json:"agentName,omitempty"`
	Mode         string    `json:"mode,omitempty"`
	Tag          string    `json:"tag,omitempty"`
}

// SessionManager manages session discovery, listing, and resume operations.
// Translated from claude-code's sessionStorage.ts (listing/enrichment logic).
type SessionManager struct {
	mu      sync.RWMutex
	baseDir string // ~/.dogclaw/projects

	// Cache: sanitized project dir → list of session summaries
	cache    map[string][]SessionSummary
	cacheAt  time.Time
	cacheTTL time.Duration
}

// NewSessionManager creates a session manager.
func NewSessionManager(baseDir string) (*SessionManager, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot get home dir: %w", err)
		}
		baseDir = filepath.Join(home, ".dogclaw", "projects")
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create projects dir: %w", err)
	}
	return &SessionManager{
		baseDir:  baseDir,
		cache:    make(map[string][]SessionSummary),
		cacheTTL: 5 * time.Second,
	}, nil
}

// ListAllSessions returns every session found under baseDir, sorted by modified time (newest first).
func (sm *SessionManager) ListAllSessions() ([]SessionSummary, error) {
	sm.mu.RLock()
	if time.Since(sm.cacheAt) < sm.cacheTTL && len(sm.cache) > 0 {
		// Merge all cached sessions
		var result []SessionSummary
		for _, sessions := range sm.cache {
			result = append(result, sessions...)
		}
		sm.mu.RUnlock()
		sort.Slice(result, func(i, j int) bool {
			return result[i].Modified.After(result[j].Modified)
		})
		return result, nil
	}
	sm.mu.RUnlock()

	sm.mu.Lock()
	defer sm.mu.Unlock()

	return sm.listAllSessionsUnlocked()
}

func (sm *SessionManager) listAllSessionsUnlocked() ([]SessionSummary, error) {
	entries, err := os.ReadDir(sm.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read projects dir: %w", err)
	}

	var allSessions []SessionSummary
	sm.cache = make(map[string][]SessionSummary)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectDir := entry.Name()
		sessions, err := sm.scanProjectDir(projectDir)
		if err != nil {
			continue // skip unreadable project dirs
		}
		sm.cache[projectDir] = sessions
		allSessions = append(allSessions, sessions...)
	}

	sm.cacheAt = time.Now()

	sort.Slice(allSessions, func(i, j int) bool {
		return allSessions[i].Modified.After(allSessions[j].Modified)
	})
	return allSessions, nil
}

// ListSessionsForCwd returns sessions for a specific sanitized project dir,
// sorted by modified time (newest first).
func (sm *SessionManager) ListSessionsForCwd(projectDir string) ([]SessionSummary, error) {
	if projectDir == "" {
		projectDir = "no-cwd"
	}

	sm.mu.RLock()
	if sessions, ok := sm.cache[projectDir]; ok && time.Since(sm.cacheAt) < sm.cacheTTL {
		result := make([]SessionSummary, len(sessions))
		copy(result, sessions)
		sm.mu.RUnlock()
		return result, nil
	}
	sm.mu.RUnlock()

	sm.mu.Lock()
	defer sm.mu.Unlock()

	sessions, err := sm.scanProjectDir(projectDir)
	if err != nil {
		return nil, err
	}
	sm.cache[projectDir] = sessions
	sm.cacheAt = time.Now()

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Modified.After(sessions[j].Modified)
	})
	return sessions, nil
}

// scanProjectDir scans a project directory for .jsonl transcript files
// and extracts summary metadata from each.
func (sm *SessionManager) scanProjectDir(projectDir string) ([]SessionSummary, error) {
	dirPath := filepath.Join(sm.baseDir, projectDir, "session")
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []SessionSummary
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		filePath := filepath.Join(dirPath, entry.Name())
		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")

		// Get file modification time via os.Stat (os.DirEntry doesn't have ModTime)
		var modTime time.Time
		if info, err := os.Stat(filePath); err == nil {
			modTime = info.ModTime()
		}

		summary := SessionSummary{
			SessionID:  sessionID,
			FilePath:   filePath,
			ProjectDir: projectDir,
			Modified:   modTime,
		}

		// Enrich from transcript file (fast: only reads metadata entries + first/last lines)
		sm.enrichFromFile(filePath, &summary)

		sessions = append(sessions, summary)
	}
	return sessions, nil
}

// enrichFromFile scans a transcript JSONL file to extract
// metadata entries, message counts, first/last prompts.
// Uses a streaming approach — reads line by line but doesn't
// hold the entire file in memory.
func (sm *SessionManager) enrichFromFile(filePath string, s *SessionSummary) {
	f, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer f.Close()

	// Use tail-read for metadata (last 64KB), then forward-read for first-prompt
	// For simplicity with smaller files, just do a single pass
	const maxScanBytes = 10 * 1024 * 1024 // 10MB limit

	scanner := newJSONScanner(f, maxScanBytes)
	lineNum := 0
	var record TranscriptRecord
	var firstUserPrompt string
	messageCount := 0
	userMsgCount := 0

	for {
		line := scanner.nextLine()
		if line == nil {
			break
		}
		lineNum++
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}

		// Only count non-sidechain transcript messages
		if !record.IsSidechain && isTranscriptMessageType(record.Type) {
			messageCount++
		}

		switch record.Type {
		case MessageTypeMetadata:
			if record.Metadata != nil {
				switch record.Metadata.Key {
				case string(MetadataCustomTitle):
					s.Title = record.Metadata.Value
				case string(MetadataLastPrompt):
					s.LastPrompt = record.Metadata.Value
				case string(MetadataAgentName):
					s.AgentName = record.Metadata.Value
				case string(MetadataMode):
					s.Mode = record.Metadata.Value
				case string(MetadataSummary):
					if s.FirstPrompt == "" {
						s.FirstPrompt = record.Metadata.Value
					}
				case "tag":
					s.Tag = record.Metadata.Value
				}
			}
		case MessageTypeUser:
			userMsgCount++
			if firstUserPrompt == "" && !record.IsSidechain {
				// Extract first meaningful user prompt
				firstUserPrompt = extractFirstPrompt(record.Content)
			}
		}
	}

	s.MessageCount = messageCount
	s.UserMessages = userMsgCount
	if firstUserPrompt != "" {
		s.FirstPrompt = firstUserPrompt
	}

	// Use file mtime if Modified is not yet set
	if s.Modified.IsZero() {
		if info, err := os.Stat(filePath); err == nil {
			s.Modified = info.ModTime()
		}
	}
}

// jsonlScanner is a simple line-by-line JSONL scanner that avoids
// bufio's default 64KB line limit (transcript lines can be very long).
type jsonlScanner struct {
	f    *os.File
	buf  []byte
	pos  int
	line []byte
	eof  bool
}

func newJSONScanner(f *os.File, maxSize int) *jsonlScanner {
	buf := make([]byte, 0, 64*1024)
	return &jsonlScanner{f: f, buf: buf}
}

func (s *jsonlScanner) nextLine() []byte {
	s.line = s.line[:0]
	for {
		if s.pos >= len(s.buf) {
			// Shift remaining data to start of buffer
			remaining := len(s.buf) - s.pos
			if remaining > 0 {
				copy(s.buf[:remaining], s.buf[s.pos:])
			}
			s.buf = s.buf[:remaining]
			s.pos = 0

			// Refill buffer
			chunk := make([]byte, 64*1024)
			n, err := s.f.Read(chunk)
			if n > 0 {
				s.buf = append(s.buf, chunk[:n]...)
			}
			if err != nil {
				if len(s.buf) == 0 {
					return nil // EOF
				}
				// Return remaining data
				line := s.buf
				s.buf = nil
				return line
			}
		}

		b := s.buf[s.pos]
		s.pos++
		if b == '\n' {
			result := make([]byte, len(s.line))
			copy(result, s.line)
			return result
		}
		s.line = append(s.line, b)
	}
}

func isTranscriptMessageType(t MessageType) bool {
	return t == MessageTypeUser || t == MessageTypeAssistant ||
		t == MessageTypeSystem || t == MessageTypeAttachment
}

// extractFirstPrompt extracts a meaningful first prompt from user message content.
// Strips out system notifications, hook output, etc.
func extractFirstPrompt(content string) string {
	content = strings.TrimSpace(content)
	if len(content) == 0 {
		return ""
	}
	// Skip XML-tag prefixed lines (IDE context, hooks, etc.)
	if strings.HasPrefix(content, "<") {
		// Find first non-XML line
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "<") {
				return truncateTo(line, 200)
			}
		}
		return ""
	}
	return truncateTo(content, 200)
}

func truncateTo(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Truncate at word boundary if possible
	if maxLen > 3 {
		truncated := s[:maxLen-3]
		if idx := strings.LastIndexAny(truncated, " \t"); idx > maxLen/2 {
			return truncated[:idx] + "…"
		}
		return truncated + "…"
	}
	return s
}

// GetSessionBySessionID finds and returns a session summary for a given session ID.
// Searches all project directories.
func (sm *SessionManager) GetSessionBySessionID(sessionID string) (*SessionSummary, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID is empty")
	}

	allSessions, err := sm.ListAllSessions()
	if err != nil {
		return nil, err
	}
	for _, s := range allSessions {
		if s.SessionID == sessionID {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("session %q not found", sessionID)
}

// SearchSessions searches sessions by query string (matches against title,
// first prompt, last prompt, session ID).
func (sm *SessionManager) SearchSessions(query string) ([]SessionSummary, error) {
	if query == "" {
		return sm.ListAllSessions()
	}

	allSessions, err := sm.ListAllSessions()
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(query)
	var results []SessionSummary
	for _, s := range allSessions {
		if strings.Contains(strings.ToLower(s.SessionID), queryLower) ||
			strings.Contains(strings.ToLower(s.Title), queryLower) ||
			strings.Contains(strings.ToLower(s.FirstPrompt), queryLower) ||
			strings.Contains(strings.ToLower(s.LastPrompt), queryLower) {
			results = append(results, s)
		}
	}
	return results, nil
}

// FormatSessionSummary returns a human-readable one-line summary for display.
func (s *SessionSummary) FormatSummary() string {
	parts := []string{}
	if s.Title != "" {
		parts = append(parts, s.Title)
	} else if s.FirstPrompt != "" {
		parts = append(parts, s.FirstPrompt+"…")
	} else {
		parts = append(parts, s.SessionID)
	}
	parts = append(parts, fmt.Sprintf("(%s)", s.SessionID[:8]))
	if s.Modified.IsZero() {
		parts = append(parts, "(no date)")
	} else {
		parts = append(parts, s.Modified.Format("2006-01-02 15:04"))
	}
	return strings.Join(parts, " ")
}

// InvalidateCache clears the session listing cache.
func (sm *SessionManager) InvalidateCache() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.cache = make(map[string][]SessionSummary)
	sm.cacheAt = time.Time{}
}
