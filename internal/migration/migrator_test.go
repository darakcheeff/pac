package migration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darakcheeff/pac/internal/storage"
)

func TestMigrateYAML(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "pac-mig-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	store, err := storage.NewStore(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	yamlData := []byte(`
environments:
  grp-servers:
    title: "Production"
    is_folder: true
    parent: "__ROOT__"
  srv-1:
    title: "Web Server"
    ip: "10.0.0.1"
    port: 22
    user: "root"
    method: "SSH"
    parent: "grp-servers"
    description: "Main Nginx"
`)

	count, err := importYAML(store, yamlData)
	if err != nil {
		t.Fatalf("YAML import failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 items imported, got %d", count)
	}

	h, err := store.GetHost("srv-1")
	if err != nil || h.Name != "Web Server" || h.GroupID != "grp-servers" {
		t.Fatalf("host import incorrect: %+v", h)
	}
}

func TestMigratePerlDumper(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "pac-mig-perl-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	store, err := storage.NewStore(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	perlDump := `$VAR1 = {
  'fld-101' => {
    'title' => 'Routers',
    'is_folder' => '1',
    'parent' => '__ROOT__'
  },
  'host-202' => {
    'title' => 'MikroTik CCR',
    'ip' => '192.168.1.1',
    'port' => '22',
    'user' => 'admin',
    'pass' => 'topsecret',
    'parent' => 'fld-101'
  }
};`

	count, err := importPerlDataDumper(store, perlDump)
	if err != nil {
		t.Fatalf("Perl import failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 items imported, got %d", count)
	}

	h, err := store.GetHost("host-202")
	if err != nil || h.Name != "MikroTik CCR" || h.GroupID != "fld-101" {
		t.Fatalf("perl host import incorrect: %+v", h)
	}
}
