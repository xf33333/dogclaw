package logger

import (
	"os"

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

// InitLogger initializes the global logger with standard configuration.
// Should be called once at startup.
func InitLogger() {
	// Output to stderr, use text format with timestamp
	global.Out = os.Stderr
	global.Formatter = &logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
		DisableColors:   false,
	}

	// Enable caller info (file:line)
	global.SetReportCaller(true)

	// Read LOG_LEVEL env var to set level (default: INFO)
	if lvl, err := logrus.ParseLevel(os.Getenv("LOG_LEVEL")); err == nil {
		global.SetLevel(lvl)
	} else {
		global.SetLevel(INFO)
	}
}

func init() {
	InitLogger()
}
