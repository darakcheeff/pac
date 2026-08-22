package sftp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// Client wraps pkg/sftp.Client with helper routines
type Client struct {
	sshClient  *ssh.Client
	sftpClient *sftp.Client
	currentDir string
	mu         sync.RWMutex
	closed     bool
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

// ListDir returns list of files/folders in remote directory
func (c *Client) ListDir(remotePath string) ([]FileItem, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed || c.sftpClient == nil {
		return nil, fmt.Errorf("sftp client is closed")
	}

	if remotePath == "" {
		remotePath = c.currentDir
	}

	entries, err := c.sftpClient.ReadDir(remotePath)
	if err != nil {
		return nil, err
	}

	var items []FileItem
	for _, entry := range entries {
		items = append(items, FileItem{
			Name:    entry.Name(),
			Path:    filepath.Join(remotePath, entry.Name()),
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
	defer c.mu.RUnlock()

	if c.closed || c.sftpClient == nil {
		return fmt.Errorf("sftp client is closed")
	}

	src, err := c.sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open remote file: %w", err)
	}
	defer src.Close()

	stat, err := src.Stat()
	if err != nil {
		return err
	}
	totalSize := stat.Size()

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
	defer c.mu.RUnlock()

	if c.closed || c.sftpClient == nil {
		return fmt.Errorf("sftp client is closed")
	}

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

	dst, err := c.sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file: %w", err)
	}
	defer dst.Close()

	return copyWithProgress(ctx, dst, src, totalSize, cb)
}

// Mkdir creates remote directory
func (c *Client) Mkdir(remotePath string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sftpClient.MkdirAll(remotePath)
}

// Remove deletes remote file or empty directory
func (c *Client) Remove(remotePath string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sftpClient.Remove(remotePath)
}

// Rename renames or moves remote file
func (c *Client) Rename(oldPath, newPath string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sftpClient.Rename(oldPath, newPath)
}

// Chmod updates file permissions
func (c *Client) Chmod(remotePath string, mode os.FileMode) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sftpClient.Chmod(remotePath, mode)
}

// Close closes the SFTP client
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	if c.sftpClient != nil {
		return c.sftpClient.Close()
	}
	return nil
}

func copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, totalSize int64, cb ProgressCallback) error {
	buf := make([]byte, 64*1024)
	var transferred int64
	startTime := time.Now()

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
				elapsed := time.Since(startTime).Seconds()
				var speed float64
				if elapsed > 0 {
					speed = float64(transferred) / elapsed
				}
				cb(transferred, totalSize, speed)
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}
