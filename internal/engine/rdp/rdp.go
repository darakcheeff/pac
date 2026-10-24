package rdp

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/darakcheeff/pac/internal/storage"
	cryptoSsh "golang.org/x/crypto/ssh"
)

// RDPSession represents an active RDP connection process
type RDPSession struct {
	cmd       *exec.Cmd
	tunnel    net.Listener
	closed    bool
	mu        sync.Mutex
	OnExit    func(err error)
	SocketXID uint
}

// ResolveRDPBinary locates the RDP client binary
// 1. First checks private monolithic pac-rdp location
// 2. Falls back to system xfreerdp3 / xfreerdp / wlfreerdp
func ResolveRDPBinary() (string, error) {
	// 1. Private / bundled helper paths
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		privatePath := filepath.Join(home, ".local", "share", "pac", "bin", "pac-rdp")
		if fi, err := os.Stat(privatePath); err == nil && !fi.IsDir() && (fi.Mode()&0111 != 0) {
			log.Printf("[RDP] Using private bundled helper: %s", privatePath)
			return privatePath, nil
		}
	}

	if sysHelper := "/usr/lib/pac/helpers/pac-rdp"; fileExecutable(sysHelper) {
		log.Printf("[RDP] Using system pac helper: %s", sysHelper)
		return sysHelper, nil
	}

	if exe, err := os.Executable(); err == nil {
		relHelper := filepath.Join(filepath.Dir(exe), "helpers", "pac-rdp")
		if fileExecutable(relHelper) {
			log.Printf("[RDP] Using relative pac helper: %s", relHelper)
			return relHelper, nil
		}
	}

	// 2. System binaries
	for _, bin := range []string{"xfreerdp3", "xfreerdp", "wlfreerdp"} {
		if path, err := exec.LookPath(bin); err == nil && path != "" {
			log.Printf("[RDP] Using system RDP client: %s (%s)", bin, path)
			return path, nil
		}
	}

	return "", fmt.Errorf("RDP client binary not found. Place 'pac-rdp' into ~/.local/share/pac/bin/ or install freerdp (sudo apt install freerdp3-x11)")
}

func fileExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && (fi.Mode()&0111 != 0)
}

// ConnectRDP initiates an RDP session with optional SSH jump bastion tunnel and XEmbed socket
func ConnectRDP(ctx context.Context, host *storage.Host, socketXID uint, jumpClient *cryptoSsh.Client) (*RDPSession, error) {
	binPath, err := ResolveRDPBinary()
	if err != nil {
		return nil, err
	}

	var tunnel net.Listener
	targetHost := host.Host
	targetPort := host.Port
	if targetPort <= 0 {
		targetPort = 3389
	}

	// Setup SSH Jump Bastion tunnel if an intermediate SSH host is configured
	if jumpClient != nil {
		log.Printf("[RDP] Setting up SSH Jump tunnel to %s:%d through bastion...", targetHost, targetPort)
		l, tErr := net.Listen("tcp", "127.0.0.1:0")
		if tErr != nil {
			return nil, fmt.Errorf("failed to create local tunnel for RDP jump host: %w", tErr)
		}
		tunnel = l
		localAddr := l.Addr().(*net.TCPAddr)
		targetHost = "127.0.0.1"
		targetPort = localAddr.Port

		go func() {
			for {
				conn, err := l.Accept()
				if err != nil {
					return
				}
				go func(c net.Conn) {
					defer c.Close()
					remoteConn, rErr := jumpClient.Dial("tcp", fmt.Sprintf("%s:%d", host.Host, host.Port))
					if rErr != nil {
						log.Printf("[RDP] Tunnel forward error: %v", rErr)
						return
					}
					defer remoteConn.Close()

					done := make(chan struct{}, 2)
					go func() { _, _ = io.Copy(remoteConn, c); done <- struct{}{} }()
					go func() { _, _ = io.Copy(c, remoteConn); done <- struct{}{} }()
					<-done
				}(conn)
			}
		}()
		log.Printf("[RDP] Local SSH tunnel established on 127.0.0.1:%d -> %s:%d", targetPort, host.Host, host.Port)
	}

	args := BuildRDPArgs(host, targetHost, targetPort, socketXID)
	log.Printf("[RDP] Launching: %s %s", binPath, strings.Join(sanitizeArgs(args), " "))

	cmd := exec.CommandContext(ctx, binPath, args...)

	// Pass password via stdin if supported or configured
	var stdinPipe io.WriteCloser
	if host.Password != "" {
		p, pErr := cmd.StdinPipe()
		if pErr == nil {
			stdinPipe = p
		}
	}

	if err := cmd.Start(); err != nil {
		if tunnel != nil {
			_ = tunnel.Close()
		}
		return nil, fmt.Errorf("failed to start RDP process: %w", err)
	}

	if stdinPipe != nil {
		go func() {
			_, _ = stdinPipe.Write([]byte(host.Password + "\n"))
			_ = stdinPipe.Close()
		}()
	}

	sess := &RDPSession{
		cmd:       cmd,
		tunnel:    tunnel,
		SocketXID: socketXID,
	}

	go func() {
		waitErr := cmd.Wait()
		log.Printf("[RDP] Process exited for host %s: %v", host.Name, waitErr)

		sess.mu.Lock()
		wasClosed := sess.closed
		sess.closed = true
		if sess.tunnel != nil {
			_ = sess.tunnel.Close()
		}
		cb := sess.OnExit
		sess.mu.Unlock()

		if !wasClosed && cb != nil {
			cb(waitErr)
		}
	}()

	return sess, nil
}

