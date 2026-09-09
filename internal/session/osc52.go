package session

import (
	"bytes"
	"encoding/base64"
	"strings"
	"sync"
)

// OSC52Parser detects and parses OSC 52 ANSI clipboard escape sequences in terminal byte streams.
// Format: \x1b]52;<target>;<base64_payload>[\x07|\x1b\\]
type OSC52Parser struct {
	buf      []byte
	inSeq    bool
	onCopy   func(target string, text string)
	mu       sync.Mutex
}

func NewOSC52Parser(onCopy func(target string, text string)) *OSC52Parser {
	return &OSC52Parser{
		onCopy: onCopy,
	}
}

// Feed processes incoming byte chunks and triggers onCopy if OSC 52 sequence is completed
func (p *OSC52Parser) Feed(data []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.buf = append(p.buf, data...)
	if len(p.buf) > 1024*1024 { // Cap buffer at 1MB to prevent memory leak on malformed streams
		p.buf = p.buf[len(p.buf)-64*1024:]
	}

	for {
		// Look for start of OSC 52: \x1b]52;
		startIdx := bytes.Index(p.buf, []byte("\x1b]52;"))
		if startIdx == -1 {
			// Also support 8-bit OSC: 0x9D 52;
			startIdx = bytes.Index(p.buf, []byte("\x9d52;"))
			if startIdx == -1 {
				// Keep only the last 5 bytes in case start sequence is split across chunks
				if len(p.buf) > 5 {
					p.buf = p.buf[len(p.buf)-5:]
				}
				break
			}
			startIdx += 4
		} else {
			startIdx += 5
		}

		// Look for terminator: \x07 (BEL) or \x1b\\ (ST)
		tail := p.buf[startIdx:]
		endIdx := -1
		termLen := 0

		belIdx := bytes.IndexByte(tail, 0x07)
		stIdx := bytes.Index(tail, []byte("\x1b\\"))

		if belIdx != -1 && (stIdx == -1 || belIdx < stIdx) {
			endIdx = belIdx
			termLen = 1
		} else if stIdx != -1 {
			endIdx = stIdx
			termLen = 2
		}

		if endIdx == -1 {
			// Sequence not completed yet, wait for next chunk
			if startIdx > 5 {
				p.buf = p.buf[startIdx-5:]
			}
			break
		}

		payload := string(tail[:endIdx])
		p.buf = tail[endIdx+termLen:]

		// Parse target and base64: <target>;<base64>
		semi := strings.IndexByte(payload, ';')
		var target, b64 string
		if semi != -1 {
			target = payload[:semi]
			b64 = payload[semi+1:]
		} else {
			target = "c"
			b64 = payload
		}

		b64 = strings.TrimSpace(b64)
		if b64 == "" || b64 == "?" {
			// Query or empty, skip
			continue
		}

		// Decode base64
		decoded, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(b64)
		}
		if err == nil && len(decoded) > 0 {
			if p.onCopy != nil {
				p.onCopy(target, string(decoded))
			}
		}
	}
}
