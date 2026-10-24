package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorageCRUD(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "pac-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Test Group
	g := &Group{
		ID:   "grp-1",
		Name: "Production Routers",
		Icon: "network-server",
	}
	if err := store.SaveGroup(g); err != nil {
		t.Fatalf("failed to save group: %v", err)
	}

	groups, err := store.GetAllGroups()
	if err != nil {
		t.Fatalf("failed to get groups: %v", err)
	}
	if len(groups) < 2 { // root + grp-1
		t.Fatalf("expected at least 2 groups, got %d", len(groups))
	}

	// Test Host
	h := &Host{
		ID:          "host-1",
		GroupID:     "grp-1",
		Name:        "MikroTik Core",
		Protocol:    ProtoSSH,
		Host:        "192.168.88.1",
		Port:        22,
		Username:    "admin",
		AuthMethod:  AuthPassword,
		Password:    "secret123",
		AutoSFTP:    true,
		EnableLogging: true,
		Notes:       "Primary gateway",
	}
	if err := store.SaveHost(h); err != nil {
		t.Fatalf("failed to save host: %v", err)
	}

	loadedHost, err := store.GetHost("host-1")
	if err != nil {
		t.Fatalf("failed to get host: %v", err)
	}
	if loadedHost.Name != "MikroTik Core" || loadedHost.Host != "192.168.88.1" {
		t.Fatalf("unexpected loaded host data: %+v", loadedHost)
	}

	// Test Serial Host with SerialAutoBaud
	serialHost := &Host{
		ID:             "serial-1",
		GroupID:        "grp-1",
		Name:           "Serial Console",
		Protocol:       ProtoSerial,
		SerialPort:     "/dev/ttyUSB0",
		SerialBaudRate: 115200,
		SerialDataBits: 8,
		SerialStopBits: 1,
		SerialParity:   "none",
		SerialAutoBaud: true,
	}
	if err := store.SaveHost(serialHost); err != nil {
		t.Fatalf("failed to save serial host: %v", err)
	}
	loadedSerial, err := store.GetHost("serial-1")
	if err != nil {
		t.Fatalf("failed to get serial host: %v", err)
	}
	if !loadedSerial.SerialAutoBaud || loadedSerial.SerialBaudRate != 115200 || loadedSerial.SerialPort != "/dev/ttyUSB0" {
		t.Fatalf("unexpected loaded serial host: %+v", loadedSerial)
	}

	// Test RDP Host with all fields
	rdpHost := &Host{
		ID:                "rdp-1",
		GroupID:           "grp-1",
		Name:              "Windows DC",
		Protocol:          ProtoRDP,
		Host:              "192.168.1.100",
		Port:              3389,
		Username:          "Administrator",
		Password:          "P@ssw0rd",
		RDPDomain:         "CORP",
		RDPResolution:     "dynamic",
		RDPColorDepth:     32,
		RDPClipboard:      true,
		RDPSound:          "pulse",
		RDPMicrophone:     true,
		RDPSharedFolder:   "/home/tech/Downloads",
		RDPIgnoreCert:     true,
		RDPPerformance:    "lan",
		RDPFontSmoothing:  true,
		RDPEmbedded:       true,
		RDGatewayServer:   "gw.corp.com:443",
		RDGatewayUsername: "gwuser",
		RDGatewayPassword: "gwpassword",
		RDGatewayDomain:   "CORP",
	}
	if err := store.SaveHost(rdpHost); err != nil {
		t.Fatalf("failed to save RDP host: %v", err)
	}
	loadedRDP, err := store.GetHost("rdp-1")
	if err != nil {
		t.Fatalf("failed to get RDP host: %v", err)
	}
	if loadedRDP.Protocol != ProtoRDP || loadedRDP.RDPDomain != "CORP" || loadedRDP.RDPSound != "pulse" ||
		!loadedRDP.RDPMicrophone || loadedRDP.RDPSharedFolder != "/home/tech/Downloads" ||
		loadedRDP.RDGatewayServer != "gw.corp.com:443" {
		t.Fatalf("unexpected loaded RDP host: %+v", loadedRDP)
	}

	// Test Note
	if err := store.SaveNote("host-1", "Test note content"); err != nil {
		t.Fatalf("failed to save note: %v", err)
	}
	note, err := store.GetNote("host-1")
	if err != nil || note != "Test note content" {
		t.Fatalf("unexpected note: %v, err: %v", note, err)
	}

	// Test Saved Sessions
	states := []SavedSessionState{
		{
			ID:             "sess-1",
			HostID:         "host-1",
			Title:          "MikroTik Core (1)",
			Protocol:       ProtoSSH,
			TabIndex:       0,
			ScrollbackDump: "RouterOS v7.14 prompt>",
		},
	}
	if err := store.SaveActiveSessions(states); err != nil {
		t.Fatalf("failed to save active sessions: %v", err)
	}

	saved, err := store.GetSavedSessions()
	if err != nil || len(saved) != 1 || saved[0].ScrollbackDump != "RouterOS v7.14 prompt>" {
		t.Fatalf("unexpected saved sessions: %+v, err: %v", saved, err)
	}
}
