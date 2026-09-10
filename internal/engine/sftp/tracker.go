package sftp

import (
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// osc7Regex matches \033]7;file://hostname/path\007 or \033]7;file://hostname/path\033\
var osc7Regex = regexp.MustCompile(`\x1b\]7;file://([^/]*)(/[^\x07\x1b]*)(?:\x07|\x1b\\)`)

// oscTitleRegex matches OSC 0 and OSC 2 window titles set by bash/zsh prompts
// e.g. \033]0;user@host: /tmp\007 or \033]0;user@host: ~/dir\007
var oscTitleRegex = regexp.MustCompile(`\x1b\][02];(?:[^;\x07\x1b]*@)?[^;\x07\x1b]*:\s*([/~][^\x07\x1b]*)(?:\x07|\x1b\\)`)

// DirectoryTracker parses terminal stream for OSC sequences to track remote working directory
type DirectoryTracker struct {
	mu       sync.Mutex
	lastPath string
	homeDir  string
	onUpdate func(path string)
}

func NewDirectoryTracker(onUpdate func(path string)) *DirectoryTracker {
	return &DirectoryTracker{
		onUpdate: onUpdate,
	}
}

// SetHomeDir sets known remote/local home directory for ~ expansion
func (dt *DirectoryTracker) SetHomeDir(home string) {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	dt.homeDir = home
}

// GetLastPath returns the last detected path
func (dt *DirectoryTracker) GetLastPath() string {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	return dt.lastPath
}

// NotifyPath handles a new directory reported by VTE signal or OSC
func (dt *DirectoryTracker) NotifyPath(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}

	dt.mu.Lock()
	home := dt.homeDir
	if path == "~" && home != "" {
		path = home
	} else if strings.HasPrefix(path, "~/") && home != "" {
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	path = filepath.Clean(path)

	if path == dt.lastPath {
		dt.mu.Unlock()
		return
	}
	dt.lastPath = path
	cb := dt.onUpdate
	dt.mu.Unlock()

	if cb != nil {
		cb(path)
	}
}

// FeedBytes inspects raw terminal stream chunks for OSC 7 and OSC 0/2 sequences
func (dt *DirectoryTracker) FeedBytes(chunk []byte) {
	str := string(chunk)

	// 1. Check OSC 7
	if strings.Contains(str, "\x1b]7;file://") {
		matches := osc7Regex.FindAllStringSubmatch(str, -1)
		for _, match := range matches {
			if len(match) >= 3 {
				rawPath := match[2]
				decodedPath, err := url.PathUnescape(rawPath)
				if err == nil && decodedPath != "" {
					dt.NotifyPath(decodedPath)
				}
			}
		}
	}

	// 2. Check OSC 0 / OSC 2 window titles
	if strings.Contains(str, "\x1b]0;") || strings.Contains(str, "\x1b]2;") {
		matches := oscTitleRegex.FindAllStringSubmatch(str, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				dt.NotifyPath(match[1])
			}
		}
	}
}
