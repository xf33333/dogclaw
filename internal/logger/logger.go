package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Level represents a log severity level (mapped to logrus levels)
type Level = logrus.Level

const (
	DEBUG = logrus.DebugLevel
	INFO  = logrus.InfoLevel
	WARN  = logrus.WarnLevel
	ERROR = logrus.ErrorLevel
)

// Global logger instance.
// logrus is already thread-safe, so no mutex needed.
var global = logrus.New()

// SetLevel sets the minimum log level for the global logger.
// Messages below this level are silently dropped.
func SetLevel(l Level) {
	global.SetLevel(l)
}

// GetLevel returns the current log level.
func GetLevel() Level {
	return global.Level
}

// Debugf logs at DEBUG level.
func Debug(format string, args ...any) {
	global.Debugf(format, args...)
}

// Infof logs at INFO level.
func Info(format string, args ...any) {
	global.Infof(format, args...)
}

// Warnf logs at WARN level.
func Warn(format string, args ...any) {
	global.Warnf(format, args...)
}

// Errorf logs at ERROR level.
func Error(format string, args ...any) {
	global.Errorf(format, args...)
}

// Debug logs at DEBUG level (printf-style).
// Alias for Debugf for backwards compatibility.
var Debugf = Debug

// Info logs at INFO level (printf-style).
// Alias for Infof for backwards compatibility.
var Infof = Info

// Warn logs at WARN level (printf-style).
// Alias for Warnf for backwards compatibility.
var Warnf = Warn

// Error logs at ERROR level (printf-style).
// Alias for Errorf for backwards compatibility.
var Errorf = Error

// WithField returns a logger with a single field.
func WithField(key string, value any) *logrus.Entry {
	return global.WithField(key, value)
}

// WithFields returns a logger with multiple fields.
func WithFields(fields logrus.Fields) *logrus.Entry {
	return global.WithFields(fields)
}

// CustomFormatter formats logs as: LEVEL-YYYY-MM-DD HH:MM:SS-file:line message
type CustomFormatter struct{}

func (f *CustomFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	// Level: INFO/DEBUG/WARN/ERROR
	level := strings.ToUpper(entry.Level.String())

	// Timestamp: 2006-01-02 15:04:05
	timestamp := entry.Time.Format("2006-01-02 15:04:05")

	// Caller info: file:line
	caller := ""
	if entry.HasCaller() {
		// Extract just filename and line
		file := filepath.Base(entry.Caller.File)
		caller = fmt.Sprintf("%s:%d", file, entry.Caller.Line)
	} else {
		caller = "unknown"
	}

	// Message
	msg := entry.Message
	if entry.Context != nil {
		msg = fmt.Sprintf("%s %v", entry.Message, entry.Context)
	}

	// Format: LEVEL-TIMESTAMP-CALLER-LINE-MESSAGE
	line := fmt.Sprintf("%s-%s-%s-%s\n", level, timestamp, caller, msg)
	return []byte(line), nil
}

// Config holds logger configuration
type Config struct {
	// Log level (default: INFO)
	Level Level
	// Output directory for log files (default: "./logs")
	LogDir string
	// Whether to output to stderr as well (default: false)
	OutputToStderr bool
	// Custom formatter (if nil, uses standard text formatter; if CustomFormatter desired, set CustomFormatter to true)
	UseCustomFormatter bool
	// Whether to enable caller info (default: true)
	EnableCaller bool
	// AsyncHook configuration
	AsyncEnabled      bool
	AsyncBufferSize   int
	AsyncFlushSeconds time.Duration
	// Prefix for log filename (default: "dogclaw")
	FilenamePrefix string
	// Whether to rotate logs daily (default: true)
	DailyRotate bool
	// Hook to attach (e.g., AsyncHook)
	Hook logrus.Hook
}

// DefaultConfig returns the default logger configuration
func DefaultConfig() *Config {
	return &Config{
		Level:              INFO,
		LogDir:             "./logs",
		OutputToStderr:     false,
		UseCustomFormatter: true,
		EnableCaller:       true,
		AsyncEnabled:       false,
		AsyncBufferSize:    1000,
		AsyncFlushSeconds:  5,
		FilenamePrefix:     "dogclaw",
		DailyRotate:        true,
	}
}

