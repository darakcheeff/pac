package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darakcheeff/pac/internal/storage"
)

func TestIsDescendantOf(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pac_tree_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Create hierarchy:
	// A -> B -> C -> D
	_ = store.SaveGroup(&storage.Group{ID: "grp_a", Name: "Group A", ParentID: ""})
	_ = store.SaveGroup(&storage.Group{ID: "grp_b", Name: "Group B", ParentID: "grp_a"})
	_ = store.SaveGroup(&storage.Group{ID: "grp_c", Name: "Group C", ParentID: "grp_b"})
	_ = store.SaveGroup(&storage.Group{ID: "grp_d", Name: "Group D", ParentID: "grp_c"})

	// D is descendant of C, B, A
	if !isDescendantOf(store, "grp_d", "grp_c") {
		t.Errorf("expected D to be descendant of C")
	}
	if !isDescendantOf(store, "grp_d", "grp_b") {
		t.Errorf("expected D to be descendant of B")
	}
	if !isDescendantOf(store, "grp_d", "grp_a") {
		t.Errorf("expected D to be descendant of A")
	}

	// A is NOT descendant of D, C, B
	if isDescendantOf(store, "grp_a", "grp_d") {
		t.Errorf("expected A NOT to be descendant of D")
	}
	if isDescendantOf(store, "grp_a", "grp_b") {
		t.Errorf("expected A NOT to be descendant of B")
	}

	// Root checks
	if isDescendantOf(store, "root", "grp_a") {
		t.Errorf("root should not be descendant of grp_a")
	}
	if isDescendantOf(store, "", "grp_a") {
		t.Errorf("empty string should not be descendant of grp_a")
	}
}
