package instance

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SocketPath returns the Unix domain socket path for single instance coordination.
func SocketPath() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir != "" {
		return filepath.Join(runtimeDir, "pac.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("pac-%d.sock", os.Getuid()))
}

// TryNotifyExisting attempts to connect to an existing running PAC instance.
// If an instance is running, it sends a command (ACTIVATE or CONNECT:<target>),
// waits for acknowledgment, and returns true.
// If no instance is running, it cleans up any stale socket file and returns false.
func TryNotifyExisting(connectTarget string) (bool, error) {
	sockPath := SocketPath()

	conn, err := net.DialTimeout("unix", sockPath, 800*time.Millisecond)
	if err != nil {
		// No active process is listening on the socket. Clean up stale socket file if any.
		_ = os.Remove(sockPath)
		return false, nil
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	var msg string
	if connectTarget != "" {
		msg = fmt.Sprintf("CONNECT:%s\n", strings.TrimSpace(connectTarget))
	} else {
		msg = "ACTIVATE\n"
	}

	if _, err := conn.Write([]byte(msg)); err != nil {
		_ = os.Remove(sockPath)
		return false, nil
	}

	reader := bufio.NewReader(conn)
	resp, _ := reader.ReadString('\n')
	if strings.HasPrefix(resp, "OK") {
		return true, nil
	}

	return true, nil
}

// Listener handles incoming activation requests from subsequent PAC executions.
type Listener struct {
	listener net.Listener
	sockPath string
	closed   chan struct{}
}

// StartListener begins listening for single instance notifications on the unix socket.
func StartListener(onActivate func(), onConnect func(target string)) (*Listener, error) {
	sockPath := SocketPath()
	_ = os.Remove(sockPath)

	l, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on socket %s: %w", sockPath, err)
	}

	_ = os.Chmod(sockPath, 0600)

	instListener := &Listener{
		listener: l,
		sockPath: sockPath,
		closed:   make(chan struct{}),
	}

	go instListener.acceptLoop(onActivate, onConnect)

	return instListener, nil
}

func (l *Listener) acceptLoop(onActivate func(), onConnect func(target string)) {
	for {
		conn, err := l.listener.Accept()
		if err != nil {
			select {
			case <-l.closed:
				return
			default:
				return
			}
		}

		go func(c net.Conn) {
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(2 * time.Second))
			reader := bufio.NewReader(c)
			line, err := reader.ReadString('\n')
			if err != nil && len(line) == 0 {
				return
			}

			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "CONNECT:") {
				target := strings.TrimPrefix(line, "CONNECT:")
				if onConnect != nil {
					onConnect(target)
				}
			} else if line == "ACTIVATE" {
				if onActivate != nil {
					onActivate()
				}
			}

			_, _ = c.Write([]byte("OK\n"))
		}(conn)
	}
}

// Close closes the listener and removes the unix socket file.
func (l *Listener) Close() error {
	select {
	case <-l.closed:
		return nil
	default:
		close(l.closed)
	}
	err := l.listener.Close()
	_ = os.Remove(l.sockPath)
	return err
}
