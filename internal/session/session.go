package session

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/darakcheeff/pac/internal/engine/local"
	"github.com/darakcheeff/pac/internal/engine/pty"
	"github.com/darakcheeff/pac/internal/engine/serial"
	"github.com/darakcheeff/pac/internal/engine/sftp"
	"github.com/darakcheeff/pac/internal/engine/ssh"
	"github.com/darakcheeff/pac/internal/engine/telnet"
	"github.com/darakcheeff/pac/internal/storage"
)

// Session represents an active terminal connection
type Session struct {
	ID         string
	Host       *storage.Host
	Title      string
	PTY        *pty.PTYBridge
	Logger     *SessionLogger
	SFTPClient *sftp.Client
	Tracker    *sftp.DirectoryTracker

	// Underlying session drivers
	SSHSession    *ssh.SSHSession
	TelnetSession *telnet.TelnetSession
	SerialSession *serial.SerialSession
	LocalSession  *local.LocalSession

	// Ring buffer for scrollback history and global search
	scrollback   []byte
	scrollbackMu sync.RWMutex

	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	closed    bool
	StartedAt time.Time
}

// StartSession connects to host and attaches PTY and logging
func StartSession(ctx context.Context, host *storage.Host, title string, defaultLogsDir string) (*Session, error) {
	bridge, err := pty.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate pty: %w", err)
	}

	logger, _ := NewSessionLogger(host, defaultLogsDir)

	ctx, cancel := context.WithCancel(ctx)
	sess := &Session{
		ID:        fmt.Sprintf("sess-%d", time.Now().UnixNano()),
		Host:      host,
		Title:     title,
		PTY:       bridge,
		Logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
		StartedAt: time.Now(),
	}

	if sess.Title == "" {
		sess.Title = host.Name
	}

	// Setup Directory Tracker
	sess.Tracker = sftp.NewDirectoryTracker(func(path string) {
		if sess.SFTPClient != nil {
			sess.SFTPClient.SetCurrentDir(path)
		}
	})

	// Start protocol driver
	switch host.Protocol {
	case storage.ProtoSSH:
		sshSess, err := ssh.ConnectSSH(ctx, host, bridge, nil)
		if err != nil {
			bridge.Close()
			if logger != nil {
				logger.Close()
			}
			return nil, err
		}
		sess.SSHSession = sshSess

		// Auto SFTP subsystem
		if host.AutoSFTP {
			if sftpCl, err := sftp.NewClient(sshSess.Client()); err == nil {
				sess.SFTPClient = sftpCl
			}
		}

	case storage.ProtoTelnet:
		tSess, err := telnet.ConnectTelnet(ctx, host, bridge)
		if err != nil {
			bridge.Close()
			return nil, err
		}
		sess.TelnetSession = tSess

	case storage.ProtoSerial:
		sSess, err := serial.ConnectSerial(ctx, host, bridge)
		if err != nil {
			bridge.Close()
			return nil, err
		}
		sess.SerialSession = sSess

	case storage.ProtoLocal:
		lSess, err := local.StartLocalShell(ctx, bridge)
		if err != nil {
			bridge.Close()
			return nil, err
		}
		sess.LocalSession = lSess
	}

	// Start stream monitor for logging, history ring buffer, and OSC 7 tracking
	go sess.streamMonitor()

	return sess, nil
}

func (s *Session) streamMonitor() {
	buf := make([]byte, 4096)
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		n, err := s.PTY.Slave.Read(buf)
		if n > 0 {
			chunk := buf[:n]

			// 1. Write to Logger if active
			if s.Logger != nil {
				_, _ = s.Logger.Write(chunk)
			}

			// 2. Feed OSC 7 tracker
			if s.Tracker != nil {
				s.Tracker.FeedBytes(chunk)
			}

			// 3. Append to scrollback ring buffer (max 1 MB per session)
			s.appendScrollback(chunk)
		}

		if err != nil {
			return
		}
	}
}

func (s *Session) appendScrollback(data []byte) {
	s.scrollbackMu.Lock()
	defer s.scrollbackMu.Unlock()

	s.scrollback = append(s.scrollback, data...)
	const maxScrollback = 1024 * 1024 // 1 MB
	if len(s.scrollback) > maxScrollback {
		s.scrollback = s.scrollback[len(s.scrollback)-maxScrollback:]
	}
}

// GetScrollbackText returns raw text accumulated in session
func (s *Session) GetScrollbackText() string {
	s.scrollbackMu.RLock()
	defer s.scrollbackMu.RUnlock()
	return string(s.scrollback)
}

// SendInput writes raw string to session PTY
func (s *Session) SendInput(input string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.PTY == nil || s.PTY.Slave == nil {
		return fmt.Errorf("session is closed")
	}

	_, err := io.WriteString(s.PTY.Slave, input)
	return err
}

// Close closes session and all underlying resources
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}

	if s.Logger != nil {
		_ = s.Logger.Close()
	}
	if s.SFTPClient != nil {
		_ = s.SFTPClient.Close()
	}
	if s.SSHSession != nil {
		_ = s.SSHSession.Close()
	}
	if s.TelnetSession != nil {
		_ = s.TelnetSession.Close()
	}
	if s.SerialSession != nil {
		_ = s.SerialSession.Close()
	}
	if s.LocalSession != nil {
		_ = s.LocalSession.Close()
	}
	if s.PTY != nil {
		_ = s.PTY.Close()
	}
	return nil
}
