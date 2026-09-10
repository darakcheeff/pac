package local

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	enginePty "github.com/darakcheeff/pac/internal/engine/pty"
	"github.com/darakcheeff/pac/internal/storage"
)

type safeBuffer struct {
	mu sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestStartLocalShellWithOutput(t *testing.T) {
	// 1. Simulate VTE PTY
	vteMaster, vteSlave, err := pty.Open()
	if err != nil {
		t.Fatalf("failed to open pty: %v", err)
	}
	defer vteMaster.Close()
	defer vteSlave.Close()

	bridge := enginePty.FromSlave(vteSlave)
	outputBuf := &safeBuffer{}

	host := &storage.Host{
		Protocol: storage.ProtoLocal,
		Host:     "/bin/bash",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sess, err := StartLocalShellWithOutput(ctx, host, bridge, outputBuf)
	if err != nil {
		t.Fatalf("StartLocalShellWithOutput failed: %v", err)
	}
	defer sess.Close()

	// Wait for prompt to appear in outputBuf
	deadline := time.Now().Add(3 * time.Second)
	foundPrompt := false
	for time.Now().Before(deadline) {
		if strings.Contains(outputBuf.String(), "$") || strings.Contains(outputBuf.String(), "#") {
			foundPrompt = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !foundPrompt {
		t.Fatalf("expected prompt in output buffer, got: %q", outputBuf.String())
	}

	// Send input through vteMaster (simulating typing in VTE terminal)
	input := []byte("echo TEST_LOCAL_ECHO_OK\n")
	_, err = vteMaster.Write(input)
	if err != nil {
		t.Fatalf("failed to write to vteMaster: %v", err)
	}

	deadline = time.Now().Add(3 * time.Second)
	foundEcho := false
	for time.Now().Before(deadline) {
		if strings.Contains(outputBuf.String(), "TEST_LOCAL_ECHO_OK") {
			foundEcho = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !foundEcho {
		t.Fatalf("expected echo in output buffer, got: %q", outputBuf.String())
	}
}
