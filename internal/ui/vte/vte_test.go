package vte

import (
	"os"
	"testing"

	"github.com/gotk3/gotk3/gtk"
)

func TestVteCreation(t *testing.T) {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("Skipping GUI test in headless environment (no DISPLAY)")
	}

	gtk.Init(nil)

	term, err := NewTerminal()
	if err != nil {
		t.Fatalf("failed to create terminal widget: %v", err)
	}

	term.SetFont("Monospace 12")
	term.ApplyColorScheme("dracula")
	term.SetScrollbackLines(5000)
	term.FeedText("Test Output\r\n")

	ok := term.SearchSetPattern("Test", false)
	if !ok {
		t.Fatalf("failed to set search pattern")
	}
}
