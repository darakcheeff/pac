package session

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/darakcheeff/pac/internal/storage"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\([a-zA-Z]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)

// SessionLogger logs terminal I/O to a file
type SessionLogger struct {
	file      *os.File
	cleanANSI bool
	mu        sync.Mutex
	closed    bool
}

func NewSessionLogger(host *storage.Host, defaultDir string) (*SessionLogger, error) {
	if !host.EnableLogging {
		return nil, nil
	}

	logDir := defaultDir
	if logDir == "" {
		home, _ := os.UserHomeDir()
		logDir = filepath.Join(home, "logs", "sessions")
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log dir: %w", err)
	}

	now := time.Now()
	filename := fmt.Sprintf("%s_%s.log", sanitizeFilename(host.Name), now.Format("2006-01-02_15-04-05"))
	if host.LogPathFormat != "" {
		format := host.LogPathFormat
		format = strings.ReplaceAll(format, "%hostname", sanitizeFilename(host.Name))
		format = strings.ReplaceAll(format, "%host", host.Host)
		filename = now.Format(format)
	}

	filePath := filepath.Join(logDir, filename)
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", filePath, err)
	}

	// Write header
	header := fmt.Sprintf("=== PAC Session Log: %s (%s) started at %s ===\n\n",
		host.Name, host.Host, now.Format(time.RFC3339))
	_, _ = f.WriteString(header)

	return &SessionLogger{
		file:      f,
		cleanANSI: host.LogCleanANSI,
	}, nil
}

func (l *SessionLogger) Write(p []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed || l.file == nil {
		return len(p), nil
	}

	if l.cleanANSI {
		clean := ansiRegex.ReplaceAll(p, []byte(""))
		_, err = l.file.Write(clean)
	} else {
		_, err = l.file.Write(p)
	}

	return len(p), err
}

func (l *SessionLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed || l.file == nil {
		return nil
	}
	l.closed = true

	footer := fmt.Sprintf("\n=== PAC Session Log ended at %s ===\n", time.Now().Format(time.RFC3339))
	_, _ = l.file.WriteString(footer)
	return l.file.Close()
}

func sanitizeFilename(name string) string {
	invalid := regexp.MustCompile(`[<>:"/\\|?*]`)
	clean := invalid.ReplaceAllString(name, "_")
	return strings.ReplaceAll(clean, " ", "_")
}
