package ssh

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

func TestListenLoopback(t *testing.T) {
	listeners, err := listenLoopback(0)
	if err != nil {
		t.Fatalf("listenLoopback failed: %v", err)
	}
	defer func() {
		for _, l := range listeners {
			_ = l.Close()
		}
	}()

	if len(listeners) == 0 {
		t.Fatalf("expected at least 1 listener")
	}

	addr := listeners[0].Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to dial listener: %v", err)
	}
	defer conn.Close()
}

func TestProxyBidirectionalHalfClose(t *testing.T) {
	// Create a local TCP echo server that acts as "remote server"
	serverListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer serverListener.Close()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := serverListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Server reads all data from client until EOF
		req, _ := io.ReadAll(conn)
		// Server then replies with response
		resp := append([]byte("RESPONSE: "), req...)
		_, _ = conn.Write(resp)
	}()

	// Create middle proxy using TCP pairs to test proxyBidirectional with real TCP half-close
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer proxyListener.Close()

	go func() {
		clientSide, err := proxyListener.Accept()
		if err != nil {
			return
		}
		defer clientSide.Close()

		targetSide, err := net.Dial("tcp", serverListener.Addr().String())
		if err != nil {
			return
		}
		defer targetSide.Close()

		proxyBidirectional(clientSide, targetSide)
	}()

	// Now connect client to proxy
	clientConn, err := net.Dial("tcp", proxyListener.Addr().String())
	if err != nil {
		t.Fatalf("dial proxy failed: %v", err)
	}
	defer clientConn.Close()

	// Client sends request
	reqData := []byte("HELLO WORLD")
	if _, err := clientConn.Write(reqData); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Client half-closes write side (like curl or browser finishing HTTP request)
	tcpClientConn := clientConn.(*net.TCPConn)
	if err := tcpClientConn.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite failed: %v", err)
	}

	// Client must still be able to read full response despite write side closed!
	var respBuf bytes.Buffer
	buf := make([]byte, 1024)
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		n, err := clientConn.Read(buf)
		if n > 0 {
			respBuf.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	expected := "RESPONSE: HELLO WORLD"
	if respBuf.String() != expected {
		t.Fatalf("expected %q, got %q", expected, respBuf.String())
	}

	<-serverDone
}
