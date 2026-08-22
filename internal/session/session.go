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
	cryptoSsh "golang.org/x/crypto/ssh"
)

// StreamSplitter splits output stream to PTY Slave, Logger, RingBuffer, and DirectoryTracker
type StreamSplitter struct {
	slave   io.Writer
	logger  *SessionLogger
	tracker *sftp.DirectoryTracker
	sess    *Session
}

func (w *StreamSplitter) Write(p []byte) (n int, err error) {
	if w.slave != nil {
		n, err = w.slave.Write(p)
	} else {
		n = len(p)
	}

	if w.logger != nil {
		_, _ = w.logger.Write(p)
	}
	if w.tracker != nil {
		w.tracker.FeedBytes(p)
	}
	if w.sess != nil {
		w.sess.appendScrollback(p)
	}
	return n, err
}

// Session represents an active terminal connection
type Session struct {
	ID         string
	Host       *storage.Host
	Title      string
	PTY        *pty.PTYBridge
	Logger     *SessionLogger
	SFTPClient *sftp.Client
	Tracker    *sftp.DirectoryTracker
	Splitter   *StreamSplitter

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
	return StartSessionWithJump(ctx, host, title, defaultLogsDir, nil)
}

// StartSessionWithJump connects to host with optional Jump Bastion SSH client
func StartSessionWithJump(ctx context.Context, host *storage.Host, title string, defaultLogsDir string, jumpClient *cryptoSsh.Client) (*Session, error) {
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

	sess.Splitter = &StreamSplitter{
		slave:   bridge.Slave,
		logger:  logger,
		tracker: sess.Tracker,
		sess:    sess,
	}

	// Start protocol driver
	switch host.Protocol {
	case storage.ProtoSSH:
		sshSess, err := ssh.ConnectSSHWithOutput(ctx, host, bridge, sess.Splitter, jumpClient)
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

	return sess, nil
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
