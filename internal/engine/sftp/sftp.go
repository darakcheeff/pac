package sftp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// FileItem represents a remote file or folder in SFTP listing
type FileItem struct {
	Name    string      `json:"name"`
	Path    string      `json:"path"`
	Size    int64       `json:"size"`
	Mode    os.FileMode `json:"mode"`
	ModTime time.Time   `json:"mod_time"`
	IsDir   bool        `json:"is_dir"`
}

// ProgressCallback reports bytes transferred and total size
type ProgressCallback func(transferred int64, total int64, speedBytesPerSec float64)

// DialSSHFunc dials a fresh SSH client for transparent SFTP reconnection.
type DialSSHFunc func() (*ssh.Client, error)

// Client wraps pkg/sftp.Client with helper routines and automatic reconnection
type Client struct {
	sshClient     *ssh.Client
	sftpClient    *sftp.Client
	dialFn        DialSSHFunc
	keepAliveStop chan struct{}
	isLocal       bool
	currentDir    string
	mu            sync.RWMutex
	reconnectMu   sync.Mutex
	closed        bool
}

// NewLocalClient initializes a Client operating on the local filesystem
func NewLocalClient(initialDir string) *Client {
	if initialDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			initialDir = home
		} else {
			initialDir = "/"
		}
	}
	return &Client{
		isLocal:    true,
		currentDir: initialDir,
	}
}

// IsLocal returns true if operating on local filesystem
func (c *Client) IsLocal() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isLocal
}

// NewClient initializes SFTP client on top of existing SSH client
func NewClient(sshClient *ssh.Client) (*Client, error) {
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return nil, fmt.Errorf("failed to start sftp subsystem: %w", err)
	}

	pwd, err := sftpClient.Getwd()
	if err != nil {
		pwd = "/"
	}

	return &Client{
		sshClient:  sshClient,
		sftpClient: sftpClient,
		currentDir: pwd,
	}, nil
}

// CurrentDir returns current remote working directory
func (c *Client) CurrentDir() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentDir
}

// SetCurrentDir updates the remote working directory
func (c *Client) SetCurrentDir(dir string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentDir = dir
}

// SetReconnectDialer configures the SSH dialer used to re-establish a dropped SFTP connection.
func (c *Client) SetReconnectDialer(fn DialSSHFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dialFn = fn
}

// StartKeepAlive sends periodic SSH keepalive requests on the underlying SFTP SSH connection
// so NAT/firewalls and sshd idle timeouts do not silently drop the SFTP channel.
func (c *Client) StartKeepAlive(interval time.Duration) {
	if interval <= 0 {
		return
	}
	c.mu.Lock()
	if c.closed || c.isLocal {
		c.mu.Unlock()
		return
	}
	if c.keepAliveStop != nil {
		close(c.keepAliveStop)
	}
	stopCh := make(chan struct{})
	c.keepAliveStop = stopCh
	c.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				c.mu.RLock()
				closed := c.closed
				sshCl := c.sshClient
				c.mu.RUnlock()
				if closed {
					return
				}
				if sshCl != nil {
					_, _, _ = sshCl.SendRequest("keepalive@openssh.com", true, nil)
				}
			}
		}
	}()
}

func isConnectionLostErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sftp.ErrSSHFxConnectionLost) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection lost") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "sftp client is closed") ||
		strings.Contains(msg, "sftp client not connected")
}

func (c *Client) getRemoteClient() (*sftp.Client, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return nil, fmt.Errorf("client is closed")
	}
	if c.sftpClient == nil {
		return nil, fmt.Errorf("sftp client is closed")
	}
	return c.sftpClient, nil
}

// reconnect replaces the dead SFTP/SSH client using dialFn.
// failedClient prevents redundant reconnects when another goroutine already reconnected.
func (c *Client) reconnect(failedClient *sftp.Client) (*sftp.Client, error) {
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client is closed")
	}
	if c.sftpClient != nil && c.sftpClient != failedClient {
		current := c.sftpClient
		c.mu.RUnlock()
		return current, nil
	}
	fn := c.dialFn
	c.mu.RUnlock()

	if fn == nil {
		return nil, fmt.Errorf("sftp reconnect not configured")
	}

	log.Printf("[SFTP] Connection lost; attempting automatic reconnect...")
	newSSH, err := fn()
	if err != nil {
		return nil, err
	}
	newSFTP, err := sftp.NewClient(newSSH)
	if err != nil {
		_ = newSSH.Close()
		return nil, err
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		if newSFTP != nil {
			_ = newSFTP.Close()
		}
		if newSSH != nil {
			_ = newSSH.Close()
		}
		return nil, fmt.Errorf("client is closed")
	}
	oldSFTP := c.sftpClient
	oldSSH := c.sshClient
	c.sftpClient = newSFTP
	c.sshClient = newSSH
	c.mu.Unlock()

	if oldSFTP != nil {
		_ = oldSFTP.Close()
	}
	if oldSSH != nil {
		_ = oldSSH.Close()
	}
	log.Printf("[SFTP] Automatic reconnect succeeded")
	return newSFTP, nil
}