// NewLogger creates a new logrus.Logger with the given configuration
func NewLogger(cfg *Config) *logrus.Logger {
	logger := logrus.New()

	// Set level
	if cfg.Level != 0 {
		logger.SetLevel(cfg.Level)
	} else {
		logger.SetLevel(INFO)
	}

	// Set caller info
	if cfg.EnableCaller {
		logger.SetReportCaller(true)
	}

	// Set formatter
	if cfg.UseCustomFormatter {
		logger.SetFormatter(&CustomFormatter{})
	} else {
		// Default text formatter with caller info support
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
			DisableColors:   true,
		})
	}

	// Attach hook if provided
	if cfg.Hook != nil {
		logger.AddHook(cfg.Hook)
	}

	// Determine output: file or stderr
	output := os.Stderr
	if cfg.LogDir != "" && !cfg.OutputToStderr {
		if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
			logger.Warnf("Failed to create log dir %s: %v, using stderr", cfg.LogDir, err)
		} else {
			if cfg.DailyRotate {
				// 使用按天轮转的日志写入器
				writer, err := NewDailyRotatingWriter(cfg.LogDir, cfg.FilenamePrefix)
				if err != nil {
					logger.Warnf("Failed to create daily rotating log writer: %v, using stderr", err)
				} else {
					logger.SetOutput(writer)
					// 注意：不要设置 output，因为我们已经通过 SetOutput 设置了 writer
					return logger
				}
			} else {
				// 不使用按天轮转，直接打开单个文件
				logFile := filepath.Join(cfg.LogDir, cfg.FilenamePrefix+".log")
				f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
				if err != nil {
					logger.Warnf("Failed to open log file %s: %v, using stderr", logFile, err)
				} else {
					output = f
				}
			}
		}
	}

	// Set output (we may want to tee to both file and stderr in the future)
	if output != nil {
		logger.SetOutput(output)
	}

	return logger
}

// InitLogger initializes the global logger with standard configuration.
// Should be called once at startup.
func InitLogger() {
	cfg := DefaultConfig()

	// Read environment variables
	if lvl, err := logrus.ParseLevel(os.Getenv("LOG_LEVEL")); err == nil {
		cfg.Level = lvl
	}

	if logFile := os.Getenv("GOCLAUDE_LOG_FILE"); logFile != "" {
		cfg.LogDir = filepath.Dir(logFile)
		cfg.DailyRotate = false
		// Custom filename would need additional logic, but for now we'll keep default rotation
		cfg.OutputToStderr = false
	}

	global = NewLogger(cfg)
}

// InitLoggerWithConfig initializes the global logger with a specific config
func InitLoggerWithConfig(cfg *Config) {
	global = NewLogger(cfg)
}

func init() {
	InitLogger()
}

// CreateDailyRotatingLogger creates a logger with daily rotation and optional async hook
// This is a convenience function for the common use case (QueryEngine style)
func CreateDailyRotatingLogger(cwd string, useCustomFormatter bool) *logrus.Logger {
	// Normalize cwd for sanitized log dir name (similar to QueryEngine)
	safeName := sanitizePathForDir(cwd)
	logsDir := filepath.Join(cwd, "logs", safeName)

	cfg := DefaultConfig()
	cfg.LogDir = logsDir
	cfg.UseCustomFormatter = useCustomFormatter
	cfg.EnableCaller = true
	cfg.DailyRotate = true
	cfg.FilenamePrefix = "dogclaw"
	cfg.Level = INFO // will be overridden by env

	// Parse level from env
	if lvl, err := logrus.ParseLevel(os.Getenv("LOG_LEVEL")); err == nil {
		cfg.Level = lvl
	}

	return NewLogger(cfg)
}

// sanitizePathForDir converts a path to a safe directory name
// e.g., "/Users/name/project" -> "Users_name_project"
func sanitizePathForDir(path string) string {
	if path == "" {
		return "no-cwd"
	}

	// Replace path separators with underscores
	safe := strings.ReplaceAll(path, string(filepath.Separator), "_")
	safe = strings.ReplaceAll(safe, "/", "_")
	safe = strings.ReplaceAll(safe, "\\", "_")

	// Replace Windows drive colon (for ~/C_... pattern)
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

// SetGlobalLogger replaces the global logger (for testing or special setups)
func SetGlobalLogger(logger *logrus.Logger) {
	global = logger
}

// GetGlobalLogger returns the current global logger
func GetGlobalLogger() *logrus.Logger {
	return global
}
