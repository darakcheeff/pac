package local

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	OnExit    func(err error)
}

func StartLocalShell(ctx context.Context, bridge *enginePty.PTYBridge) (*LocalSession, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	cmd := exec.Command(shell, "-l")

	envMap := make(map[string]string)
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	uid := os.Getuid()
	userRun := fmt.Sprintf("/run/user/%d", uid)

	// 1. DISPLAY detection: if empty, find active X11 display socket or fallback to :0
	if envMap["DISPLAY"] == "" {
		if matches, err := filepath.Glob("/tmp/.X11-unix/X*"); err == nil && len(matches) > 0 {
			num := strings.TrimPrefix(filepath.Base(matches[0]), "X")
			envMap["DISPLAY"] = ":" + num
		} else {
			envMap["DISPLAY"] = ":0"
		}
	}

	// 2. XAUTHORITY detection: if empty, search common user locations
	if envMap["XAUTHORITY"] == "" {
		home, _ := os.UserHomeDir()
		candidates := []string{
			filepath.Join(home, ".Xauthority"),
			filepath.Join(userRun, "Xauthority"),
			filepath.Join(userRun, "gdm", "Xauthority"),
		}
		for _, cand := range candidates {
			if _, err := os.Stat(cand); err == nil {
				envMap["XAUTHORITY"] = cand
				break
			}
		}
	}

	// 3. WAYLAND_DISPLAY detection for Wayland sessions
	if envMap["WAYLAND_DISPLAY"] == "" {
		if wMatches, err := filepath.Glob(filepath.Join(userRun, "wayland-*")); err == nil && len(wMatches) > 0 {
			envMap["WAYLAND_DISPLAY"] = filepath.Base(wMatches[0])
		}
	}

	// 4. XDG_RUNTIME_DIR
	if envMap["XDG_RUNTIME_DIR"] == "" {
		if _, err := os.Stat(userRun); err == nil {
			envMap["XDG_RUNTIME_DIR"] = userRun
		}
	}

	// 5. DBUS_SESSION_BUS_ADDRESS
	if envMap["DBUS_SESSION_BUS_ADDRESS"] == "" && envMap["XDG_RUNTIME_DIR"] != "" {
		busPath := filepath.Join(envMap["XDG_RUNTIME_DIR"], "bus")
		if _, err := os.Stat(busPath); err == nil {
			envMap["DBUS_SESSION_BUS_ADDRESS"] = "unix:path=" + busPath
		}
	}

	envMap["TERM"] = "xterm-256color"

	var envSlice []string
	for k, v := range envMap {
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = envSlice

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
		waitErr := cmd.Wait()
		s.mu.Lock()
		wasClosed := s.closed
		s.mu.Unlock()
		s.Close()
		if !wasClosed && s.OnExit != nil {
			s.OnExit(waitErr)
		}
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
