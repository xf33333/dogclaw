package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DailyRotatingWriter 是一个支持按天自动轮转的日志写入器
type DailyRotatingWriter struct {
	logDir       string
	filenamePrefix string
	currentDate  string
	file         *os.File
	mu           sync.Mutex
}

// NewDailyRotatingWriter 创建一个新的按天轮转的日志写入器
func NewDailyRotatingWriter(logDir, filenamePrefix string) (*DailyRotatingWriter, error) {
	// 确保日志目录存在
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log dir: %w", err)
	}

	w := &DailyRotatingWriter{
		logDir:         logDir,
		filenamePrefix: filenamePrefix,
	}

	// 初始化打开今天的日志文件
	if err := w.rotate(); err != nil {
		return nil, err
	}

	return w, nil
}

// Write 实现 io.Writer 接口，写入日志并检查是否需要轮转
func (w *DailyRotatingWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// 检查是否需要轮转
	if w.needsRotate() {
		if err := w.rotateUnlocked(); err != nil {
			return 0, err
		}
	}

	if w.file == nil {
		return 0, fmt.Errorf("log file not opened")
	}

	return w.file.Write(p)
}

// needsRotate 检查是否需要轮转到新的日期文件
func (w *DailyRotatingWriter) needsRotate() bool {
	today := time.Now().Format("2006-01-02")
	return w.currentDate != today
}

// rotate 轮转日志文件（带锁）
func (w *DailyRotatingWriter) rotate() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.rotateUnlocked()
}

// rotateUnlocked 轮转日志文件（无锁，内部使用）
func (w *DailyRotatingWriter) rotateUnlocked() error {
	today := time.Now().Format("2006-01-02")
	
	// 如果日期没变化，不需要轮转
	if w.currentDate == today {
		return nil
	}

	// 关闭旧文件
	if w.file != nil {
		w.file.Sync()
		w.file.Close()
		w.file = nil
	}

	// 构建新文件名
	logFile := filepath.Join(w.logDir, fmt.Sprintf("%s-%s.log", w.filenamePrefix, today))

	// 打开新文件
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file %s: %w", logFile, err)
	}

	w.file = f
	w.currentDate = today

	return nil
}

// Close 关闭日志文件
func (w *DailyRotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		w.file.Sync()
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

// Sync 强制将缓冲区内容写入磁盘
func (w *DailyRotatingWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		return w.file.Sync()
	}
	return nil
}
