package ssh

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/darakcheeff/pac/internal/storage"
	"golang.org/x/crypto/ssh"
)

// ForwardManager manages active port forward listeners
type ForwardManager struct {
	client    *ssh.Client
	listeners []net.Listener
	mu        sync.Mutex
	closed    bool
}

func NewForwardManager(client *ssh.Client) *ForwardManager {
	return &ForwardManager{
		client: client,
	}
}

// StartForwardings starts all configured port forwardings
func (fm *ForwardManager) StartForwardings(forwards []storage.PortForward) error {
	for _, f := range forwards {
		switch f.Type {
		case "local", "L":
			if err := fm.StartLocalForward(f.LocalPort, f.RemoteHost, f.RemotePort); err != nil {
				return err
			}
		case "remote", "R":
			if err := fm.StartRemoteForward(f.RemotePort, f.RemoteHost, f.LocalPort); err != nil {
				return err
			}
		case "dynamic", "D":
			if err := fm.StartDynamicSOCKS5(f.LocalPort); err != nil {
				return err
			}
		}
	}
	return nil
}

// StartLocalForward (-L localPort:remoteHost:remotePort)
func (fm *ForwardManager) StartLocalForward(localPort int, remoteHost string, remotePort int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		return fmt.Errorf("local forward listen failed on port %d: %w", localPort, err)
	}

	fm.mu.Lock()
	fm.listeners = append(fm.listeners, listener)
	fm.mu.Unlock()

	go func() {
		for {
			localConn, err := listener.Accept()
			if err != nil {
				return
			}
			go fm.handleLocalForwardConn(localConn, remoteHost, remotePort)
		}
	}()

	return nil
}

func (fm *ForwardManager) handleLocalForwardConn(localConn net.Conn, remoteHost string, remotePort int) {
	defer localConn.Close()

	remoteConn, err := fm.client.Dial("tcp", fmt.Sprintf("%s:%d", remoteHost, remotePort))
	if err != nil {
		return
	}
	defer remoteConn.Close()

	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(remoteConn, localConn)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(localConn, remoteConn)
		errc <- err
	}()
	<-errc
}

// StartRemoteForward (-R remotePort:localHost:localPort)
func (fm *ForwardManager) StartRemoteForward(remotePort int, localHost string, localPort int) error {
	remoteListener, err := fm.client.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", remotePort))
	if err != nil {
		return fmt.Errorf("remote forward listen failed on port %d: %w", remotePort, err)
	}

	fm.mu.Lock()
	fm.listeners = append(fm.listeners, remoteListener)
	fm.mu.Unlock()

	go func() {
		for {
			remoteConn, err := remoteListener.Accept()
			if err != nil {
				return
			}
			go fm.handleRemoteForwardConn(remoteConn, localHost, localPort)
		}
	}()

	return nil
}

func (fm *ForwardManager) handleRemoteForwardConn(remoteConn net.Conn, localHost string, localPort int) {
	defer remoteConn.Close()

	if localHost == "" {
		localHost = "127.0.0.1"
	}
	localConn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", localHost, localPort))
	if err != nil {
		return
	}
	defer localConn.Close()

	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(localConn, remoteConn)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(remoteConn, localConn)
		errc <- err
	}()
	<-errc
}

// StartDynamicSOCKS5 (-D localPort)
func (fm *ForwardManager) StartDynamicSOCKS5(localPort int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		return fmt.Errorf("socks5 listen failed on port %d: %w", localPort, err)
	}

	fm.mu.Lock()
	fm.listeners = append(fm.listeners, listener)
	fm.mu.Unlock()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go fm.handleSOCKS5Conn(conn)
		}
	}()

	return nil
}

func (fm *ForwardManager) handleSOCKS5Conn(conn net.Conn) {
	defer conn.Close()

	// Read SOCKS5 handshake (version + auth methods)
	buf := make([]byte, 256)
	if _, err := io.ReadFull(conn, buf[:2]); err != nil || buf[0] != 0x05 {
		return
	}

	numMethods := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:numMethods]); err != nil {
		return
	}

	// Respond: SOCKS5 NO_AUTH (0x00)
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// Read Request Details (VER, CMD, RSV, ATYP, DST.ADDR, DST.PORT)
	if _, err := io.ReadFull(conn, buf[:4]); err != nil || buf[0] != 0x05 || buf[1] != 0x01 { // CMD 0x01 = CONNECT
		return
	}

	var targetHost string
	switch buf[3] { // ATYP
	case 0x01: // IPv4
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return
		}
		targetHost = net.IP(buf[:4]).String()
	case 0x03: // Domain name
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return
		}
		domainLen := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:domainLen]); err != nil {
			return
		}
		targetHost = string(buf[:domainLen])
	case 0x04: // IPv6
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return
		}
		targetHost = net.IP(buf[:16]).String()
	default:
		return
	}

	// Read Port
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	targetPort := binary.BigEndian.Uint16(buf[:2])

	// Connect to target through SSH
	targetConn, err := fm.client.Dial("tcp", fmt.Sprintf("%s:%d", targetHost, targetPort))
	if err != nil {
		// SOCKS5 reply: 0x05, 0x05 (Connection Refused), 0x00, 0x01 (IPv4 0.0.0.0:0)
		_, _ = conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer targetConn.Close()

	// SOCKS5 reply: 0x05, 0x00 (Success), 0x00, 0x01 (IPv4 0.0.0.0:0)
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	// Bi-directional tunnel
	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(targetConn, conn)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(conn, targetConn)
		errc <- err
	}()
	<-errc
}

// Close terminates all listeners
func (fm *ForwardManager) Close() {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if fm.closed {
		return
	}
	fm.closed = true

	for _, l := range fm.listeners {
		_ = l.Close()
	}
	fm.listeners = nil
}
