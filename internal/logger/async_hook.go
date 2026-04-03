package logger

import (
	"bufio"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// AsyncHook 是一个异步日志钩子，将日志序列化后由后台 goroutine 批量写入文件
type AsyncHook struct {
	// 通道大小，缓冲序列化后的日志字节
	bufferSize int
	// 刷新间隔
	flushInterval time.Duration
	// 是否启用
	enabled bool
	// 文件路径
	logFile string
	// 文件句柄
	file *os.File
	// 写入器
	writer *bufio.Writer
	// 通道用于接收序列化的日志字节
	entryChan chan []byte
	// 停止通道
	stopChan chan struct{}
	// 等待组
	wg sync.WaitGroup
	// 互斥锁
	mu sync.Mutex
	// 统计
	totalEntries   int
	droppedEntries int
	// formatter 用于格式化日志条目
	formatter logrus.Formatter
}

// NewAsyncHook 创建异步日志钩子
func NewAsyncHook(logFile string, bufferSize int, flushInterval time.Duration, formatter logrus.Formatter) (*AsyncHook, error) {
	// 确保日志目录存在
	logDir := filepath.Dir(logFile)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	// 打开日志文件（追加模式）
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	hook := &AsyncHook{
		bufferSize:    bufferSize,
		flushInterval: flushInterval,
		enabled:       true,
		logFile:       logFile,
		file:          file,
		writer:        bufio.NewWriter(file),
		entryChan:     make(chan []byte, bufferSize),
		stopChan:      make(chan struct{}),
		formatter:     formatter,
	}

	// 启动后台写入 goroutine
	hook.wg.Add(1)
	go hook.writerWorker()

	return hook, nil
}

// Levels 返回此钩子处理的日志级别（所有级别）
func (h *AsyncHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire 将日志条目序列化后发送到通道（非阻塞，如果通道满则丢弃）
func (h *AsyncHook) Fire(entry *logrus.Entry) error {
	if !h.enabled {
		return nil
	}

	// 使用 formatter 序列化 entry
	bytes, err := h.formatter.Format(entry)
	if err != nil {
		return err
	}

	// 尝试非阻塞发送
	select {
	case h.entryChan <- bytes:
		return nil
	default:
		// 通道已满，丢弃该日志条目并计数
		h.mu.Lock()
		h.droppedEntries++
		h.mu.Unlock()
		return nil
	}
}

// writerWorker 后台工作函数，从通道读取并写入文件
func (h *AsyncHook) writerWorker() {
	defer h.wg.Done()
	defer h.file.Close()

	ticker := time.NewTicker(h.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case bytes, ok := <-h.entryChan:
			if !ok {
				// 通道关闭，刷新剩余内容后退出
				h.writer.Flush()
				return
			}
			// 直接写入 buffer
			_, err := h.writer.Write(bytes)
			if err != nil {
				// 写入失败，尝试重新打开文件
				h.reopenFile()
				// 重试写入一次
				h.writer.Write(bytes)
			}
			h.totalEntries++

		case <-ticker.C:
			// 定期刷新 buffer
			h.writer.Flush()

		case <-h.stopChan:
			// 收到停止信号，刷新并退出
			h.writer.Flush()
			return
		}
	}
}

// reopenFile 重新打开日志文件（当文件出错时调用）
func (h *AsyncHook) reopenFile() {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 关闭旧文件
	if h.file != nil {
		h.file.Close()
	}

	// 重新打开
	file, err := os.OpenFile(h.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// 如果还是失败，将输出重定向到 stderr
		h.file = nil
		h.writer = bufio.NewWriter(os.Stderr)
		return
	}

	h.file = file
	h.writer = bufio.NewWriter(file)
}

// Flush 强制刷新 buffer 到磁盘
func (h *AsyncHook) Flush() {
	h.writer.Flush()
}

// Stop 停止异步写入，等待所有条目写入完成
func (h *AsyncHook) Stop() {
	close(h.stopChan)
	h.wg.Wait()
	h.Flush()
	if h.file != nil {
		h.file.Close()
	}
}

// Stats 返回统计信息
func (h *AsyncHook) Stats() (total, dropped int) {
	h.mu.Lock()
	total = h.totalEntries
	dropped = h.droppedEntries
	h.mu.Unlock()
	return
}

// SetEnabled 启用或禁用异步写入
func (h *AsyncHook) SetEnabled(enabled bool) {
	h.enabled = enabled
}
