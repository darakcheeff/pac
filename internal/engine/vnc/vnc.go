package vnc

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

// VNCSession represents an active VNC connection process
type VNCSession struct {
	cmd       *exec.Cmd
	tunnel    net.Listener
	closed    bool
	mu        sync.Mutex
	OnExit    func(err error)
	SocketXID uint
}

// ResolveVNCBinary locates the VNC client binary:
// 1. First checks private monolithic pac-vnc location (bundled or relative)
// 2. Falls back to system vncviewer (TigerVNC/TightVNC) or gvncviewer
func ResolveVNCBinary() (string, error) {
	// 1. Current executable directory (same dir as pac or ./helpers/pac-vnc)
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates := []string{
			filepath.Join(dir, "pac-vnc"),
			filepath.Join(dir, "helpers", "pac-vnc"),
		}
		for _, c := range candidates {
			if fileExecutable(c) {
				log.Printf("[VNC] Using relative bundled helper: %s", c)
				return c, nil
			}
		}
	}

	// 2. Private user share path (~/.local/share/pac/bin/pac-vnc)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		privatePath := filepath.Join(home, ".local", "share", "pac", "bin", "pac-vnc")
		if fileExecutable(privatePath) {
			log.Printf("[VNC] Using private bundled helper: %s", privatePath)
			return privatePath, nil
		}
	}

	// 3. System pac helper path (/usr/lib/pac/helpers/pac-vnc)
	if sysHelper := "/usr/lib/pac/helpers/pac-vnc"; fileExecutable(sysHelper) {
		log.Printf("[VNC] Using system pac helper: %s", sysHelper)
		return sysHelper, nil
	}

	// 4. System VNC binaries
	for _, bin := range []string{"vncviewer", "gvncviewer", "xtigervncviewer", "tightvncviewer"} {
		if path, err := exec.LookPath(bin); err == nil && path != "" {
			log.Printf("[VNC] Using system VNC client: %s (%s)", bin, path)
			return path, nil
		}
	}

	return "", fmt.Errorf("VNC client binary not found. Compile 'pac-vnc' into executable dir or install tigervnc-viewer")
}

func fileExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && (fi.Mode()&0111 != 0)
}

// ConnectVNC initiates a VNC session with optional SSH jump bastion tunnel and XEmbed socket
func ConnectVNC(ctx context.Context, host *storage.Host, socketXID uint, jumpClient *cryptoSsh.Client) (*VNCSession, error) {
	binPath, err := ResolveVNCBinary()
	if err != nil {
		return nil, err
	}

	var tunnel net.Listener
	targetHost := host.Host
	targetPort := host.Port
	if targetPort <= 0 {
		targetPort = 5900
	}

	// Setup SSH Jump Bastion tunnel if an intermediate SSH host is configured
	if jumpClient != nil {
		log.Printf("[VNC] Setting up SSH Jump tunnel to %s:%d through bastion...", targetHost, targetPort)
		l, tErr := net.Listen("tcp", "127.0.0.1:0")
		if tErr != nil {
			return nil, fmt.Errorf("failed to create local tunnel for VNC jump host: %w", tErr)
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
					remoteDest := fmt.Sprintf("%s:%d", host.Host, host.Port)
					if host.Port <= 0 {
						remoteDest = fmt.Sprintf("%s:5900", host.Host)
					}
					remoteConn, err := jumpClient.Dial("tcp", remoteDest)
					if err != nil {
						log.Printf("[VNC] Error dialing through jump bastion: %v", err)
						return
					}
					defer remoteConn.Close()

					var wg sync.WaitGroup
					wg.Add(2)
					go func() {
						defer wg.Done()
						_, _ = io.Copy(remoteConn, c)
					}()
					go func() {
						defer wg.Done()
						_, _ = io.Copy(c, remoteConn)
					}()
					wg.Wait()
				}(conn)
			}
		}()
	}

	args, stdinPass, err := BuildVNCArgs(binPath, host, targetHost, targetPort, socketXID)
	if err != nil {
		if tunnel != nil {
			_ = tunnel.Close()
		}
		return nil, err
	}

	log.Printf("[VNC] Spawning %s %v", binPath, sanitizeArgs(args))
	cmd := exec.CommandContext(ctx, binPath, args...)

	var stdinWriter io.WriteCloser
	if stdinPass != "" {
		w, err := cmd.StdinPipe()
		if err != nil {
			if tunnel != nil {
				_ = tunnel.Close()
			}
			return nil, fmt.Errorf("failed to create stdin pipe for VNC password: %w", err)
		}
		stdinWriter = w
	}

	if err := cmd.Start(); err != nil {
		if tunnel != nil {
			_ = tunnel.Close()
		}
		return nil, fmt.Errorf("failed to start VNC client: %w", err)
	}

	if stdinWriter != nil {
		go func() {
			defer stdinWriter.Close()
			_, _ = io.WriteString(stdinWriter, stdinPass+"\n")
		}()
	}

	sess := &VNCSession{
		cmd:       cmd,
		tunnel:    tunnel,
		SocketXID: socketXID,
	}

	go func() {
		exitErr := cmd.Wait()
		sess.mu.Lock()
		sess.closed = true
		sess.mu.Unlock()

		if tunnel != nil {
			_ = tunnel.Close()
		}

		if sess.OnExit != nil {
			sess.OnExit(exitErr)
		}
	}()

	return sess, nil
}

