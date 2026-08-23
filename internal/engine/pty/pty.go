package pty

import (
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
)

// Winsize represents terminal window dimensions
type Winsize struct {
	Rows uint16
	Cols uint16
	X    uint16
	Y    uint16
}

// PTYBridge manages a master/slave pseudo-terminal pair for VTE and Go streams
type PTYBridge struct {
	Master *os.File
	Slave  *os.File
	mu     sync.Mutex
	closed bool
}

// Open creates a new connected PTY master and slave pair
func Open() (*PTYBridge, error) {
	master, slave, err := pty.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open pty: %w", err)
	}

	// Crucial: Set Master FD to non-blocking so GTK/VTE never block the main UI loop
	_ = syscall.SetNonblock(int(master.Fd()), true)

	return &PTYBridge{
		Master: master,
		Slave:  slave,
	}, nil
}

// SetSize updates the terminal rows and columns on the PTY
func (p *PTYBridge) SetSize(ws Winsize) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed || p.Master == nil {
		return nil
	}

	return pty.Setsize(p.Master, &pty.Winsize{
		Rows: ws.Rows,
		Cols: ws.Cols,
		X:    ws.X,
		Y:    ws.Y,
	})
}

// GetSize reads the current PTY window dimensions
func (p *PTYBridge) GetSize() (Winsize, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed || p.Master == nil {
		return Winsize{Rows: 24, Cols: 80}, nil
	}

	ws, err := pty.GetsizeFull(p.Master)
	if err != nil {
		return Winsize{Rows: 24, Cols: 80}, err
	}

	return Winsize{
		Rows: ws.Rows,
		Cols: ws.Cols,
		X:    ws.X,
		Y:    ws.Y,
	}, nil
}

// BridgeIO bi-directionally copies between Go ReadWriter (e.g. SSH channel/socket) and PTY Master
func (p *PTYBridge) BridgeIO(stream io.ReadWriter) (<-chan error, <-chan error) {
	errIn := make(chan error, 1)
	errOut := make(chan error, 1)

	// Stream (Remote) -> PTY Master (VTE)
	go func() {
		_, err := io.Copy(p.Master, stream)
		errIn <- err
	}()

	// PTY Master (VTE) -> Stream (Remote)
	go func() {
		_, err := io.Copy(stream, p.Master)
		errOut <- err
	}()

	return errIn, errOut
}

// Close closes master and slave handles
func (p *PTYBridge) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true

	var err1, err2 error
	if p.Slave != nil {
		err1 = p.Slave.Close()
	}
	if p.Master != nil {
		err2 = p.Master.Close()
	}

	if err1 != nil {
		return err1
	}
	return err2
}

// SetNonblock sets non-blocking mode on descriptor
func SetNonblock(fd uintptr, nonblocking bool) error {
	return syscall.SetNonblock(int(fd), nonblocking)
}

// WinsizeFromFD extracts terminal window size from fd
func WinsizeFromFD(fd uintptr) (*Winsize, error) {
	var ws Winsize
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)),
	)
	if errno != 0 {
		return nil, errno
	}
	return &ws, nil
}
