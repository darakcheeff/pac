package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRemoteEditWatcher(t *testing.T) {
	mgr, err := NewRemoteEditManager()
	if err != nil {
		t.Fatalf("failed to create watcher mgr: %v", err)
	}
	defer mgr.Close()

	uploaded := make(chan string, 1)

	downloadFn := func(localPath string) error {
		return os.WriteFile(localPath, []byte("initial content"), 0644)
	}

	uploadFn := func(ctx context.Context, localPath, remotePath string) error {
		content, _ := os.ReadFile(localPath)
		uploaded <- string(content)
		return nil
	}

	err = mgr.OpenForEditing("test-host", "/etc/nginx/nginx.conf", downloadFn, uploadFn, "")
	if err != nil {
		t.Fatalf("OpenForEditing failed: %v", err)
	}

	// Trigger simulated edit write
	baseName := filepath.Base("/etc/nginx/nginx.conf")
	localPath := filepath.Join(mgr.tempDir, "test-host", baseName)

	time.Sleep(50 * time.Millisecond)
	_ = os.WriteFile(localPath, []byte("updated nginx config content"), 0644)

	select {
	case res := <-uploaded:
		if res != "updated nginx config content" {
			t.Fatalf("expected updated content, got %s", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for upload callback")
	}
}