// BuildRDPArgs creates command line flags for FreeRDP / pac-rdp
func BuildRDPArgs(host *storage.Host, targetHost string, targetPort int, socketXID uint) []string {
	var args []string

	// Target host and port
	args = append(args, fmt.Sprintf("/v:%s:%d", targetHost, targetPort))

	// User and Domain
	if host.Username != "" {
		args = append(args, fmt.Sprintf("/u:%s", host.Username))
	}
	if host.RDPDomain != "" {
		args = append(args, fmt.Sprintf("/d:%s", host.RDPDomain))
	}

	// Embedding into GTK socket if provided
	if socketXID > 0 {
		args = append(args, fmt.Sprintf("/parent-window:0x%x", socketXID))
	}

	// Password via stdin (modern FreeRDP) or command line fallback
	if host.Password != "" {
		args = append(args, "/from-stdin")
	}

	// Resolution & Sizing
	res := strings.TrimSpace(strings.ToLower(host.RDPResolution))
	if res == "" || res == "dynamic" {
		args = append(args, "/dynamic-resolution")
	} else if strings.Contains(res, "x") {
		args = append(args, fmt.Sprintf("/size:%s", res))
	} else {
		args = append(args, "/dynamic-resolution")
	}

	// Color depth
	depth := host.RDPColorDepth
	if depth != 16 && depth != 24 && depth != 32 {
		depth = 32
	}
	args = append(args, fmt.Sprintf("/bpp:%d", depth))

	// Clipboard (Enabled by default)
	if host.RDPClipboard {
		args = append(args, "+clipboard")
	} else {
		args = append(args, "-clipboard")
	}

	// Sound Redirection
	sound := strings.TrimSpace(strings.ToLower(host.RDPSound))
	switch sound {
	case "pulse":
		args = append(args, "/sound:sys:pulse")
	case "alsa":
		args = append(args, "/sound:sys:alsa")
	case "off":
		// no sound arg
	default:
		// Default to pulse
		args = append(args, "/sound:sys:pulse")
	}

	// Microphone Redirection
	if host.RDPMicrophone {
		args = append(args, "/microphone:sys:pulse")
	}

	// Drive / Folder Redirection
	if host.RDPSharedFolder != "" {
		folderPath := host.RDPSharedFolder
		shareName := filepath.Base(folderPath)
		if shareName == "" || shareName == "/" || shareName == "." {
			shareName = "shared"
		}
		args = append(args, fmt.Sprintf("/drive:%s,%s", shareName, folderPath))
	}

	// Certificate Validation (Ignore by default for admin convenience)
	if host.RDPIgnoreCert {
		args = append(args, "/cert:ignore")
	}

	// Performance & Visual Effects
	perf := strings.ToLower(host.RDPPerformance)
	if perf == "wan" || perf == "modem" {
		args = append(args, "/network:modem", "-wallpaper", "-themes", "-window-drag")
	} else {
		args = append(args, "/network:auto")
	}

	// Font Smoothing (ClearType)
	if host.RDPFontSmoothing {
		args = append(args, "+fonts")
	}

	// Auto-reconnect
	args = append(args, "+auto-reconnect")

	// Microsoft RD Gateway (if configured)
	if host.RDGatewayServer != "" {
		args = append(args, fmt.Sprintf("/g:%s", host.RDGatewayServer))
		if host.RDGatewayUsername != "" {
			args = append(args, fmt.Sprintf("/gu:%s", host.RDGatewayUsername))
		}
		if host.RDGatewayPassword != "" {
			args = append(args, fmt.Sprintf("/gp:%s", host.RDGatewayPassword))
		}
		if host.RDGatewayDomain != "" {
			args = append(args, fmt.Sprintf("/gd:%s", host.RDGatewayDomain))
		}
	}

	return args
}

// sanitizeArgs masks sensitive fields in logs
func sanitizeArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.HasPrefix(a, "/p:") {
			out[i] = "/p:******"
		} else if strings.HasPrefix(a, "/gp:") {
			out[i] = "/gp:******"
		} else {
			out[i] = a
		}
	}
	return out
}

// Close terminates the RDP session process and closes associated tunnels
func (s *RDPSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	if s.tunnel != nil {
		_ = s.tunnel.Close()
	}

	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	return nil
}
