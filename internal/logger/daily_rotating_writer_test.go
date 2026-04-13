package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDailyRotatingWriter 测试按天轮转的日志写入器
func TestDailyRotatingWriter(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "logtest")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建写入器
	writer, err := NewDailyRotatingWriter(tmpDir, "test")
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer writer.Close()

	// 写入第一条日志
	msg1 := []byte("Test log 1\n")
	n, err := writer.Write(msg1)
	if err != nil {
		t.Fatalf("Failed to write log 1: %v", err)
	}
	if n != len(msg1) {
		t.Errorf("Wrote %d bytes, expected %d", n, len(msg1))
	}

	// 验证今天的文件存在
	today := time.Now().Format("2006-01-02")
	todayFile := filepath.Join(tmpDir, "test-"+today+".log")
	if _, err := os.Stat(todayFile); os.IsNotExist(err) {
		t.Errorf("Today's log file %s does not exist", todayFile)
	}

	// 检查文件内容
	content, err := os.ReadFile(todayFile)
	if err != nil {
		t.Fatalf("Failed to read today's log file: %v", err)
	}
	if string(content) != string(msg1) {
		t.Errorf("File content mismatch: got %q, expected %q", string(content), string(msg1))
	}

	t.Logf("Test passed! Today's log file created: %s", todayFile)
}

// TestDailyRotatingWriterManual 手动测试 - 不会自动执行
func TestDailyRotatingWriterManual(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping manual test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "logtest-manual")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Logf("Manual test log directory: %s", tmpDir)
	t.Logf("You can watch this directory and change your system date to test rotation")

	writer, err := NewDailyRotatingWriter(tmpDir, "manual-test")
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer writer.Close()

	// 写入一些测试日志
	for i := 0; i < 5; i++ {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		msg := []byte(timestamp + " - Test log message\n")
		_, err := writer.Write(msg)
		if err != nil {
			t.Errorf("Failed to write log: %v", err)
		}
		time.Sleep(1 * time.Second)
		t.Logf("Wrote log %d", i+1)
	}

	t.Log("Manual test complete. Check the log directory.")
}
