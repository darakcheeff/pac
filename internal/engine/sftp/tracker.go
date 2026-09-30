package sftp

import (
	"net/url"
	pathPkg "path"
	"regexp"
	"strings"
	"sync"
)

// osc7Regex matches \033]7;file://hostname/path\007 or \033]7;file://hostname/path\033\
var osc7Regex = regexp.MustCompile(`\x1b\]7;file://(?:[^/]*)(/[^\x07\x1b]*)(?:\x07|\x1b\\)`)

// osc99Regex matches ConPTY / Windows Terminal OSC 9;9 Current Working Directory: \033]9;9;"C:\path"\007
var osc99Regex = regexp.MustCompile(`\x1b\]9;9;(?:"([^"\x07\x1b]+)"|([^\x07\x1b]+))(?:\x07|\x1b\\)`)

// oscTitleRegex matches Unix OSC 0 and OSC 2 window titles set by bash/zsh prompts
// e.g. \033]0;user@host: /tmp\007 or \033]0;user@host: ~/dir\007
var oscTitleRegex = regexp.MustCompile(`\x1b\][02];(?:[^;\x07\x1b]*@)?[^;\x07\x1b]*:\s*([/~][^\x07\x1b]*)(?:\x07|\x1b\\)`)

// oscWinTitleRegex matches Windows OSC 0 and OSC 2 window titles
// e.g. \033]0;C:\Users\tech\007 or \033]0;Administrator: C:\Windows\System32\007
var oscWinTitleRegex = regexp.MustCompile(`\x1b\][02];.*?(?:^|[\s:])([A-Za-z]:[\\/][^\x07\x1b]*)(?:\x07|\x1b\\)`)

// winPromptRegex matches cmd.exe and PowerShell prompts in raw terminal output
// e.g. "C:\Users\tech>" or "PS C:\Users\tech>" or "PS C:\Users\tech> "
var winPromptRegex = regexp.MustCompile(`(?m)(?:^|[\r\n])(?:PS\s+)?([A-Za-z]:[\\/][^>\r\n\x1b\x07]*)\s*>`)

// DirectoryTracker parses terminal stream for OSC sequences and shell prompts to track remote working directory
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

// normalizePath standardizes Unix and Windows paths for SFTP client usage (forward slashes)
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}

	// Remove leading slash if path is /C:/... or /c:/... (from file:///C:/...)
	if len(p) >= 4 && p[0] == '/' && p[2] == ':' &&
		((p[1] >= 'A' && p[1] <= 'Z') || (p[1] >= 'a' && p[1] <= 'z')) {
		p = p[1:]
	}

	// Windows drive path: C:\Users or C:/Users or C:
	if len(p) >= 2 && p[1] == ':' &&
		((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) {
		drive := strings.ToUpper(string(p[0])) + ":"
		rest := p[2:]
		rest = strings.ReplaceAll(rest, "\\", "/")
		rest = strings.TrimPrefix(rest, "/")
		cleaned := pathPkg.Clean(rest)
		if cleaned == "." || cleaned == "" {
			return drive + "/"
		}
		return drive + "/" + strings.TrimPrefix(cleaned, "/")
	}

	// Unix path
	p = strings.ReplaceAll(p, "\\", "/")
	return pathPkg.Clean(p)
}

// NotifyPath handles a new directory reported by VTE signal, OSC, or prompt detection
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
		path = pathPkg.Join(home, strings.TrimPrefix(path, "~/"))
	}
	normalized := normalizePath(path)

	if normalized == "" || normalized == dt.lastPath {
		dt.mu.Unlock()
		return
	}
	dt.lastPath = normalized
	cb := dt.onUpdate
	dt.mu.Unlock()

	if cb != nil {
		cb(normalized)
	}
}

// FeedBytes inspects raw terminal stream chunks for OSC 7, ConPTY OSC 9;9, OSC 0/2 window titles,
// and Windows cmd/PowerShell prompt strings
func (dt *DirectoryTracker) FeedBytes(chunk []byte) {
	str := string(chunk)

	// 1. Check OSC 7: \x1b]7;file://...
	if strings.Contains(str, "\x1b]7;file://") {
		matches := osc7Regex.FindAllStringSubmatch(str, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				rawPath := match[1]
				decodedPath, err := url.PathUnescape(rawPath)
				if err == nil && decodedPath != "" {
					dt.NotifyPath(decodedPath)
				}
			}
		}
	}

	// 2. Check ConPTY OSC 9;9 (Windows Current Working Directory)
	if strings.Contains(str, "\x1b]9;9;") {
		matches := osc99Regex.FindAllStringSubmatch(str, -1)
		for _, match := range matches {
			var p string
			if len(match) >= 2 && match[1] != "" {
				p = match[1]
			} else if len(match) >= 3 && match[2] != "" {
				p = match[2]
			}
			if p != "" {
				dt.NotifyPath(p)
			}
		}
	}

	// 3. Check OSC 0 / OSC 2 window titles
	if strings.Contains(str, "\x1b]0;") || strings.Contains(str, "\x1b]2;") {
		// Unix title: \x1b]0;user@host: /path\x07
		if matches := oscTitleRegex.FindAllStringSubmatch(str, -1); len(matches) > 0 {
			for _, match := range matches {
				if len(match) >= 2 {
					dt.NotifyPath(match[1])
				}
			}
		}
		// Windows title: \x1b]0;C:\Users\tech\x07 or \x1b]0;Administrator: C:\path\x07
		if matches := oscWinTitleRegex.FindAllStringSubmatch(str, -1); len(matches) > 0 {
			for _, match := range matches {
				if len(match) >= 2 {
					dt.NotifyPath(match[1])
				}
			}
		}
	}

	// 4. Check Windows command prompts (cmd.exe: C:\Users> or PowerShell: PS C:\Users>)
	if matches := winPromptRegex.FindAllStringSubmatch(str, -1); len(matches) > 0 {
		lastMatch := matches[len(matches)-1]
		if len(lastMatch) >= 2 {
			dt.NotifyPath(lastMatch[1])
		}
	}
}
