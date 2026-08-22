package pty

import (
	"strings"
	"testing"
)

func TestPTYOpenAndBridge(t *testing.T) {
	bridge, err := Open()
	if err != nil {
		t.Fatalf("failed to open pty: %v", err)
	}
	defer bridge.Close()

	if err := bridge.SetSize(Winsize{Rows: 40, Cols: 120}); err != nil {
		t.Fatalf("failed to set size: %v", err)
	}

	size, err := bridge.GetSize()
	if err != nil {
		t.Fatalf("failed to get size: %v", err)
	}
	if size.Rows != 40 || size.Cols != 120 {
		t.Fatalf("unexpected size: %+v", size)
	}

	// Test write through slave to master
	msg := []byte("Hello PTY Master\n")
	go func() {
		_, _ = bridge.Slave.Write(msg)
	}()

	buf := make([]byte, 64)
	n, err := bridge.Master.Read(buf)
	if err != nil {
		t.Fatalf("failed to read from master: %v", err)
	}
	received := string(buf[:n])
	if !strings.HasPrefix(received, "Hello PTY Master") {
		t.Fatalf("expected prefix 'Hello PTY Master', got %q", received)
	}
}
