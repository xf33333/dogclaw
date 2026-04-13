// Package transcript provides session transcript persistence using JSONL format.
// It records the complete conversation history for each session, supporting
// resume, token estimation, and message chain tracking.
package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MessageType represents the type of a message in the transcript
type MessageType string

const (
	MessageTypeUser       MessageType = "user"
	MessageTypeAssistant  MessageType = "assistant"
	MessageTypeAttachment MessageType = "attachment"
	MessageTypeSystem     MessageType = "system"
	MessageTypeMetadata   MessageType = "metadata" // Internal metadata entries
)

// MetadataKey represents keys for metadata entries
type MetadataKey string

const (
	MetadataLastPrompt    MetadataKey = "last-prompt"
	MetadataCustomTitle   MetadataKey = "custom-title"
	MetadataAgentName     MetadataKey = "agent-name"
	MetadataAgentColor    MetadataKey = "agent-color"
	MetadataMode          MetadataKey = "mode"
	MetadataPRLink        MetadataKey = "pr-link"
	MetadataSummary       MetadataKey = "summary"
	MetadataCompaction    MetadataKey = "compaction"
	MetadataTokenEstimate MetadataKey = "token-estimate"
)

// TranscriptVersion is the current version of the transcript format
const TranscriptVersion = "0.1.0"

// DefaultMaxFileSize is the maximum transcript file size before rotation (1MB)
const DefaultMaxFileSize = 1 * 1024 * 1024

// TranscriptRecord represents a single record in the JSONL transcript file
type TranscriptRecord struct {
	// Type is the message type (user, assistant, attachment, system, metadata)
	Type MessageType `json:"type"`
	// UUID is the unique identifier for this message
	UUID string `json:"uuid"`
	// ParentUUID links to the parent message for conversation chain reconstruction
	ParentUUID string `json:"parentUuid,omitempty"`
	// IsSidechain indicates if this is from a side conversation (e.g., subagent)
	IsSidechain bool `json:"isSidechain"`
	// SessionID is the session this record belongs to
	SessionID string `json:"sessionId"`
	// Cwd is the working directory when the message was recorded
	Cwd string `json:"cwd,omitempty"`
	// Version is the transcript format version
	Version string `json:"version"`
	// GitBranch is the current git branch (if applicable)
	GitBranch string `json:"gitBranch,omitempty"`
	// Timestamp is when this record was created
	Timestamp int64 `json:"timestamp"`
	// Role is the message role (for user/assistant messages)
	Role string `json:"role,omitempty"`
	// Content is the message content (text or JSON-encoded structured data)
	Content string `json:"content,omitempty"`
	// ToolUseID is the tool_use id for tool result messages
	ToolUseID string `json:"toolUseId,omitempty"`
	// ToolName is the name of the tool that was called
	ToolName string `json:"toolName,omitempty"`
	// Metadata is used for metadata-type records
	Metadata *MetadataEntry `json:"metadata,omitempty"`
}

// MetadataEntry is the metadata payload for metadata-type records
type MetadataEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// TranscriptFile manages a single session's transcript file
type TranscriptFile struct {
	mu         sync.Mutex
	filePath   string
	writeQueue []TranscriptRecord
	seenUUIDs  map[string]bool
	queueTimer *time.Timer
	flushDone  chan struct{}
	isClosed   bool

	// File rotation support
	maxFileSize  int64     // Maximum file size before rotation (default 1MB)
	currentSize  int64     // Current approximate file size in bytes
	rotationTime time.Time // Time of current file creation
}

// NewTranscriptFile creates a new transcript file manager for the given path
func NewTranscriptFile(filePath string) *TranscriptFile {
	tf := &TranscriptFile{
		filePath:     filePath,
		writeQueue:   make([]TranscriptRecord, 0, 64),
		seenUUIDs:    make(map[string]bool),
		flushDone:    make(chan struct{}),
		maxFileSize:  DefaultMaxFileSize,
		rotationTime: time.Now(),
	}

	// Ensure parent directory exists
	dir := filepath.Dir(filePath)
	os.MkdirAll(dir, 0755)

	// Get current file size if file exists (for rotation decision)
	if info, err := os.Stat(filePath); err == nil {
		tf.currentSize = info.Size()
	}

	return tf
}

// Queue adds a record to the write queue
func (tf *TranscriptFile) Queue(record TranscriptRecord) {
	if tf.shouldDeduplicate(record.UUID) {
		return
	}

	tf.mu.Lock()
	tf.seenUUIDs[record.UUID] = true
	tf.writeQueue = append(tf.writeQueue, record)
	tf.mu.Unlock()

	// Schedule flush
	tf.scheduleFlush()
}

// shouldDeduplicate checks if this UUID has already been seen
// Must be called before acquiring the lock to avoid blocking
func (tf *TranscriptFile) shouldDeduplicate(uuid string) bool {
	tf.mu.Lock()
	defer tf.mu.Unlock()
	return tf.seenUUIDs[uuid]
}

