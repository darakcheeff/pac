package ssh

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
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

// listenLoopback binds on both 127.0.0.1 and [::1] (dual-stack localhost)
func listenLoopback(port int) ([]net.Listener, error) {
	var listeners []net.Listener

	v4Addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	l4, err4 := net.Listen("tcp4", v4Addr)
	if err4 != nil {
		return nil, fmt.Errorf("listen %s failed: %w", v4Addr, err4)
	}
	listeners = append(listeners, l4)

	// Use actual bound port (in case port 0 was passed)
	actualPort := l4.Addr().(*net.TCPAddr).Port

	v6Addr := net.JoinHostPort("::1", strconv.Itoa(actualPort))
	if l6, err6 := net.Listen("tcp6", v6Addr); err6 == nil {
		listeners = append(listeners, l6)
	}

	return listeners, nil
}

// proxyBidirectional relays traffic between two connections with clean half-close support
func proxyBidirectional(conn1, conn2 net.Conn) {
	var once sync.Once
	closeBoth := func() {
		_ = conn1.Close()
		_ = conn2.Close()
	}

	var wg sync.WaitGroup
	wg.Add(2)

	pipe := func(dst, src net.Conn) {
		defer wg.Done()
		_, err := io.Copy(dst, src)
		if err != nil {
			once.Do(closeBoth)
			return
		}

		type closeWriter interface {
			CloseWrite() error
		}
		if cw, ok := dst.(closeWriter); ok {
			_ = cw.CloseWrite()
		} else {
			once.Do(closeBoth)
		}
	}

	go pipe(conn1, conn2)
	go pipe(conn2, conn1)
	wg.Wait()
}

// StartLocalForward (-L localPort:remoteHost:remotePort)
func (fm *ForwardManager) StartLocalForward(localPort int, remoteHost string, remotePort int) error {
	listeners, err := listenLoopback(localPort)
	if err != nil {
		return fmt.Errorf("local forward listen failed on port %d: %w", localPort, err)
	}

	targetAddr := net.JoinHostPort(remoteHost, strconv.Itoa(remotePort))
	log.Printf("[PortFwd] Local forward started: localhost:%d -> %s", localPort, targetAddr)

	fm.mu.Lock()
	fm.listeners = append(fm.listeners, listeners...)
	fm.mu.Unlock()

	for _, l := range listeners {
		go func(listener net.Listener) {
			for {
				localConn, err := listener.Accept()
				if err != nil {
					return
				}
				go fm.handleLocalForwardConn(localConn, targetAddr)
			}
		}(l)
	}

	return nil
}

func (fm *ForwardManager) handleLocalForwardConn(localConn net.Conn, targetAddr string) {
	defer localConn.Close()

	remoteConn, err := fm.client.Dial("tcp", targetAddr)
	if err != nil {
		log.Printf("[PortFwd] Local forward dial %s failed: %v", targetAddr, err)
		return
	}
	defer remoteConn.Close()

	proxyBidirectional(localConn, remoteConn)
}

// StartRemoteForward (-R remotePort:localHost:localPort)
func (fm *ForwardManager) StartRemoteForward(remotePort int, localHost string, localPort int) error {
	if localHost == "" {
		localHost = "127.0.0.1"
	}
	localAddr := net.JoinHostPort(localHost, strconv.Itoa(localPort))

	// Bind on remote host. Standard OpenSSH servers default to 127.0.0.1 (GatewayPorts no)
	remoteAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(remotePort))
	remoteListener, err := fm.client.Listen("tcp", remoteAddr)
	if err != nil {
		// Fallback to ":remotePort" if server permits
		fallbackAddr := fmt.Sprintf(":%d", remotePort)
		if fbListener, fbErr := fm.client.Listen("tcp", fallbackAddr); fbErr == nil {
			remoteListener = fbListener
		} else {
			return fmt.Errorf("remote forward listen failed on port %d: %w", remotePort, err)
		}
	}

	log.Printf("[PortFwd] Remote forward started: remote:%d -> %s", remotePort, localAddr)

	fm.mu.Lock()
	fm.listeners = append(fm.listeners, remoteListener)
	fm.mu.Unlock()

	go func() {
		for {
			remoteConn, err := remoteListener.Accept()
			if err != nil {
				return
			}
			go fm.handleRemoteForwardConn(remoteConn, localAddr)
		}
	}()

	return nil
}

func (fm *ForwardManager) handleRemoteForwardConn(remoteConn net.Conn, localAddr string) {
	defer remoteConn.Close()

	localConn, err := net.Dial("tcp", localAddr)
	if err != nil {
		log.Printf("[PortFwd] Remote forward dial local %s failed: %v", localAddr, err)
		return
	}
	defer localConn.Close()

	proxyBidirectional(remoteConn, localConn)
}

