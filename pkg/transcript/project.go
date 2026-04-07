package transcript

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// ProjectManager manages transcript files for projects (sessions grouped by CWD)
type ProjectManager struct {
	mu              sync.RWMutex
	baseDir         string
	transcriptFiles map[string]*TranscriptFile // key: sessionID
}

// NewProjectManager creates a new project manager
func NewProjectManager(baseDir string) (*ProjectManager, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			baseDir = ".dogclaw/projects"
		} else {
			baseDir = filepath.Join(home, ".dogclaw", "projects")
		}
	}

	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create projects directory: %w", err)
	}

	return &ProjectManager{
		baseDir:         baseDir,
		transcriptFiles: make(map[string]*TranscriptFile),
	}, nil
}

// GetTranscriptFile gets or creates a transcript file for the given session
func (pm *ProjectManager) GetTranscriptFile(sessionID, cwd string) *TranscriptFile {
	pm.mu.RLock()
	if tf, ok := pm.transcriptFiles[sessionID]; ok {
		pm.mu.RUnlock()
		return tf
	}
	pm.mu.RUnlock()

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Double check
	if tf, ok := pm.transcriptFiles[sessionID]; ok {
		return tf
	}

	tf := pm.createTranscriptFile(sessionID, cwd)
	pm.transcriptFiles[sessionID] = tf
	return tf
}

// createTranscriptFile creates a new transcript file for a session
func (pm *ProjectManager) createTranscriptFile(sessionID, cwd string) *TranscriptFile {
	projectDir := pm.sanitizeCWDForPath(cwd)
	sessionDir := filepath.Join(pm.baseDir, projectDir, "session")

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		// Fall back to a safe directory name
		sessionDir = filepath.Join(pm.baseDir, "unknown", "session")
		os.MkdirAll(sessionDir, 0755)
	}

	filePath := filepath.Join(sessionDir, sessionID+".jsonl")
	return NewTranscriptFile(filePath)
}

// sanitizeCWDForPath converts a CWD to a safe directory name
func (pm *ProjectManager) sanitizeCWDForPath(cwd string) string {
	if cwd == "" {
		return "no-cwd"
	}

	// Replace path separators with underscores
	safe := strings.ReplaceAll(cwd, string(filepath.Separator), "_")
	safe = strings.ReplaceAll(safe, "/", "_")
	safe = strings.ReplaceAll(safe, "\\", "_")

	// Replace Windows drive colon
	if len(safe) > 1 && safe[1] == ':' {
		safe = safe[:1] + "_" + safe[2:]
	}

	// Remove or replace any remaining problematic characters
	re := regexp.MustCompile(`[^a-zA-Z0-9_.-]`)
	safe = re.ReplaceAllString(safe, "_")

	// Collapse multiple underscores
	for strings.Contains(safe, "__") {
		safe = strings.ReplaceAll(safe, "__", "_")
	}

	// Trim leading/trailing underscores
	safe = strings.Trim(safe, "_")

	if safe == "" {
		safe = "unknown"
	}

	return safe
}

// Close closes all transcript files
func (pm *ProjectManager) Close() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var lastErr error
	for sessionID, tf := range pm.transcriptFiles {
		if err := tf.Close(); err != nil {
			lastErr = err
		}
		delete(pm.transcriptFiles, sessionID)
	}
	return lastErr
}

// CloseSession closes a specific session's transcript file
func (pm *ProjectManager) CloseSession(sessionID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if tf, ok := pm.transcriptFiles[sessionID]; ok {
		delete(pm.transcriptFiles, sessionID)
		return tf.Close()
	}
	return nil
}

// ListSessions lists all transcript files for all projects
func (pm *ProjectManager) ListSessions() ([]SessionInfo, error) {
	entries, err := os.ReadDir(pm.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read projects directory: %w", err)
	}

	var sessions []SessionInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectDir := entry.Name()
		sessionDir := filepath.Join(pm.baseDir, projectDir, "session")
		jsonlFiles, err := filepath.Glob(filepath.Join(sessionDir, "*.jsonl"))
		if err != nil {
			continue
		}

		for _, jsonlFile := range jsonlFiles {
			sessionID := strings.TrimSuffix(filepath.Base(jsonlFile), ".jsonl")
			sessions = append(sessions, SessionInfo{
				SessionID:   sessionID,
				ProjectPath: projectDir,
				FilePath:    jsonlFile,
			})
		}
	}

	return sessions, nil
}

// SessionInfo contains information about a stored session
type SessionInfo struct {
	SessionID   string
	ProjectPath string
	FilePath    string
}

// GetBaseDir returns the base directory for transcript storage
func (pm *ProjectManager) GetBaseDir() string {
	return pm.baseDir
}
