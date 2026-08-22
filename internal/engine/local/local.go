package local

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
	enginePty "github.com/darakcheeff/pac/internal/engine/pty"
)

type LocalSession struct {
	cmd       *exec.Cmd
	ptyBridge *enginePty.PTYBridge
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	closed    bool
}

func StartLocalShell(ctx context.Context, bridge *enginePty.PTYBridge) (*LocalSession, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ws, _ := bridge.GetSize()
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: ws.Rows,
		Cols: ws.Cols,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start local shell pty: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	s := &LocalSession{
		cmd:       cmd,
		ptyBridge: bridge,
		ctx:       ctx,
		cancel:    cancel,
	}

	go bridge.BridgeIO(ptmx)

	go func() {
		_ = cmd.Wait()
		s.Close()
	}()

	return s, nil
}

func (s *LocalSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.cancel()

	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	return nil
}
