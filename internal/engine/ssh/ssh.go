package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/darakcheeff/pac/internal/engine/pty"
	"github.com/darakcheeff/pac/internal/storage"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// SSHSession encapsulates an active SSH connection, channels, and PTY
type SSHSession struct {
	client         *ssh.Client
	session        *ssh.Session
	ptyBridge      *pty.PTYBridge
	forwardManager *ForwardManager
	ctx            context.Context
	cancel         context.CancelFunc
	mu             sync.Mutex
	closed         bool
}

// ConnectSSH establishes SSH connection based on host profile
func ConnectSSH(ctx context.Context, host *storage.Host, bridge *pty.PTYBridge, jumpClient *ssh.Client) (*SSHSession, error) {
	return ConnectSSHWithOutput(ctx, host, bridge, bridge.Slave, jumpClient)
}

// ConnectSSHWithOutput establishes SSH connection and routes stdout/stderr to outputWriter
func ConnectSSHWithOutput(ctx context.Context, host *storage.Host, bridge *pty.PTYBridge, outputWriter io.Writer, jumpClient *ssh.Client) (*SSHSession, error) {
	authMethods := []ssh.AuthMethod{}

	// 1. SSH Agent
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			ag := agent.NewClient(conn)
			authMethods = append(authMethods, ssh.PublicKeysCallback(ag.Signers))
		}
	}

	// 2. Private Key
	if host.KeyPath != "" {
		keyBytes, err := os.ReadFile(host.KeyPath)
		if err == nil {
			var signer ssh.Signer
			if host.KeyPass != "" {
				signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(host.KeyPass))
			} else {
				signer, err = ssh.ParsePrivateKey(keyBytes)
			}
			if err == nil {
				authMethods = append(authMethods, ssh.PublicKeys(signer))
			}
		}
	}

	// 3. Password
	if host.Password != "" {
		authMethods = append(authMethods, ssh.Password(host.Password))
		// Also add keyboard-interactive fallback
		authMethods = append(authMethods, ssh.KeyboardInteractive(
			func(user, instruction string, questions []string, echos []bool) (answers []string, err error) {
				answers = make([]string, len(questions))
				for i := range questions {
					answers[i] = host.Password
				}
				return answers, nil
			},
		))
	}

	if len(authMethods) == 0 {
		return nil, errors.New("no authentication methods available")
	}

	config := &ssh.ClientConfig{
		User:            host.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	var client *ssh.Client
	targetAddr := fmt.Sprintf("%s:%d", host.Host, host.Port)

	if jumpClient != nil {
		// Tunnel via bastion / jump host
		conn, err := jumpClient.Dial("tcp", targetAddr)
		if err != nil {
			return nil, fmt.Errorf("jump host dial failed: %w", err)
		}
		ncc, chans, reqs, err := ssh.NewClientConn(conn, targetAddr, config)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("jump client handshake failed: %w", err)
		}
		client = ssh.NewClient(ncc, chans, reqs)
	} else {
		// Direct dial
		var err error
		client, err = ssh.Dial("tcp", targetAddr, config)
		if err != nil {
			return nil, fmt.Errorf("ssh dial failed: %w", err)
		}
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to open ssh session: %w", err)
	}

	// Setup Terminal Modes
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,     // enable echoing
		ssh.TTY_OP_ISPEED: 115200,
		ssh.TTY_OP_OSPEED: 115200,
	}

	termType := host.TerminalType
	if termType == "" {
		termType = "xterm-256color"
	}

	size, _ := bridge.GetSize()
	if err := session.RequestPty(termType, int(size.Rows), int(size.Cols), modes); err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("pty request failed: %w", err)
	}

	// X11 Forwarding if enabled
	if host.X11Forwarding {
		_ = SetupX11Forwarding(client, session)
	}

	// Connect pipes: user typing from bridge.Slave -> SSH stdin
	session.Stdin = bridge.Slave
	if outputWriter != nil {
		session.Stdout = outputWriter
		session.Stderr = outputWriter
	} else {
		session.Stdout = bridge.Slave
		session.Stderr = bridge.Slave
	}

	if err := session.Shell(); err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("failed to start shell: %w", err)
	}

	// Start Port Forwardings
	fwdMgr := NewForwardManager(client)
	if len(host.PortForwards) > 0 {
		_ = fwdMgr.StartForwardings(host.PortForwards)
	}

	ctx, cancel := context.WithCancel(ctx)
	s := &SSHSession{
		client:         client,
		session:        session,
		ptyBridge:      bridge,
		forwardManager: fwdMgr,
		ctx:            ctx,
		cancel:         cancel,
	}

	// Start KeepAlive loop
	go s.keepAliveLoop()

	return s, nil
}

// WindowChange sends terminal resize signal to remote host
func (s *SSHSession) WindowChange(rows, cols int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.session == nil {
		return nil
	}

	return s.session.WindowChange(rows, cols)
}

// Client returns the underlying ssh.Client (useful for SFTP subsystem)
func (s *SSHSession) Client() *ssh.Client {
	return s.client
}

// Wait waits for the remote session to exit
func (s *SSHSession) Wait() error {
	if s.session == nil {
		return nil
	}
	return s.session.Wait()
}

// Close closes session, forwards, and underlying client
func (s *SSHSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true
	s.cancel()

	if s.forwardManager != nil {
		s.forwardManager.Close()
	}
	if s.session != nil {
		_ = s.session.Close()
	}
	if s.client != nil {
		_ = s.client.Close()
	}
	return nil
}

func (s *SSHSession) keepAliveLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.closed || s.client == nil {
				s.mu.Unlock()
				return
			}
			client := s.client
			s.mu.Unlock()

			// Send keepalive request
			_, _, _ = client.SendRequest("keepalive@openssh.com", true, nil)
		}
	}
}
