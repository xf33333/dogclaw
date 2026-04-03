// Package transcript provides session transcript persistence using JSONL format.
// It records the complete conversation history for each session, supporting
// resume, token estimation, and message chain tracking.
package transcript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
}

// NewTranscriptFile creates a new transcript file manager for the given path
func NewTranscriptFile(filePath string) *TranscriptFile {
	tf := &TranscriptFile{
		filePath:   filePath,
		writeQueue: make([]TranscriptRecord, 0, 64),
		seenUUIDs:  make(map[string]bool),
		flushDone:  make(chan struct{}),
	}

	// Ensure parent directory exists
	dir := filepath.Dir(filePath)
	os.MkdirAll(dir, 0755)

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

	f, err := os.OpenFile(tf.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open transcript file: %w", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return fmt.Errorf("failed to encode transcript record: %w", err)
		}
	}

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

// ReadMetadata reads metadata entries from the end of the transcript file.
// It uses a tail-read optimization (reads only last 64KB) for efficiency.
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

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat transcript file: %w", err)
	}

	// Tail-read optimization: only read last 64KB
	const tailReadSize = 64 * 1024
	fileSize := info.Size()
	readOffset := int64(0)
	if fileSize > tailReadSize {
		readOffset = fileSize - tailReadSize
	}

	// Seek to the beginning of the last chunk
	f.Seek(readOffset, 0)

	decoder := json.NewDecoder(f)
	var record TranscriptRecord
	for {
		if err := decoder.Decode(&record); err != nil {
			break // EOF or invalid JSON
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

// ReadAllRecords reads all transcript records from the JSONL file.
// Used for session resume — reconstructs the full conversation history.
func (tf *TranscriptFile) ReadAllRecords() ([]TranscriptRecord, error) {
	f, err := os.Open(tf.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open transcript file: %w", err)
	}
	defer f.Close()

	var records []TranscriptRecord
	scanner := bufio.NewScanner(f)
	// Increase buffer size for large content blocks
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record TranscriptRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue // Skip malformed lines
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading transcript: %w", err)
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