// scheduleFlush schedules a flush of the write queue
func (tf *TranscriptFile) scheduleFlush() {
	if tf.isClosed {
		return
	}

	// Cancel existing timer
	if tf.queueTimer != nil {
		tf.queueTimer.Stop()
	}

	tf.queueTimer = time.AfterFunc(100*time.Millisecond, func() {
		tf.Flush()
	})
}

// Flush writes all queued records to the transcript file
func (tf *TranscriptFile) Flush() error {
	tf.mu.Lock()
	if len(tf.writeQueue) == 0 {
		tf.mu.Unlock()
		return nil
	}

	records := make([]TranscriptRecord, len(tf.writeQueue))
	copy(records, tf.writeQueue)
	tf.writeQueue = tf.writeQueue[:0]
	tf.mu.Unlock()

	return tf.appendRecords(records)
}

// appendRecords appends records to the JSONL file
func (tf *TranscriptFile) appendRecords(records []TranscriptRecord) error {
	tf.mu.Lock()
	if tf.isClosed {
		tf.mu.Unlock()
		return fmt.Errorf("transcript file is closed")
	}
	tf.mu.Unlock()

	// Check if rotation is needed before writing
	if err := tf.checkAndRotate(records); err != nil {
		return fmt.Errorf("rotation failed: %w", err)
	}

	// Serialize records to measure size and append
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return fmt.Errorf("failed to encode transcript record: %w", err)
		}
	}

	// Write to disk
	f, err := os.OpenFile(tf.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open transcript file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("failed to write transcript records: %w", err)
	}

	// Update current file size under lock
	tf.mu.Lock()
	tf.currentSize += int64(buf.Len())
	tf.mu.Unlock()

	return nil
}

// Close flushes remaining records and closes the transcript file
func (tf *TranscriptFile) Close() error {
	tf.mu.Lock()
	if tf.isClosed {
		tf.mu.Unlock()
		return nil
	}
	tf.isClosed = true
	if tf.queueTimer != nil {
		tf.queueTimer.Stop()
	}

	records := make([]TranscriptRecord, len(tf.writeQueue))
	copy(records, tf.writeQueue)
	tf.writeQueue = nil
	tf.mu.Unlock()

	if len(records) > 0 {
		if err := tf.appendRecords(records); err != nil {
			return err
		}
	}

	close(tf.flushDone)
	return nil
}

// ReadMetadata reads metadata entries from the transcript file.
// It reads the entire file and processes lines from the end to find metadata entries.
func (tf *TranscriptFile) ReadMetadata() (map[string]string, error) {
	metadata := make(map[string]string)

	f, err := os.Open(tf.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return metadata, nil
		}
		return nil, fmt.Errorf("failed to open transcript file: %w", err)
	}
	defer f.Close()

	// Read entire file content
	content, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read transcript file: %w", err)
	}

	// Split content into lines
	lines := bytes.Split(content, []byte{'\n'})

	// Process lines from the end to find metadata
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var record TranscriptRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue // Skip invalid JSON
		}
		if record.Type == MessageTypeMetadata && record.Metadata != nil {
			metadata[record.Metadata.Key] = record.Metadata.Value
		}
	}

	return metadata, nil
}

// TruncateOrphanedRecords removes records from the given position onwards.
// This is used to implement tombstone behavior for removing orphaned messages.
func (tf *TranscriptFile) TruncateOrphanedRecords(position int64) error {
	if position <= 0 {
		return nil
	}

	return os.Truncate(tf.filePath, position)
}

// WriteMetadata appends a metadata record to the transcript
func (tf *TranscriptFile) WriteMetadata(key, value string) error {
	record := TranscriptRecord{
		Type:      MessageTypeMetadata,
		UUID:      fmt.Sprintf("meta-%d-%s", time.Now().UnixMilli(), key),
		SessionID: "meta",
		Version:   TranscriptVersion,
		Timestamp: time.Now().UnixMilli(),
		Metadata: &MetadataEntry{
			Key:   string(key),
			Value: value,
		},
	}
	return tf.appendRecords([]TranscriptRecord{record})
}

// GetFilePath returns the path to the transcript file
func (tf *TranscriptFile) GetFilePath() string {
	return tf.filePath
}

// SetMaxFileSize sets the maximum file size before rotation (in bytes)
func (tf *TranscriptFile) SetMaxFileSize(bytes int64) {
	tf.mu.Lock()
	tf.maxFileSize = bytes
	tf.mu.Unlock()
}

// MaxFileSize returns the current maximum file size before rotation
func (tf *TranscriptFile) MaxFileSize() int64 {
	tf.mu.Lock()
	defer tf.mu.Unlock()
	return tf.maxFileSize
}

