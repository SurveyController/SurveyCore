package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
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

var levelColors = map[Level]string{
	LevelDebug: "\033[90m",
	LevelInfo:  "\033[32m",
	LevelWarn:  "\033[33m",
	LevelError: "\033[31m",
}

const (
	colorCyan  = "\033[36m"
	colorReset = "\033[0m"
)

// Field is one structured log parameter.
type Field struct {
	Key   string
	Value any
}

// F creates one structured log field.
func F(key string, value any) Field {
	return Field{Key: key, Value: value}
}

// Logger is a simple structured logger.
type Logger struct {
	mu     sync.Mutex
	level  Level
	logger *log.Logger
	prefix string
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
	l.logFields(level, fmt.Sprintf(format, args...))
}

func colorEnabled() bool {
	return os.Getenv("NO_COLOR") == ""
}

func levelLabel(level Level) string {
	label := fmt.Sprintf("[%s]", levelNames[level])
	if !colorEnabled() {
		return label
	}
	return levelColors[level] + label + colorReset
}

func formatFields(fields []Field) string {
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.Key == "" {
			continue
		}
		key := field.Key + "="
		if colorEnabled() {
			key = colorCyan + key + colorReset
		}
		parts = append(parts, fmt.Sprintf("%s%v", key, field.Value))
	}
	return strings.Join(parts, " ")
}

func (l *Logger) logFields(level Level, msg string, fields ...Field) {
	if level < l.level {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	ts := time.Now().Format("2006-01-02 15:04:05")
	parts := []string{ts, levelLabel(level)}
	if l.prefix != "" {
		parts = append(parts, "component="+l.prefix)
	}
	if formatted := formatFields(fields); formatted != "" {
		parts = append(parts, formatted)
	}
	if msg != "" {
		key := "msg="
		if colorEnabled() {
			key = colorCyan + key + colorReset
		}
		parts = append(parts, key+msg)
	}
	l.logger.Print(strings.Join(parts, " "))
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

// Log logs a message with structured fields.
func (l *Logger) Log(level Level, msg string, fields ...Field) {
	l.logFields(level, msg, fields...)
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

// SetOutput changes the default logger output, mostly for tests.
func SetOutput(output io.Writer) {
	defaultLogger.logger.SetOutput(output)
}

// Debug logs a debug message using the default logger.
func Debug(format string, args ...any) { defaultLogger.Debug(format, args...) }

// Info logs an info message using the default logger.
func Info(format string, args ...any) { defaultLogger.Info(format, args...) }

// Warn logs a warning message using the default logger.
func Warn(format string, args ...any) { defaultLogger.Warn(format, args...) }

// Error logs an error message using the default logger.
func Error(format string, args ...any) { defaultLogger.Error(format, args...) }

// Log logs a structured message using the default logger.
func Log(level Level, msg string, fields ...Field) { defaultLogger.Log(level, msg, fields...) }

// DebugFields logs a debug message with fields.
func DebugFields(msg string, fields ...Field) { defaultLogger.Log(LevelDebug, msg, fields...) }

// InfoFields logs an info message with fields.
func InfoFields(msg string, fields ...Field) { defaultLogger.Log(LevelInfo, msg, fields...) }

// WarnFields logs a warning message with fields.
func WarnFields(msg string, fields ...Field) { defaultLogger.Log(LevelWarn, msg, fields...) }

// ErrorFields logs an error message with fields.
func ErrorFields(msg string, fields ...Field) { defaultLogger.Log(LevelError, msg, fields...) }