func (c *Client) withReconnect(op func(*sftp.Client) error) error {
	sc, err := c.getRemoteClient()
	if err != nil {
		if !isConnectionLostErr(err) {
			return err
		}
		sc, recErr := c.reconnect(nil)
		if recErr != nil {
			return err
		}
		return op(sc)
	}

	err = op(sc)
	if err != nil && isConnectionLostErr(err) {
		newSC, recErr := c.reconnect(sc)
		if recErr != nil {
			return err
		}
		return op(newSC)
	}
	return err
}

// resolvePath expands ~ and ~/ based on current or home directory
func (c *Client) resolvePath(remotePath string) string {
	if remotePath == "" {
		return c.currentDir
	}

	if remotePath == "~" {
		if c.isLocal {
			if h, err := os.UserHomeDir(); err == nil && h != "" {
				return h
			}
		} else if c.sftpClient != nil {
			if pwd, err := c.sftpClient.Getwd(); err == nil && pwd != "" {
				return pwd
			}
		}
		return c.currentDir
	}

	if strings.HasPrefix(remotePath, "~/") {
		base := c.currentDir
		if c.isLocal {
			if h, err := os.UserHomeDir(); err == nil && h != "" {
				base = h
			}
		} else if c.sftpClient != nil {
			if pwd, err := c.sftpClient.Getwd(); err == nil && pwd != "" {
				base = pwd
			}
		}
		return path.Join(base, strings.TrimPrefix(remotePath, "~/"))
	}

	return remotePath
}

// ListDir returns list of files/folders in remote directory
func (c *Client) ListDir(remotePath string) ([]FileItem, error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client is closed")
	}
	remotePath = c.resolvePath(remotePath)
	isLocal := c.isLocal
	c.mu.RUnlock()

	if isLocal {
		entries, err := os.ReadDir(remotePath)
		if err != nil {
			return nil, err
		}
		var items []FileItem
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			items = append(items, FileItem{
				Name:    entry.Name(),
				Path:    filepath.Join(remotePath, entry.Name()),
				Size:    info.Size(),
				Mode:    info.Mode(),
				ModTime: info.ModTime(),
				IsDir:   entry.IsDir(),
			})
		}
		return items, nil
	}

	var entries []os.FileInfo
	err := c.withReconnect(func(sc *sftp.Client) error {
		var rErr error
		entries, rErr = sc.ReadDir(remotePath)
		return rErr
	})
	if err != nil {
		return nil, err
	}

	var items []FileItem
	for _, entry := range entries {
		items = append(items, FileItem{
			Name:    entry.Name(),
			Path:    path.Join(remotePath, entry.Name()),
			Size:    entry.Size(),
			Mode:    entry.Mode(),
			ModTime: entry.ModTime(),
			IsDir:   entry.IsDir(),
		})
	}
	return items, nil
}

// DownloadFile downloads remote file to local path with progress
func (c *Client) DownloadFile(ctx context.Context, remotePath, localPath string, cb ProgressCallback) error {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return fmt.Errorf("client is closed")
	}

	remotePath = c.resolvePath(remotePath)
	isLocal := c.isLocal
	c.mu.RUnlock()

	if isLocal {
		src, err := os.Open(remotePath)
		if err != nil {
			return err
		}
		defer src.Close()
		stat, err := src.Stat()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			return err
		}
		dst, err := os.Create(localPath)
		if err != nil {
			return err
		}
		defer dst.Close()
		return copyWithProgress(ctx, dst, src, stat.Size(), cb)
	}

	var src *sftp.File
	var totalSize int64
	err := c.withReconnect(func(sc *sftp.Client) error {
		f, oErr := sc.Open(remotePath)
		if oErr != nil {
			return oErr
		}
		stat, sErr := f.Stat()
		if sErr != nil {
			_ = f.Close()
			return sErr
		}
		src = f
		totalSize = stat.Size()
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to open remote file: %w", err)
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}

	dst, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer dst.Close()

	return copyWithProgress(ctx, dst, src, totalSize, cb)
}

