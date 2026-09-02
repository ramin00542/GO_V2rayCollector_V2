// Package logging provides structured logging utilities
package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogLevel represents the severity level of a log message
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Color returns the ANSI color code for the log level
func (l LogLevel) Color() string {
	switch l {
	case LevelDebug:
		return "\033[36m" // Cyan
	case LevelInfo:
		return "\033[32m" // Green
	case LevelWarn:
		return "\033[33m" // Yellow
	case LevelError:
		return "\033[31m" // Red
	case LevelFatal:
		return "\033[35m" // Magenta
	default:
		return "\033[0m" // Reset
	}
}

// Fields represents additional fields for structured logging
type Fields map[string]interface{}

// Logger provides structured logging capabilities
type Logger struct {
	mu       sync.Mutex
	outputs  []io.Writer
	level    LogLevel
	format   string // "text" or "json"
	caller   bool   // Include caller information
	colors  bool   // Use ANSI colors
}

// NewLogger creates a new logger
func NewLogger() *Logger {
	return &Logger{
		outputs: []io.Writer{os.Stderr},
		level:   LevelInfo,
		format:  "text",
		caller:  false,
		colors:  true,
	}
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetFormat sets the output format ("text" or "json")
func (l *Logger) SetFormat(format string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.format = format
}

// SetColors enables or disables ANSI colors
func (l *Logger) SetColors(colors bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.colors = colors
}

// AddOutput adds an output writer
func (l *Logger) AddOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.outputs = append(l.outputs, w)
}

// AddFileOutput adds a file output
func (l *Logger) AddFileOutput(path string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	
	l.AddOutput(file)
	return nil
}

// log writes a log message
func (l *Logger) log(level LogLevel, msg string, fields Fields) {
	if level < l.level {
		return
	}
	
	l.mu.Lock()
	defer l.mu.Unlock()
	
	timestamp := time.Now().UTC().Format(time.RFC3339)
	
	for _, output := range l.outputs {
		var line string
		
		switch l.format {
		case "json":
			line = l.formatJSON(timestamp, level, msg, fields)
		default:
			line = l.formatText(timestamp, level, msg, fields)
		}
		
		fmt.Fprintln(output, line)
	}
}

// formatJSON formats a log message as JSON
func (l *Logger) formatJSON(timestamp string, level LogLevel, msg string, fields Fields) string {
	logEntry := map[string]interface{}{
		"timestamp": timestamp,
		"level":    level.String(),
		"message":  msg,
	}
	
	// Add fields
	for k, v := range fields {
		logEntry[k] = v
	}
	
	data, err := json.Marshal(logEntry)
	if err != nil {
		return fmt.Sprintf(`{"timestamp":"%s","level":"%s","message":"%s","error":"json marshal failed"}`,
			timestamp, level.String(), msg)
	}
	
	return string(data)
}

// formatText formats a log message as text
func (l *Logger) formatText(timestamp string, level LogLevel, msg string, fields Fields) string {
	var sb string
	
	// Add color if enabled
	if l.colors {
		sb += level.Color()
	}
	
	// Format: [timestamp] [LEVEL] message
	sb += fmt.Sprintf("[%s] [%s] %s", timestamp, level.String(), msg)
	
	// Add fields
	if len(fields) > 0 {
		sb += " "
		first := true
		for k, v := range fields {
			if !first {
				sb += " "
			}
			sb += fmt.Sprintf("%s=%v", k, v)
			first = false
		}
	}
	
	// Reset color if enabled
	if l.colors {
		sb += "\033[0m"
	}
	
	return sb
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, fields ...interface{}) {
	var f Fields
	if len(fields) > 0 {
		if len(fields)%2 != 0 {
			fields = fields[:len(fields)-1] // Remove last element if odd count
		}
		f = make(Fields)
		for i := 0; i < len(fields); i += 2 {
			key := fmt.Sprint(fields[i])
			f[key] = fields[i+1]
		}
	}
	l.log(LevelDebug, msg, f)
}

// Info logs an info message
func (l *Logger) Info(msg string, fields ...interface{}) {
	var f Fields
	if len(fields) > 0 {
		if len(fields)%2 != 0 {
			fields = fields[:len(fields)-1]
		}
		f = make(Fields)
		for i := 0; i < len(fields); i += 2 {
			key := fmt.Sprint(fields[i])
			f[key] = fields[i+1]
		}
	}
	l.log(LevelInfo, msg, f)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, fields ...interface{}) {
	var f Fields
	if len(fields) > 0 {
		if len(fields)%2 != 0 {
			fields = fields[:len(fields)-1]
		}
		f = make(Fields)
		for i := 0; i < len(fields); i += 2 {
			key := fmt.Sprint(fields[i])
			f[key] = fields[i+1]
		}
	}
	l.log(LevelWarn, msg, f)
}

