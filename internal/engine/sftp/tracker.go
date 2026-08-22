package sftp

import (
	"net/url"
	"regexp"
	"strings"
)

// OSC7Regex matches \033]7;file://hostname/path\007 or \033]7;file://hostname/path\033\
var osc7Regex = regexp.MustCompile(`\x1b\]7;file://([^/]*)(/[^\x07\x1b]*)(?:\x07|\x1b\\)`)

// DirectoryTracker parses terminal stream for OSC 7 sequences to track remote working directory
type DirectoryTracker struct {
	lastPath string
	onUpdate func(path string)
}

func NewDirectoryTracker(onUpdate func(path string)) *DirectoryTracker {
	return &DirectoryTracker{
		onUpdate: onUpdate,
	}
}

// FeedBytes inspects raw terminal stream chunks
func (dt *DirectoryTracker) FeedBytes(chunk []byte) {
	if !strings.Contains(string(chunk), "\x1b]7;file://") {
		return
	}

	matches := osc7Regex.FindAllStringSubmatch(string(chunk), -1)
	for _, match := range matches {
		if len(match) >= 3 {
			rawPath := match[2]
			decodedPath, err := url.PathUnescape(rawPath)
			if err == nil && decodedPath != "" && decodedPath != dt.lastPath {
				dt.lastPath = decodedPath
				if dt.onUpdate != nil {
					dt.onUpdate(decodedPath)
				}
			}
		}
	}
}
