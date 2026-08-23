package session

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/darakcheeff/pac/internal/storage"
)

var (
	stateCacheMu sync.Mutex
	lastStateHash string
)

// SaveState dumps current active session states into SQLite database only when state has changed
func SaveState(store *storage.Store, sessions []*Session) error {
	var states []storage.SavedSessionState
	hasher := sha256.New()

	for idx, s := range sessions {
		if s == nil {
			continue
		}

		hostID := ""
		protocol := storage.ProtoLocal
		if s.Host != nil {
			hostID = s.Host.ID
			protocol = s.Host.Protocol
		}

		scrollback := s.GetScrollbackText()
		if len(scrollback) > 50*1024 {
			scrollback = scrollback[len(scrollback)-50*1024:]
		}

		workingDir := "/"
		if s.SFTPClient != nil {
			workingDir = s.SFTPClient.CurrentDir()
		}

		st := storage.SavedSessionState{
			ID:             s.ID,
			HostID:         hostID,
			Title:          s.Title,
			Protocol:       protocol,
			TabIndex:       idx,
			WorkingDir:     workingDir,
			ScrollbackDump: scrollback,
			Notes:          s.Notes,
			SavedAt:        time.Now(),
		}
		states = append(states, st)

		// Feed hash state
		hasher.Write([]byte(fmt.Sprintf("%s|%s|%s|%s|%d|%s|%s|%d\n",
			st.ID, st.HostID, st.Title, st.Protocol, st.TabIndex, st.WorkingDir, st.Notes, len(st.ScrollbackDump))))
	}

	currentHash := hex.EncodeToString(hasher.Sum(nil))

	stateCacheMu.Lock()
	if currentHash == lastStateHash {
		stateCacheMu.Unlock()
		// No changes detected since last save, skip disk writes entirely
		return nil
	}
	lastStateHash = currentHash
	stateCacheMu.Unlock()

	log.Printf("[STATE] State change detected. Saving %d active session(s) to SQLite...", len(states))
	err := store.SaveActiveSessions(states)
	if err != nil {
		log.Printf("[STATE] ERROR saving active sessions: %v", err)
	} else {
		log.Printf("[STATE] Successfully synced %d session state(s) to disk.", len(states))
	}
	return err
}

// FormatRestoredHistoryHeader formats separator string for restored history buffer
func FormatRestoredHistoryHeader(savedAt time.Time) string {
	return fmt.Sprintf("\r\n\x1b[1;33m--- [Восстановленная история сессии: %s] ---\x1b[0m\r\n\r\n",
		savedAt.Format("2006-01-02 15:04:05"))
}
