package session

import (
	"testing"
)

func TestOSC52Parser(t *testing.T) {
	var copiedTarget, copiedText string
	parser := NewOSC52Parser(func(target, text string) {
		copiedTarget = target
		copiedText = text
	})

	// Test 1: Standard Bel-terminated OSC 52
	// "Hello OSC 52" base64 is "SGVsbG8gT1NDIDUy"
	parser.Feed([]byte("\x1b]52;c;SGVsbG8gT1NDIDUy\x07"))
	if copiedTarget != "c" || copiedText != "Hello OSC 52" {
		t.Fatalf("Test 1 failed: target=%q text=%q", copiedTarget, copiedText)
	}

	// Test 2: ST-terminated OSC 52 (\x1b\\)
	// "Zellij Copied" base64 is "WmVsbGlqIENvcGllZA=="
	copiedTarget, copiedText = "", ""
	parser.Feed([]byte("prefix text \x1b]52;p;WmVsbGlqIENvcGllZA==\x1b\\ suffix text"))
	if copiedTarget != "p" || copiedText != "Zellij Copied" {
		t.Fatalf("Test 2 failed: target=%q text=%q", copiedTarget, copiedText)
	}

	// Test 3: Chunked stream
	copiedTarget, copiedText = "", ""
	parser.Feed([]byte("some data \x1b]52;c;T3Blb"))
	parser.Feed([]byte("kNvZGUgQ29waWVk\x07 extra data")) // "T3BlbkNvZGUgQ29waWVk" -> "OpenCode Copied"
	if copiedTarget != "c" || copiedText != "OpenCode Copied" {
		t.Fatalf("Test 3 failed: target=%q text=%q", copiedTarget, copiedText)
	}
}
