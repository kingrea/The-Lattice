package logbook

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Level represents the severity of a log entry.
type Level string

const (
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

// Logbook persists workflow progress to a simple text file.
type Logbook struct {
	path string
	mu   sync.Mutex
}

// New creates a logbook that writes to the provided path.
func New(path string) (*Logbook, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return &Logbook{path: path}, nil
}

// Path returns the file backing this logbook.
func (l *Logbook) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Append writes a single entry to the logbook.
func (l *Logbook) Append(level Level, message string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	line := fmt.Sprintf("%s %-5s %s\n",
		time.Now().UTC().Format(time.RFC3339),
		string(level),
		strings.TrimSpace(message),
	)
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.WriteString(line)
}

// Tail returns up to maxLines of the most recent log entries.
func (l *Logbook) Tail(maxLines int) []string {
	if l == nil || maxLines <= 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := os.Open(l.path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) == 0 {
		return nil
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return lines
}

// Info appends an informational entry.
func (l *Logbook) Info(format string, args ...any) {
	l.Append(LevelInfo, fmt.Sprintf(format, args...))
}

// Warn appends a warning entry.
func (l *Logbook) Warn(format string, args ...any) {
	l.Append(LevelWarn, fmt.Sprintf(format, args...))
}

// Error appends an error entry.
func (l *Logbook) Error(format string, args ...any) {
	l.Append(LevelError, fmt.Sprintf(format, args...))
}