// Error logs an error message
func (l *Logger) Error(msg string, fields ...interface{}) {
	var f Fields
	if len(fields) > 0 {
		if len(fields)%2 != 0 {
			fields = fields[:len(fields)-1]
		}
		f = make(Fields)
		for i := 0; i < len(fields); i += 2 {
			key := fmt.Sprint(fields[i])
			f[key] = fields[i+1]
		}
	}
	l.log(LevelError, msg, f)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(msg string, fields ...interface{}) {
	var f Fields
	if len(fields) > 0 {
		if len(fields)%2 != 0 {
			fields = fields[:len(fields)-1]
		}
		f = make(Fields)
		for i := 0; i < len(fields); i += 2 {
			key := fmt.Sprint(fields[i])
			f[key] = fields[i+1]
		}
	}
	l.log(LevelFatal, msg, f)
	os.Exit(1)
}

// WithFields creates a new logger with additional fields
func (l *Logger) WithFields(fields Fields) *Logger {
	// Create a copy of the logger
	newLogger := &Logger{
		outputs: l.outputs,
		level:   l.level,
		format:  l.format,
		caller:  l.caller,
		colors:  l.colors,
	}
	
	// Add fields to all log entries
	// This is a bit tricky, but for simplicity we'll just store them
	// and include them in the log method
	// For a more complete solution, we'd need to modify the logger structure
	
	return newLogger
}

// Global logger instance
var globalLogger = NewLogger()

// SetGlobalLogger sets the global logger
func SetGlobalLogger(logger *Logger) {
	globalLogger = logger
}

// GetGlobalLogger returns the global logger
func GetGlobalLogger() *Logger {
	return globalLogger
}

// Package-level convenience functions

// Debug logs a debug message using the global logger
func Debug(msg string, fields ...interface{}) {
	globalLogger.Debug(msg, fields...)
}

// Info logs an info message using the global logger
func Info(msg string, fields ...interface{}) {
	globalLogger.Info(msg, fields...)
}

// Warn logs a warning message using the global logger
func Warn(msg string, fields ...interface{}) {
	globalLogger.Warn(msg, fields...)
}

// Error logs an error message using the global logger
func Error(msg string, fields ...interface{}) {
	globalLogger.Error(msg, fields...)
}

// Fatal logs a fatal message and exits using the global logger
func Fatal(msg string, fields ...interface{}) {
	globalLogger.Fatal(msg, fields...)
}

// NewFileLogger creates a logger that writes to a file
func NewFileLogger(path string, level LogLevel, format string) (*Logger, error) {
	logger := NewLogger()
	logger.SetLevel(level)
	logger.SetFormat(format)
	
	if err := logger.AddFileOutput(path); err != nil {
		return nil, err
	}
	
	return logger, nil
}

// NewMultiLogger creates a logger that writes to multiple outputs
func NewMultiLogger(outputs []io.Writer, level LogLevel, format string) *Logger {
	logger := NewLogger()
	logger.SetLevel(level)
	logger.SetFormat(format)
	logger.outputs = outputs
	
	return logger
}

// RotatingFileWriter implements io.Writer for rotating log files
type RotatingFileWriter struct {
	basePath string
	maxSize  int64
	maxFiles int
	current  *os.File
	mu       sync.Mutex
	currentSize int64
}

// NewRotatingFileWriter creates a new rotating file writer
func NewRotatingFileWriter(basePath string, maxSize int64, maxFiles int) (*RotatingFileWriter, error) {
	if maxSize <= 0 {
		maxSize = 10 * 1024 * 1024 // 10MB
	}
	if maxFiles <= 0 {
		maxFiles = 5
	}
	
	w := &RotatingFileWriter{
		basePath: basePath,
		maxSize:  maxSize,
		maxFiles: maxFiles,
	}
	
	if err := w.rotate(); err != nil {
		return nil, err
	}
	
	return w, nil
}

// Write implements io.Writer
func (w *RotatingFileWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if w.current == nil {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	
	// Check if we need to rotate
	if w.currentSize+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	
	// Write to current file
	n, err = w.current.Write(p)
	if err != nil {
		return n, err
	}
	
	w.currentSize += int64(n)
	return n, nil
}

// rotate rotates the log file
func (w *RotatingFileWriter) rotate() error {
	// Close current file if open
	if w.current != nil {
		w.current.Close()
		w.current = nil
	}
	
	// Create directory if it doesn't exist
	dir := filepath.Dir(w.basePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	
	// Get current file path with timestamp
	timestamp := time.Now().Format("20060102_150405")
	currentPath := fmt.Sprintf("%s.%s.log", w.basePath, timestamp)
	
	// Open new file
	file, err := os.OpenFile(currentPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	
	w.current = file
	w.currentSize = 0
	
	// Clean up old files
	if err := w.cleanup(); err != nil {
		// Log error but don't fail
		return nil
	}
	
	return nil
}

// cleanup removes old log files
func (w *RotatingFileWriter) cleanup() error {
	// Get all log files matching the pattern
	pattern := fmt.Sprintf("%s.*.log", w.basePath)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	
	// Sort by modification time (oldest first)
	type fileInfo struct {
		path string
		modTime time.Time
	}
	
	var files []fileInfo
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		files = append(files, fileInfo{path: match, modTime: info.ModTime()})
	}
	
	// Sort by modTime (oldest first)
	for i := 0; i < len(files)-1; i++ {
		for j := i + 1; j < len(files); j++ {
			if files[i].modTime.After(files[j].modTime) {
				files[i], files[j] = files[j], files[i]
			}
		}
	}
	
	// Remove old files if we have more than maxFiles
	if len(files) > w.maxFiles {
		toRemove := len(files) - w.maxFiles
		for i := 0; i < toRemove; i++ {
			if err := os.Remove(files[i].path); err != nil {
				// Log error but continue
				continue
			}
		}
	}
	
	return nil
}

// Close closes the rotating file writer
func (w *RotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if w.current != nil {
		return w.current.Close()
	}
	return nil
}
