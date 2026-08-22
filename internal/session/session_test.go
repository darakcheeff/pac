package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darakcheeff/pac/internal/storage"
)

func TestSessionManagerAndGlobalSearch(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "pac-sess-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	store, _ := storage.NewStore(filepath.Join(tempDir, "test.db"))
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.CloseAll()

	h1 := &storage.Host{ID: "h1", Name: "Gateway 1", Protocol: storage.ProtoLocal}
	h2 := &storage.Host{ID: "h2", Name: "DB Server", Protocol: storage.ProtoLocal}

	s1 := &Session{
		ID:        "s1",
		Host:      h1,
		Title:     "Gateway 1",
		StartedAt: time.Now(),
	}
	s1.appendScrollback([]byte("systemd[1]: Starting Nginx Service...\nNginx started successfully\n"))

	s2 := &Session{
		ID:        "s2",
		Host:      h2,
		Title:     "DB Server",
		StartedAt: time.Now(),
	}
	s2.appendScrollback([]byte("PostgreSQL error: connection refused on port 5432\nFatal database failure\n"))

	mgr.Register(s1)
	mgr.Register(s2)

	// Test Global Search
	matches := mgr.GlobalSearch(context.Background(), "error", false)
	if len(matches) != 1 {
		t.Fatalf("expected 1 search match, got %d", len(matches))
	}
	if matches[0].HostName != "DB Server" || matches[0].SessionID != "s2" {
		t.Fatalf("unexpected match: %+v", matches[0])
	}
}
