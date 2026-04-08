package main

import (
	"fmt"
	"os"
	"path/filepath"

	"dogclaw/pkg/transcript"
)

func main() {
	// Create temp transcript file
	tmpDir := os.TempDir()
	filePath := filepath.Join(tmpDir, "test-session.jsonl")

	// Create transcript file
	tf := transcript.NewTranscriptFile(filePath)
	defer os.RemoveAll(filePath) // Cleanup

	// Set max file size to 500 bytes for quick rotation
	tf.SetMaxFileSize(500)

	// Queue many records with large content to trigger rotation
	totalRecords := 25 // Should create ~ rotate after 2 files
	fmt.Printf("Creating %d records with 200 byte content each...\n", totalRecords)

	for i := 0; i < totalRecords; i++ {
		record := transcript.TranscriptRecord{
			Type:      "user",
			UUID:      fmt.Sprintf("msg-%d", i),
			SessionID: "test-session",
			Timestamp: 1000 + int64(i),
			Content:   fmt.Sprintf("This is a test message with 200 bytes content: %s\n", createString(200)),
		}
		tf.Queue(record)
	}

	// Flush all pending records
	if err := tf.Flush(); err != nil {
		fmt.Printf("Flush error: %v\n", err)
		return
	}

	// Check files created
	fmt.Println("\nTranscript files created:")
	matches, _ := filepath.Glob(filePath + "*")
	for _, f := range matches {
		info, _ := os.Stat(f)
		fmt.Printf("  - %s (%d bytes)\n", filepath.Base(f), info.Size())
	}

	// Test replay (read all records)
	fmt.Println("\nReplaying transcripts...")
	info, err := tf.Replay()
	if err != nil {
		fmt.Printf("Replay error: %v\n", err)
		return
	}
	fmt.Printf("Session: %s, Records: %d\n", info.SessionID, len(info.Records))
	fmt.Printf("Stats: User=%d, Assistant=%d, Metadata=%d\n",
		info.Stats.UserMessages,
		info.Stats.AssistantMessages,
		info.Stats.MetadataEntries)
}

func createString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}