// StartDynamicSOCKS5 (-D localPort)
func (fm *ForwardManager) StartDynamicSOCKS5(localPort int) error {
	listeners, err := listenLoopback(localPort)
	if err != nil {
		return fmt.Errorf("socks listen failed on port %d: %w", localPort, err)
	}

	log.Printf("[PortFwd] Dynamic SOCKS proxy started on localhost:%d", localPort)

	fm.mu.Lock()
	fm.listeners = append(fm.listeners, listeners...)
	fm.mu.Unlock()

	for _, l := range listeners {
		go func(listener net.Listener) {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				go fm.handleDynamicConn(conn)
			}
		}(l)
	}

	return nil
}

func (fm *ForwardManager) handleDynamicConn(conn net.Conn) {
	defer conn.Close()

	// Read initial version byte
	var ver [1]byte
	if _, err := io.ReadFull(conn, ver[:]); err != nil {
		return
	}

	if ver[0] == 0x05 {
		fm.handleSOCKS5(conn)
	} else if ver[0] == 0x04 {
		fm.handleSOCKS4(conn)
	}
}

func (fm *ForwardManager) handleSOCKS5(conn net.Conn) {
	var numMethods [1]byte
	if _, err := io.ReadFull(conn, numMethods[:]); err != nil {
		return
	}

	methods := make([]byte, numMethods[0])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}

	// Reply: SOCKS5 NO_AUTH (0x00)
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// Read Request Details (VER, CMD, RSV, ATYP)
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil || buf[0] != 0x05 || buf[1] != 0x01 { // CMD 0x01 = CONNECT
		return
	}

	var targetHost string
	switch buf[3] { // ATYP
	case 0x01: // IPv4
		var ip [4]byte
		if _, err := io.ReadFull(conn, ip[:]); err != nil {
			return
		}
		targetHost = net.IP(ip[:]).String()
	case 0x03: // Domain name
		var domainLen [1]byte
		if _, err := io.ReadFull(conn, domainLen[:]); err != nil {
			return
		}
		domain := make([]byte, domainLen[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return
		}
		targetHost = string(domain)
	case 0x04: // IPv6
		var ip [16]byte
		if _, err := io.ReadFull(conn, ip[:]); err != nil {
			return
		}
		targetHost = net.IP(ip[:]).String()
	default:
		return
	}

	// Read Port (uint16 BigEndian)
	var portBytes [2]byte
	if _, err := io.ReadFull(conn, portBytes[:]); err != nil {
		return
	}
	targetPort := binary.BigEndian.Uint16(portBytes[:])
	targetAddr := net.JoinHostPort(targetHost, strconv.Itoa(int(targetPort)))

	// Connect to target through SSH
	targetConn, err := fm.client.Dial("tcp", targetAddr)
	if err != nil {
		// SOCKS5 reply: 0x05, 0x05 (Connection Refused), 0x00, 0x01 (IPv4 0.0.0.0:0)
		_, _ = conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		log.Printf("[PortFwd] SOCKS5 dial %s failed: %v", targetAddr, err)
		return
	}
	defer targetConn.Close()

	// SOCKS5 reply: 0x05, 0x00 (Success), 0x00, 0x01 (IPv4 0.0.0.0:0)
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	proxyBidirectional(conn, targetConn)
}

func (fm *ForwardManager) handleSOCKS4(conn net.Conn) {
	// buf: CMD (1 byte), DSTPORT (2 bytes), DSTIP (4 bytes)
	var req [7]byte
	if _, err := io.ReadFull(conn, req[:]); err != nil || req[0] != 0x01 { // CMD 0x01 = CONNECT
		return
	}

	targetPort := binary.BigEndian.Uint16(req[1:3])
	ip := req[3:7]

	// Read UserID until null terminator
	for {
		var b [1]byte
		if _, err := io.ReadFull(conn, b[:]); err != nil || b[0] == 0x00 {
			break
		}
	}

	var targetHost string
	// Check SOCKS4a (IP is 0.0.0.x with x != 0)
	if ip[0] == 0 && ip[1] == 0 && ip[2] == 0 && ip[3] != 0 {
		var domainBuf bytes.Buffer
		for {
			var b [1]byte
			if _, err := io.ReadFull(conn, b[:]); err != nil || b[0] == 0x00 {
				break
			}
			domainBuf.WriteByte(b[0])
		}
		targetHost = domainBuf.String()
	} else {
		targetHost = net.IP(ip).String()
	}

	targetAddr := net.JoinHostPort(targetHost, strconv.Itoa(int(targetPort)))
	targetConn, err := fm.client.Dial("tcp", targetAddr)
	if err != nil {
		// SOCKS4 reply: 0x00, 0x5b (Request Rejected)
		_, _ = conn.Write([]byte{0x00, 0x5b, 0, 0, 0, 0, 0, 0})
		log.Printf("[PortFwd] SOCKS4 dial %s failed: %v", targetAddr, err)
		return
	}
	defer targetConn.Close()

	// SOCKS4 reply: 0x00, 0x5a (Request Granted)
	if _, err := conn.Write([]byte{0x00, 0x5a, req[1], req[2], req[3], req[4], req[5], req[6]}); err != nil {
		return
	}

	proxyBidirectional(conn, targetConn)
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
