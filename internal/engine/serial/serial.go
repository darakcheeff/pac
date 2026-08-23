package serial

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/darakcheeff/pac/internal/engine/pty"
	"github.com/darakcheeff/pac/internal/storage"
	"go.bug.st/serial"
)

type SerialSession struct {
	rwc       io.ReadWriteCloser
	ptyBridge *pty.PTYBridge
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	closed    bool
}

func ConnectSerial(ctx context.Context, host *storage.Host, bridge *pty.PTYBridge, outputWriter io.Writer) (*SerialSession, error) {
	devPath := host.SerialPort
	if devPath == "" {
		devPath = host.Host
	}
	if devPath == "" {
		devPath = "/dev/ttyUSB0"
	}

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

	switch host.SerialParity {
	case "even", "E":
		mode.Parity = serial.EvenParity
	case "odd", "O":
		mode.Parity = serial.OddParity
	default:
		mode.Parity = serial.NoParity
	}

	switch host.SerialStopBits {
	case 2:
		mode.StopBits = serial.TwoStopBits
	default:
		mode.StopBits = serial.OneStopBit
	}

	var rwc io.ReadWriteCloser
	port, err := serial.Open(devPath, mode)
	if err == nil {
		rwc = port
	} else {
		// Fallback for virtual PTYs, socat pipes, and emulated ttys
		f, fErr := os.OpenFile(devPath, os.O_RDWR, 0666)
		if fErr != nil {
			return nil, fmt.Errorf("failed to open serial device %s: %w", devPath, err)
		}
		rwc = f
	}

	ctx, cancel := context.WithCancel(ctx)
	s := &SerialSession{
		rwc:       rwc,
		ptyBridge: bridge,
		ctx:       ctx,
		cancel:    cancel,
	}

	destWriter := outputWriter
	if destWriter == nil {
		destWriter = bridge.Slave
	}

	go func() {
		_, _ = io.Copy(destWriter, rwc)
	}()
	go func() {
		_, _ = io.Copy(rwc, bridge.Slave)
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
	return s.rwc.Close()
}
