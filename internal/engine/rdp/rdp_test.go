package rdp

import (
	"strings"
	"testing"

	"github.com/darakcheeff/pac/internal/storage"
)

func TestBuildRDPArgs(t *testing.T) {
	h := &storage.Host{
		Host:              "192.168.1.50",
		Port:              3389,
		Username:          "admin",
		Password:          "secret123",
		RDPDomain:         "CORP",
		RDPResolution:     "1920x1080",
		RDPColorDepth:     32,
		RDPClipboard:      true,
		RDPSound:          "pulse",
		RDPMicrophone:     true,
		RDPSharedFolder:   "/home/tech/share",
		RDPIgnoreCert:     true,
		RDPFontSmoothing:  true,
		RDGatewayServer:   "gateway.corp.com:443",
		RDGatewayUsername: "gwadmin",
		RDGatewayPassword: "gwpassword",
		RDGatewayDomain:   "CORP",
	}

	args := BuildRDPArgs(h, h.Host, h.Port, 0x1a00042)
	joined := strings.Join(args, " ")

	expectedTokens := []string{
		"/v:192.168.1.50:3389",
		"/u:admin",
		"/d:CORP",
		"/parent-window:0x1a00042",
		"/from-stdin",
		"/size:1920x1080",
		"/bpp:32",
		"+clipboard",
		"/sound:sys:pulse",
		"/microphone:sys:pulse",
		"/drive:share,/home/tech/share",
		"/cert:ignore",
		"+fonts",
		"/g:gateway.corp.com:443",
		"/gu:gwadmin",
		"/gp:gwpassword",
		"/gd:CORP",
	}

	for _, token := range expectedTokens {
		if !strings.Contains(joined, token) {
			t.Errorf("expected argument token %q not found in: %s", token, joined)
		}
	}
}

func TestSanitizeArgs(t *testing.T) {
	args := []string{"/v:10.0.0.1", "/p:SuperSecret", "/gp:GatewaySecret", "+clipboard"}
	sanitized := sanitizeArgs(args)
	joined := strings.Join(sanitized, " ")

	if strings.Contains(joined, "SuperSecret") || strings.Contains(joined, "GatewaySecret") {
		t.Errorf("passwords were not sanitized: %s", joined)
	}
	if !strings.Contains(joined, "/p:******") || !strings.Contains(joined, "/gp:******") {
		t.Errorf("expected masked password placeholders: %s", joined)
	}
}
