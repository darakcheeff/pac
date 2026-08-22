package serial

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/darakcheeff/pac/internal/engine/pty"
	"github.com/darakcheeff/pac/internal/storage"
	"go.bug.st/serial"
)

type SerialSession struct {
	port      serial.Port
	ptyBridge *pty.PTYBridge
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	closed    bool
}

func ConnectSerial(ctx context.Context, host *storage.Host, bridge *pty.PTYBridge) (*SerialSession, error) {
	baud := host.SerialBaudRate
	if baud == 0 {
		baud = 115200
	}
	dataBits := host.SerialDataBits
	if dataBits == 0 {
		dataBits = 8
	}

	mode := &serial.Mode{
		BaudRate: baud,
		DataBits: dataBits,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(host.SerialPort, mode)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port %s: %w", host.SerialPort, err)
	}

	ctx, cancel := context.WithCancel(ctx)
	s := &SerialSession{
		port:      port,
		ptyBridge: bridge,
		ctx:       ctx,
		cancel:    cancel,
	}

	go func() {
		_, _ = io.Copy(bridge.Slave, port)
	}()
	go func() {
		_, _ = io.Copy(port, bridge.Slave)
	}()

	return s, nil
}

func (s *SerialSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.cancel()
	return s.port.Close()
}
