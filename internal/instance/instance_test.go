package instance

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSingleInstanceWorkflow(t *testing.T) {
	// Override socket path via XDG_RUNTIME_DIR to a temporary directory for isolation
	tmpDir, err := os.MkdirTemp("", "pac-inst-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	origRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	defer os.Setenv("XDG_RUNTIME_DIR", origRuntimeDir)
	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	expectedSock := filepath.Join(tmpDir, "pac.sock")
	if SocketPath() != expectedSock {
		t.Fatalf("expected sock path %s, got %s", expectedSock, SocketPath())
	}

	// 1. Initially, no instance is running
	notified, err := TryNotifyExisting("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notified {
		t.Fatalf("expected false when no instance is running")
	}

	// 2. Start listener
	activateCh := make(chan struct{}, 1)
	connectCh := make(chan string, 1)

	l, err := StartListener(func() {
		activateCh <- struct{}{}
	}, func(target string) {
		connectCh <- target
	})
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer l.Close()

	// 3. Second instance notifies with ACTIVATE
	notified, err = TryNotifyExisting("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !notified {
		t.Fatalf("expected true when instance is listening")
	}

	select {
	case <-activateCh:
		// Success!
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for ACTIVATE callback")
	}

	// 4. Third instance notifies with CONNECT:MyHost
	notified, err = TryNotifyExisting("MyHost")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !notified {
		t.Fatalf("expected true when instance is listening")
	}

	select {
	case target := <-connectCh:
		if target != "MyHost" {
			t.Fatalf("expected target 'MyHost', got '%s'", target)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for CONNECT callback")
	}

	// 5. Close listener and verify cleanup
	if err := l.Close(); err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}

	if _, err := os.Stat(expectedSock); !os.IsNotExist(err) {
		t.Fatalf("expected socket file to be deleted on Close")
	}
}
