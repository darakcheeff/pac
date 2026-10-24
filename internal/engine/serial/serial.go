package serial

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/darakcheeff/pac/internal/engine/pty"
	"github.com/darakcheeff/pac/internal/storage"
	"go.bug.st/serial"
)

type SerialSession struct {
	port               serial.Port
	rwc                io.ReadWriteCloser
	ptyBridge          *pty.PTYBridge
	ctx                context.Context
	cancel             context.CancelFunc
	mu                 sync.Mutex
	closed             bool
	baud               int
	dataBits           int
	parity             string
	stopBits           int
	devPath            string
	autoBaud           bool
	OnExit             func(err error)
	OnBaudRateDetected func(detectedBaud, currentBaud int)

	probeMu   sync.Mutex
	isProbing bool
	probeCh   chan []byte
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
		Parity:   parseParity(host.SerialParity),
		StopBits: parseStopBits(host.SerialStopBits),
	}

	var rwc io.ReadWriteCloser
	var sPort serial.Port
	port, err := serial.Open(devPath, mode)
	if err == nil {
		rwc = port
		sPort = port
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
		port:      sPort,
		rwc:       rwc,
		ptyBridge: bridge,
		ctx:       ctx,
		cancel:    cancel,
		baud:      baud,
		dataBits:  dataBits,
		parity:    host.SerialParity,
		stopBits:  host.SerialStopBits,
		devPath:   devPath,
		autoBaud:  host.SerialAutoBaud,
		probeCh:   make(chan []byte, 16),
	}

	destWriter := outputWriter
	if destWriter == nil {
		destWriter = bridge.Slave
	}

	// Reading from serial -> PTY slave / terminal
	go func() {
		buf := make([]byte, 1024)
		var inspectedBytes int
		var garbageBytes int
		const maxInspect = 120
		var garbageAlerted bool

		for {
			n, rErr := s.rwc.Read(buf)
			if n > 0 {
				s.probeMu.Lock()
				if s.isProbing {
					chunk := make([]byte, n)
					copy(chunk, buf[:n])
					select {
					case s.probeCh <- chunk:
					default:
					}
					s.probeMu.Unlock()
					continue
				}
				s.probeMu.Unlock()

				if s.autoBaud && !garbageAlerted && inspectedBytes < maxInspect {
					for _, b := range buf[:n] {
						inspectedBytes++
						// Framing errors or baud mismatch usually produces nulls (0x00), 0xFF,
						// unprintable control characters (< 0x20 except \r, \n, \t, \x1b), or high-byte noise
						if (b < 0x20 && b != '\r' && b != '\n' && b != '\t' && b != 0x1b) || b == 0x00 || b >= 0x7F {
							garbageBytes++
						}
						if inspectedBytes >= 40 {
							garbageRatio := float64(garbageBytes) / float64(inspectedBytes)
							if garbageRatio > 0.55 {
								garbageAlerted = true
								go s.triggerAutoBaudDetection()
								break
							}
						}
					}
				}
				if _, wErr := destWriter.Write(buf[:n]); wErr != nil {
					break
				}
			}
			if rErr != nil {
				s.mu.Lock()
				wasClosed := s.closed
				s.mu.Unlock()
				s.Close()
				if !wasClosed && s.OnExit != nil {
					s.OnExit(rErr)
				}
				return
			}
		}
	}()

	// User input from PTY -> serial device
	go func() {
		_, _ = io.Copy(rwc, bridge.Slave)
	}()

	return s, nil
}

func parseParity(p string) serial.Parity {
	switch p {
	case "even", "E":
		return serial.EvenParity
	case "odd", "O":
		return serial.OddParity
	case "mark", "M":
		return serial.MarkParity
	case "space", "S":
		return serial.SpaceParity
	default:
		return serial.NoParity
	}
}

func parseStopBits(sb int) serial.StopBits {
	switch sb {
	case 2:
		return serial.TwoStopBits
	default:
		return serial.OneStopBit
	}
}