// UploadFile uploads local file to remote path with progress
func (c *Client) UploadFile(ctx context.Context, localPath, remotePath string, cb ProgressCallback) error {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return fmt.Errorf("client is closed")
	}

	remotePath = c.resolvePath(remotePath)
	isLocal := c.isLocal
	c.mu.RUnlock()

	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer src.Close()

	stat, err := src.Stat()
	if err != nil {
		return err
	}
	totalSize := stat.Size()

	if isLocal {
		if err := os.MkdirAll(filepath.Dir(remotePath), 0755); err != nil {
			return err
		}
		dst, err := os.Create(remotePath)
		if err != nil {
			return err
		}
		defer dst.Close()
		return copyWithProgress(ctx, dst, src, totalSize, cb)
	}

	var dst *sftp.File
	err = c.withReconnect(func(sc *sftp.Client) error {
		f, cErr := sc.Create(remotePath)
		if cErr != nil {
			return cErr
		}
		dst = f
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create remote file: %w", err)
	}
	defer dst.Close()

	return copyWithProgress(ctx, dst, src, totalSize, cb)
}

// Mkdir creates remote directory
func (c *Client) Mkdir(remotePath string) error {
	c.mu.RLock()
	remotePath = c.resolvePath(remotePath)
	isLocal := c.isLocal
	c.mu.RUnlock()
	if isLocal {
		return os.MkdirAll(remotePath, 0755)
	}
	return c.withReconnect(func(sc *sftp.Client) error {
		return sc.MkdirAll(remotePath)
	})
}

// Remove deletes remote file or directory (recursively if non-empty)
func (c *Client) Remove(remotePath string) error {
	c.mu.RLock()
	remotePath = c.resolvePath(remotePath)
	isLocal := c.isLocal
	c.mu.RUnlock()
	if isLocal {
		return os.RemoveAll(remotePath)
	}
	return c.withReconnect(func(sc *sftp.Client) error {
		if err := sc.Remove(remotePath); err == nil {
			return nil
		}
		if errDir := sc.RemoveDirectory(remotePath); errDir == nil {
			return nil
		}
		return removeRecursiveWithClient(sc, remotePath)
	})
}

func removeRecursiveWithClient(sc *sftp.Client, remotePath string) error {
	entries, err := sc.ReadDir(remotePath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		subPath := path.Join(remotePath, entry.Name())
		if entry.IsDir() {
			if err := removeRecursiveWithClient(sc, subPath); err != nil {
				return err
			}
		} else {
			if err := sc.Remove(subPath); err != nil {
				return err
			}
		}
	}
	return sc.RemoveDirectory(remotePath)
}

// Rename renames or moves remote file
func (c *Client) Rename(oldPath, newPath string) error {
	c.mu.RLock()
	oldPath = c.resolvePath(oldPath)
	newPath = c.resolvePath(newPath)
	isLocal := c.isLocal
	c.mu.RUnlock()
	if isLocal {
		return os.Rename(oldPath, newPath)
	}
	return c.withReconnect(func(sc *sftp.Client) error {
		return sc.Rename(oldPath, newPath)
	})
}

// Chmod updates file permissions
func (c *Client) Chmod(remotePath string, mode os.FileMode) error {
	c.mu.RLock()
	remotePath = c.resolvePath(remotePath)
	isLocal := c.isLocal
	c.mu.RUnlock()
	if isLocal {
		return os.Chmod(remotePath, mode)
	}
	return c.withReconnect(func(sc *sftp.Client) error {
		return sc.Chmod(remotePath, mode)
	})
}

// Close closes the SFTP client
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	if c.keepAliveStop != nil {
		close(c.keepAliveStop)
		c.keepAliveStop = nil
	}
	sftpClient := c.sftpClient
	sshClient := c.sshClient
	c.mu.Unlock()

	var err error
	if sftpClient != nil {
		err = sftpClient.Close()
	}
	if sshClient != nil {
		_ = sshClient.Close()
	}
	return err
}

func copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, totalSize int64, cb ProgressCallback) error {
	buf := make([]byte, 64*1024)
	var transferred int64
	startTime := time.Now()
	lastCbTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := src.Read(buf)
		if n > 0 {
			if _, wErr := dst.Write(buf[:n]); wErr != nil {
				return wErr
			}
			transferred += int64(n)

			if cb != nil {
				now := time.Now()
				if now.Sub(lastCbTime) >= 100*time.Millisecond || (totalSize > 0 && transferred >= totalSize) {
					lastCbTime = now
					elapsed := now.Sub(startTime).Seconds()
					var speed float64
					if elapsed > 0 {
						speed = float64(transferred) / elapsed
					}
					cb(transferred, totalSize, speed)
				}
			}
		}

		if err == io.EOF {
			if cb != nil && totalSize > 0 && transferred > 0 {
				elapsed := time.Since(startTime).Seconds()
				var speed float64
				if elapsed > 0 {
					speed = float64(transferred) / elapsed
				}
				cb(transferred, totalSize, speed)
			}
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}
