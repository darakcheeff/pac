package ssh

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// SetupX11Forwarding requests X11 forwarding on the session and handles incoming x11 channels
func SetupX11Forwarding(client *ssh.Client, session *ssh.Session) error {
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0"
	}

	// Request X11 forwarding on session
	req := struct {
		SingleConnection bool
		AuthProtocol     string
		AuthCookie       string
		ScreenNumber     uint32
	}{
		SingleConnection: false,
		AuthProtocol:     "MIT-MAGIC-COOKIE-1",
		AuthCookie:       "00000000000000000000000000000000",
		ScreenNumber:     0,
	}

	ok, err := session.SendRequest("x11-req", true, ssh.Marshal(&req))
	if err != nil {
		return fmt.Errorf("x11-req error: %w", err)
	}
	if !ok {
		return fmt.Errorf("server refused x11-req")
	}

	// Listen for incoming x11 channels from remote server
	x11Channels := client.HandleChannelOpen("x11")
	go func() {
		for newChan := range x11Channels {
			go handleX11Channel(newChan, display)
		}
	}()

	return nil
}

func handleX11Channel(newChan ssh.NewChannel, display string) {
	ch, reqs, err := newChan.Accept()
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	defer ch.Close()

	// Connect to local X11 display socket
	localConn, err := connectLocalDisplay(display)
	if err != nil {
		return
	}
	defer localConn.Close()

	// Proxy between X11 client (remote app) and local X server
	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(localConn, ch)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(ch, localConn)
		errc <- err
	}()
	<-errc
}

func connectLocalDisplay(display string) (net.Conn, error) {
	// Parse display string, e.g. ":0", "localhost:10.0", "/tmp/launch-XXXX/:0"
	displayNum := "0"
	if strings.HasPrefix(display, ":") {
		parts := strings.Split(strings.TrimPrefix(display, ":"), ".")
		displayNum = parts[0]
	} else if strings.Contains(display, ":") {
		parts := strings.Split(display, ":")
		if len(parts) > 1 {
			sub := strings.Split(parts[1], ".")
			displayNum = sub[0]
		}
	}

	// Try Unix domain socket first
	socketPath := filepath.Join("/tmp/.X11-unix", "X"+displayNum)
	if conn, err := net.Dial("unix", socketPath); err == nil {
		return conn, nil
	}

	// Fallback to TCP port 6000 + displayNum
	port := 6000
	var dInt int
	if _, err := fmt.Sscanf(displayNum, "%d", &dInt); err == nil {
		port += dInt
	}
	return net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
}

type x11ChannelData struct {
	OriginatorAddress string
	OriginatorPort    uint32
}

func parseX11ChannelData(extraData []byte) (*x11ChannelData, error) {
	var data x11ChannelData
	if len(extraData) < 4 {
		return &data, nil
	}
	addrLen := binary.BigEndian.Uint32(extraData[:4])
	if int(addrLen)+8 <= len(extraData) {
		data.OriginatorAddress = string(extraData[4 : 4+addrLen])
		data.OriginatorPort = binary.BigEndian.Uint32(extraData[4+addrLen : 8+addrLen])
	}
	return &data, nil
}