// SetBaudRate changes baud rate dynamically on the active serial port
func (s *SerialSession) SetBaudRate(newBaud int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.port == nil {
		return fmt.Errorf("serial port does not support dynamic baud rate change")
	}

	mode := &serial.Mode{
		BaudRate: newBaud,
		DataBits: s.dataBits,
		Parity:   parseParity(s.parity),
		StopBits: parseStopBits(s.stopBits),
	}
	_ = s.port.SetReadTimeout(serial.NoTimeout)
	if err := s.port.SetMode(mode); err != nil {
		return err
	}
	s.baud = newBaud
	log.Printf("[SERIAL] Baud rate changed to %d on %s", newBaud, s.devPath)
	return nil
}

// CurrentBaudRate returns active baud rate
func (s *SerialSession) CurrentBaudRate() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.baud
}

// triggerAutoBaudDetection scans alternative baud rates and notifies if a cleaner stream is found
func (s *SerialSession) triggerAutoBaudDetection() {
	s.mu.Lock()
	if s.closed || s.port == nil {
		s.mu.Unlock()
		return
	}
	currentBaud := s.baud
	s.mu.Unlock()

	s.probeMu.Lock()
	if s.isProbing {
		s.probeMu.Unlock()
		return
	}
	s.isProbing = true
	// Drain any residual bytes in probeCh
	for len(s.probeCh) > 0 {
		<-s.probeCh
	}
	s.probeMu.Unlock()

	defer func() {
		s.probeMu.Lock()
		s.isProbing = false
		for len(s.probeCh) > 0 {
			<-s.probeCh
		}
		s.probeMu.Unlock()
	}()

	log.Printf("[SERIAL] Garbage stream detected at %d baud on %s. Starting auto-detect probe...", currentBaud, s.devPath)

	candidates := []int{115200, 9600, 57600, 38400, 19200, 230400, 4800}
	bestBaud := 0
	bestScore := 0.0

	for _, cand := range candidates {
		if cand == currentBaud {
			continue
		}
		score := s.probeBaudRate(cand)
		log.Printf("[SERIAL] Probed %d baud: score=%.1f%%", cand, score)
		if score > bestScore {
			bestScore = score
			bestBaud = cand
		}
		if bestScore >= 80.0 {
			break
		}
	}

	// Restore original baud rate
	_ = s.SetBaudRate(currentBaud)

	if bestBaud > 0 && bestScore >= 50.0 {
		log.Printf("[SERIAL] Best baud rate detected: %d (confidence: %.1f%%)", bestBaud, bestScore)
		s.mu.Lock()
		cb := s.OnBaudRateDetected
		s.mu.Unlock()
		if cb != nil {
			cb(bestBaud, currentBaud)
		}
	}
}

// probeBaudRate temporarily sets candidate baud rate, probes with CRLF, and scores readability
func (s *SerialSession) probeBaudRate(baud int) float64 {
	s.mu.Lock()
	if s.closed || s.port == nil {
		s.mu.Unlock()
		return 0
	}
	mode := &serial.Mode{
		BaudRate: baud,
		DataBits: s.dataBits,
		Parity:   parseParity(s.parity),
		StopBits: parseStopBits(s.stopBits),
	}
	if err := s.port.SetMode(mode); err != nil {
		s.mu.Unlock()
		return 0
	}

	s.probeMu.Lock()
	for len(s.probeCh) > 0 {
		<-s.probeCh
	}
	s.probeMu.Unlock()

	// Send probe to elicit response from device
	_, _ = s.port.Write([]byte("\r\n"))
	s.mu.Unlock()

	var collected []byte
	timeout := time.After(250 * time.Millisecond)

collectLoop:
	for {
		select {
		case chunk := <-s.probeCh:
			collected = append(collected, chunk...)
			if len(collected) >= 128 {
				break collectLoop
			}
		case <-timeout:
			break collectLoop
		}
	}

	if len(collected) == 0 {
		return 0
	}

	// Score received buffer
	printable := 0
	for _, b := range collected {
		if (b >= 0x20 && b <= 0x7E) || b == '\r' || b == '\n' || b == '\t' || b == 0x1b {
			printable++
		}
	}
	score := float64(printable) / float64(len(collected)) * 100.0

	strLower := strings.ToLower(string(collected))
	keywords := []string{"login", "password", "user", "console", "router", "switch", "mikrotik", "cisco", "linux", "boot", ">", "#", "$"}
	for _, kw := range keywords {
		if strings.Contains(strLower, kw) {
			score += 25.0
			break
		}
	}

	if score > 100.0 {
		score = 100.0
	}
	return score
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
