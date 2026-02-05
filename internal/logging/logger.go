package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kingrea/The-Lattice/internal/config"
)

// Logger appends timestamped lines to .lattice/logs/lattice.log so users
// can inspect failures even after tmux closes the session.
type Logger struct {
	file *os.File
}

// New creates (or reuses) the log file for the current project directory.
func New(projectDir string) (*Logger, error) {
	logDir := filepath.Join(projectDir, config.LatticeDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("logging: ensure log dir: %w", err)
	}
	path := filepath.Join(logDir, "lattice.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("logging: open log file: %w", err)
	}
	return &Logger{file: f}, nil
}

// Close releases the file handle.
func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}

// Printf writes a single timestamped line to the log file.
func (l *Logger) Printf(format string, args ...any) {
	if l == nil || l.file == nil {
		return
	}
	line := fmt.Sprintf(format, args...)
	line = strings.TrimRight(line, "\n")
	timestamp := time.Now().Format(time.RFC3339)
	fmt.Fprintf(l.file, "[%s] %s\n", timestamp, line)
}
