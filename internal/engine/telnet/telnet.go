package telnet

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/darakcheeff/pac/internal/engine/pty"
	"github.com/darakcheeff/pac/internal/storage"
)

// Telnet Protocol Constants (RFC 854)
const (
	IAC  = 255 // Interpret As Command
	DONT = 254
	DO   = 253
	WONT = 252
	WILL = 251
	SB   = 250 // Subnegotiation Begin
	SE   = 240 // Subnegotiation End
	ECHO = 1
	SGA  = 3   // Suppress Go Ahead
	NAWS = 31  // Negotiate About Window Size
)

type TelnetSession struct {
	conn      net.Conn
	ptyBridge *pty.PTYBridge
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	closed    bool
}

func ConnectTelnet(ctx context.Context, host *storage.Host, bridge *pty.PTYBridge) (*TelnetSession, error) {
	port := host.Port
	if port == 0 {
		port = 23
	}

	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host.Host, port))
	if err != nil {
		return nil, fmt.Errorf("telnet connection failed: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	s := &TelnetSession{
		conn:      conn,
		ptyBridge: bridge,
		ctx:       ctx,
		cancel:    cancel,
	}

	go s.readLoop()
	go s.writeLoop()

	return s, nil
}

func (s *TelnetSession) readLoop() {
	buf := make([]byte, 4096)
	cleanBuf := make([]byte, 4096)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		n, err := s.conn.Read(buf)
		if err != nil {
			s.Close()
			return
		}

		// Filter IAC commands and negotiate
		idx := 0
		for i := 0; i < n; i++ {
			if buf[i] == IAC && i+2 < n {
				cmd := buf[i+1]
				opt := buf[i+2]
				s.handleIAC(cmd, opt)
				i += 2
				continue
			}
			cleanBuf[idx] = buf[i]
			idx++
		}

		if idx > 0 {
			_, _ = s.ptyBridge.Slave.Write(cleanBuf[:idx])
		}
	}
}

func (s *TelnetSession) handleIAC(cmd, opt byte) {
	// Respond to basic negotiation: WILL SGA, WILL NAWS, DONT ECHO
	switch cmd {
	case DO:
		if opt == NAWS || opt == SGA {
			_, _ = s.conn.Write([]byte{IAC, WILL, opt})
		} else {
			_, _ = s.conn.Write([]byte{IAC, WONT, opt})
		}
	case WILL:
		if opt == ECHO || opt == SGA {
			_, _ = s.conn.Write([]byte{IAC, DO, opt})
		} else {
			_, _ = s.conn.Write([]byte{IAC, DONT, opt})
		}
	}
}

func (s *TelnetSession) writeLoop() {
	_, _ = io.Copy(s.conn, s.ptyBridge.Slave)
}

func (s *TelnetSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.cancel()
	return s.conn.Close()
}
