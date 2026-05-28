package logging

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// Level represents the log level.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

var levelNames = map[Level]string{
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO",
	LevelWarn:  "WARN",
	LevelError: "ERROR",
}

// Logger is a simple structured logger.
type Logger struct {
	mu      sync.Mutex
	level   Level
	logger  *log.Logger
	prefix  string
}

// New creates a new logger with the given level and prefix.
func New(level Level, prefix string) *Logger {
	return &Logger{
		level:  level,
		logger: log.New(os.Stderr, "", 0),
		prefix: prefix,
	}
}

func (l *Logger) log(level Level, format string, args ...any) {
	if level < l.level {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	ts := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	prefix := ""
	if l.prefix != "" {
		prefix = fmt.Sprintf("[%s] ", l.prefix)
	}
	l.logger.Printf("%s [%s] %s%s", ts, levelNames[level], prefix, msg)
}

// Debug logs a debug message.
func (l *Logger) Debug(format string, args ...any) {
	l.log(LevelDebug, format, args...)
}

// Info logs an info message.
func (l *Logger) Info(format string, args ...any) {
	l.log(LevelInfo, format, args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(format string, args ...any) {
	l.log(LevelWarn, format, args...)
}

// Error logs an error message.
func (l *Logger) Error(format string, args ...any) {
	l.log(LevelError, format, args...)
}

// Default logger
var defaultLogger = New(LevelInfo, "")

// SetLevel sets the default logger's level.
func SetLevel(level Level) {
	defaultLogger.level = level
}

// SetPrefix sets the default logger's prefix.
func SetPrefix(prefix string) {
	defaultLogger.prefix = prefix
}

// Debug logs a debug message using the default logger.
func Debug(format string, args ...any) { defaultLogger.Debug(format, args...) }

// Info logs an info message using the default logger.
func Info(format string, args ...any) { defaultLogger.Info(format, args...) }

// Warn logs a warning message using the default logger.
func Warn(format string, args ...any) { defaultLogger.Warn(format, args...) }

// Error logs an error message using the default logger.
func Error(format string, args ...any) { defaultLogger.Error(format, args...) }
