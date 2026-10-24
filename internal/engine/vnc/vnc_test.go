package vnc

import (
	"strings"
	"testing"

	"github.com/darakcheeff/pac/internal/storage"
)

func TestBuildVNCArgsPacVNC(t *testing.T) {
	host := &storage.Host{
		Name:        "TestVNC",
		Host:        "192.168.1.50",
		Port:        5901,
		VNCPassword: "secretPassword",
		VNCScale:    true,
		VNCViewOnly: true,
		VNCShared:   true,
	}

	args, stdinPass, err := BuildVNCArgs("/opt/pac/pac-vnc", host, "192.168.1.50", 5901, 0x1234)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stdinPass != "secretPassword" {
		t.Fatalf("expected stdinPass 'secretPassword', got '%s'", stdinPass)
	}

	expectedParts := []string{
		"--host=192.168.1.50",
		"--port=5901",
		"--title=TestVNC",
		"--socket-xid=4660",
		"--scale=true",
		"--view-only=true",
		"--shared=true",
		"--password-stdin=true",
	}

	argStr := strings.Join(args, " ")
	for _, exp := range expectedParts {
		if !strings.Contains(argStr, exp) {
			t.Errorf("expected arg %s in %s", exp, argStr)
		}
	}
}

func TestBuildVNCArgsTigerVNC(t *testing.T) {
	host := &storage.Host{
		Name:           "TigerHost",
		Host:           "10.0.0.1",
		Port:           5900,
		VNCScale:       true,
		VNCViewOnly:    false,
		VNCShared:      true,
		VNCQuality:     7,
		VNCCompression: 3,
		VNCEncoding:    "tight",
	}

	args, stdinPass, err := BuildVNCArgs("/usr/bin/vncviewer", host, "10.0.0.1", 5900, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stdinPass != "" {
		t.Fatalf("expected empty stdinPass for tigervnc, got '%s'", stdinPass)
	}

	argStr := strings.Join(args, " ")
	expectedParts := []string{
		"-RemoteResize=1",
		"-Shared=1",
		"-QualityLevel=7",
		"-CompressLevel=3",
		"-PreferredEncoding=Tight",
		"10.0.0.1::5900",
	}

	for _, exp := range expectedParts {
		if !strings.Contains(argStr, exp) {
			t.Errorf("expected arg %s in %s", exp, argStr)
		}
	}
}

func TestSanitizeArgs(t *testing.T) {
	raw := []string{"--host=1.2.3.4", "--password=supersecret", "--scale=true"}
	san := sanitizeArgs(raw)
	if strings.Contains(strings.Join(san, " "), "supersecret") {
		t.Fatalf("password not sanitized: %v", san)
	}
	if san[1] != "--password=******" {
		t.Fatalf("unexpected sanitized string: %s", san[1])
	}
}
