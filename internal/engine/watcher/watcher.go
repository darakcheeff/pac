package watcher

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// UploadHandler is called when a watched file is modified locally
type UploadHandler func(ctx context.Context, localPath, remotePath string) error

// TrackedFile stores metadata for open remote file
type TrackedFile struct {
	LocalPath  string
	RemotePath string
	HostID     string
	LastSaved  time.Time
	OnUpload   UploadHandler
}

// RemoteEditManager manages files opened for external editing
type RemoteEditManager struct {
	fsWatcher *fsnotify.Watcher
	files     map[string]*TrackedFile
	debounce  map[string]*time.Timer
	tempDir   string
	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewRemoteEditManager creates and starts the watcher service
func NewRemoteEditManager() (*RemoteEditManager, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("pac-sftp-%d", os.Getpid()))
	if err := os.MkdirAll(tempDir, 0700); err != nil {
		watcher.Close()
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	m := &RemoteEditManager{
		fsWatcher: watcher,
		files:     make(map[string]*TrackedFile),
		debounce:  make(map[string]*time.Timer),
		tempDir:   tempDir,
		ctx:       ctx,
		cancel:    cancel,
	}

	go m.eventLoop()
	return m, nil
}

// OpenForEditing tracks a file, launches external editor, and watches for saves
func (m *RemoteEditManager) OpenForEditing(hostID, remotePath string, downloadFn func(localPath string) error, uploadFn UploadHandler, preferredEditor string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	hostTempDir := filepath.Join(m.tempDir, hostID)
	_ = os.MkdirAll(hostTempDir, 0700)

	baseName := filepath.Base(remotePath)
	localPath := filepath.Join(hostTempDir, baseName)

	// Download the remote file
	if err := downloadFn(localPath); err != nil {
		return fmt.Errorf("failed to download remote file for editing: %w", err)
	}

	// Add file to fsnotify watcher
	if err := m.fsWatcher.Add(localPath); err != nil {
		return fmt.Errorf("failed to watch file %s: %w", localPath, err)
	}

	m.files[localPath] = &TrackedFile{
		LocalPath:  localPath,
		RemotePath: remotePath,
		HostID:     hostID,
		LastSaved:  time.Now(),
		OnUpload:   uploadFn,
	}

	// Launch editor in background
	go m.launchEditor(localPath, preferredEditor)

	return nil
}

func (m *RemoteEditManager) launchEditor(filePath, preferredEditor string) {
	var cmd *exec.Cmd
	if preferredEditor != "" {
		cmd = exec.Command(preferredEditor, filePath)
	} else if ed := os.Getenv("EDITOR"); ed != "" {
		cmd = exec.Command(ed, filePath)
	} else if _, err := exec.LookPath("xed"); err == nil {
		cmd = exec.Command("xed", filePath)
	} else if _, err := exec.LookPath("gedit"); err == nil {
		cmd = exec.Command("gedit", filePath)
	} else {
		cmd = exec.Command("xdg-open", filePath)
	}

	_ = cmd.Start()
}

func (m *RemoteEditManager) eventLoop() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case event, ok := <-m.fsWatcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				m.handleFileModified(event.Name)
			}
		case <-m.fsWatcher.Errors:
			// log watcher error if any
		}
	}
}

func (m *RemoteEditManager) handleFileModified(localPath string) {
	m.mu.Lock()
	tracked, exists := m.files[localPath]
	if !exists {
		m.mu.Unlock()
		return
	}

	// Debounce rapid writes (300ms)
	if timer, ok := m.debounce[localPath]; ok {
		timer.Stop()
	}

	m.debounce[localPath] = time.AfterFunc(300*time.Millisecond, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.debounce, localPath)

		if tracked.OnUpload != nil {
			go func(t *TrackedFile) {
				_ = t.OnUpload(context.Background(), t.LocalPath, t.RemotePath)
			}(tracked)
		}
	})
	m.mu.Unlock()
}

// Close stops the watcher and cleans up temp files
func (m *RemoteEditManager) Close() error {
	m.cancel()
	m.mu.Lock()
	defer m.mu.Unlock()

	_ = m.fsWatcher.Close()
	_ = os.RemoveAll(m.tempDir)
	return nil
}