// BuildVNCArgs builds argument slice according to the target VNC client binary type
func BuildVNCArgs(binPath string, host *storage.Host, targetHost string, targetPort int, socketXID uint) ([]string, string, error) {
	base := strings.ToLower(filepath.Base(binPath))
	var args []string
	var stdinPass string

	pass := host.VNCPassword
	if pass == "" {
		pass = host.Password
	}

	if strings.Contains(base, "pac-vnc") {
		args = append(args,
			fmt.Sprintf("--host=%s", targetHost),
			fmt.Sprintf("--port=%d", targetPort),
			fmt.Sprintf("--title=%s", host.Name),
		)
		if host.Username != "" {
			args = append(args, fmt.Sprintf("--user=%s", host.Username))
		}
		if socketXID > 0 {
			args = append(args, fmt.Sprintf("--socket-xid=%d", socketXID))
		}
		if host.VNCScale {
			args = append(args, "--scale=true")
		} else {
			args = append(args, "--scale=false")
		}
		if host.VNCViewOnly {
			args = append(args, "--view-only=true")
		}
		if host.VNCShared {
			args = append(args, "--shared=true")
		}
		if pass != "" {
			args = append(args, "--password-stdin=true")
			stdinPass = pass
		}
		return args, stdinPass, nil
	}

	// TigerVNC / TightVNC vncviewer
	if strings.Contains(base, "vncviewer") {
		if socketXID > 0 {
			// Some vncviewer variants support -embed, others don't
			args = append(args, fmt.Sprintf("-embed=%d", socketXID))
		}
		if host.VNCScale {
			args = append(args, "-RemoteResize=1")
		}
		if host.VNCViewOnly {
			args = append(args, "-ViewOnly=1")
		}
		if host.VNCShared {
			args = append(args, "-Shared=1")
		}
		if host.VNCQuality > 0 {
			args = append(args, fmt.Sprintf("-QualityLevel=%d", host.VNCQuality))
		}
		if host.VNCCompression > 0 {
			args = append(args, fmt.Sprintf("-CompressLevel=%d", host.VNCCompression))
		}
		if host.VNCEncoding != "" && host.VNCEncoding != "auto" {
			args = append(args, fmt.Sprintf("-PreferredEncoding=%s", strings.Title(host.VNCEncoding)))
		}

		target := fmt.Sprintf("%s::%d", targetHost, targetPort)
		args = append(args, target)
		return args, "", nil
	}

	// gvncviewer
	if strings.Contains(base, "gvncviewer") {
		target := fmt.Sprintf("%s:%d", targetHost, targetPort)
		args = append(args, target)
		return args, "", nil
	}

	return nil, "", fmt.Errorf("unsupported VNC client binary: %s", binPath)
}

func sanitizeArgs(args []string) []string {
	sanitized := make([]string, len(args))
	for i, a := range args {
		if strings.HasPrefix(a, "--password=") {
			sanitized[i] = "--password=******"
		} else {
			sanitized[i] = a
		}
	}
	return sanitized
}

// Close terminates the VNC session process and cleans up any SSH tunnel
func (s *VNCSession) Close() error {
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

// IsRunning reports whether the VNC process is currently running
func (s *VNCSession) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed
}