// ReadAllRecords reads all transcript records from all rotated transcript files.
// Used for session resume — reconstructs the full conversation history.
func (tf *TranscriptFile) ReadAllRecords() ([]TranscriptRecord, error) {
	// Find all rotated files for this session
	matches, err := filepath.Glob(tf.filePath + "*")
	if err != nil {
		return nil, fmt.Errorf("failed to glob transcript files: %w", err)
	}

	// Filter and sort: main file first (if exists), then rotated files chronologically
	var files []string
	for _, f := range matches {
		// Include main file and rotated files (.*.jsonl pattern)
		if f == tf.filePath || strings.Contains(f, ".") && strings.HasSuffix(f, ".jsonl") {
			files = append(files, f)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		// Prefer lexicographic order which puts the main file first (no timestamp)
		// and then rotated files in chronological order (timestamp increases)
		return files[i] < files[j]
	})

	var allRecords []TranscriptRecord
	for _, file := range files {
		recs, err := readRecordsFromFile(file)
		if err != nil {
			// Skip file with warning (could log in future)
			continue
		}
		allRecords = append(allRecords, recs...)
	}

	// Merge sort by timestamp (records from different files might overlap)
	sort.Slice(allRecords, func(i, j int) bool {
		return allRecords[i].Timestamp < allRecords[j].Timestamp
	})

	return allRecords, nil
}

// readRecordsFromFile reads transcript records from a single file
func readRecordsFromFile(path string) ([]TranscriptRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []TranscriptRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		var r TranscriptRecord
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			continue // Skip malformed lines
		}
		records = append(records, r)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// ReplayStats contains statistics about a replay operation.
type ReplayStats struct {
	UserMessages      int
	AssistantMessages int
	ToolCalls         int
	ToolResults       int
	MetadataEntries   int
	SkippedRecords    int // sidechains or unsupported types
}

// ReplaySessionInfo contains the result of replaying a transcript session.
type ReplaySessionInfo struct {
	Records   []TranscriptRecord
	Stats     ReplayStats
	SessionID string
	Cwd       string
}

// Replay reads and reconstructs a full conversation from the transcript file.
// It returns all records and statistics, excluding sidechains.
func (tf *TranscriptFile) Replay() (*ReplaySessionInfo, error) {
	records, err := tf.ReadAllRecords()
	if err != nil {
		return nil, err
	}

	stats := ReplayStats{}
	var filtered []TranscriptRecord
	for _, r := range records {
		if r.IsSidechain {
			stats.SkippedRecords++
			continue
		}
		switch r.Type {
		case MessageTypeUser:
			stats.UserMessages++
		case MessageTypeAssistant:
			stats.AssistantMessages++
		case MessageTypeMetadata:
			stats.MetadataEntries++
		default:
			// Count tool operations
			if r.ToolUseID != "" {
				if r.ToolName != "" {
					stats.ToolCalls++
				} else {
					stats.ToolResults++
				}
			}
		}
		filtered = append(filtered, r)

		// Capture session-level info
		if r.SessionID != "" && r.SessionID != "meta" && r.SessionID != "metadata" {
			// will be set below
			_ = true // just tracking
		}
	}

	// Determine session ID and CWD from first non-metadata record
	sessionID := ""
	cwd := ""
	for _, r := range filtered {
		if r.Type != MessageTypeMetadata {
			sessionID = r.SessionID
			cwd = r.Cwd
			break
		}
	}
	if sessionID == "" {
		sessionID = tf.SessionIDFromPath()
	}

	return &ReplaySessionInfo{
		Records:   filtered,
		Stats:     stats,
		SessionID: sessionID,
		Cwd:       cwd,
	}, nil
}

// SessionIDFromPath extracts the session ID from the file path.
func (tf *TranscriptFile) SessionIDFromPath() string {
	base := filepath.Base(tf.filePath)
	return base[:len(base)-len(".jsonl")]
}

// checkAndRotate checks if the current file size exceeds maxFileSize and rotates if needed
func (tf *TranscriptFile) checkAndRotate(pendingRecords []TranscriptRecord) error {
	tf.mu.Lock()
	defer tf.mu.Unlock()

	if tf.isClosed {
		return fmt.Errorf("transcript file is closed")
	}

	// Estimate size of pending records (approximate)
	var pendingSizeEstimate int64
	for _, r := range pendingRecords {
		// Rough estimate: each record ~ 200 bytes base + content length
		pendingSizeEstimate += 200 + int64(len(r.Content))
	}

	// Check if rotation needed
	if tf.currentSize+pendingSizeEstimate > tf.maxFileSize {
		// Perform rotation (assumes lock is held)
		if err := tf.rotate(); err != nil {
			return err
		}
	}

	return nil
}

// rotate performs the actual rotation. Caller must hold tf.mu.
func (tf *TranscriptFile) rotate() error {
	// Generate rotated filename: session-123.jsonl -> session-123.1700000000.jsonl
	base := tf.filePath
	ext := ".jsonl"
	if strings.HasSuffix(base, ext) {
		base = base[:len(base)-len(ext)]
	}
	timestamp := time.Now().Unix()
	rotatedPath := fmt.Sprintf("%s.%d.jsonl", base, timestamp)

	// Rename current file to rotated path
	if err := os.Rename(tf.filePath, rotatedPath); err != nil {
		// If source doesn't exist, that's fine
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to rotate file: %w", err)
		}
	}

	// Reset state for new file
	tf.currentSize = 0
	tf.rotationTime = time.Now()

	// Clear the UUID deduplication map since we started a fresh file.
	// The old file is archived; any message in the new file will get a fresh UUID.
	tf.seenUUIDs = make(map[string]bool)

	return nil
}
