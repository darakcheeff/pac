package session

import (
	"fmt"
	"time"

	"github.com/darakcheeff/pac/internal/storage"
)

// SaveState dumps current active session states into SQLite database
func SaveState(store *storage.Store, sessions []*Session) error {
	var states []storage.SavedSessionState

	for idx, s := range sessions {
		if s == nil || s.Host == nil {
			continue
		}

		scrollback := s.GetScrollbackText()
		// Limit to last 50 KB of scrollback
		if len(scrollback) > 50*1024 {
			scrollback = scrollback[len(scrollback)-50*1024:]
		}

		workingDir := "/"
		if s.SFTPClient != nil {
			workingDir = s.SFTPClient.CurrentDir()
		}

		states = append(states, storage.SavedSessionState{
			ID:             s.ID,
			HostID:         s.Host.ID,
			Title:          s.Title,
			Protocol:       s.Host.Protocol,
			TabIndex:       idx,
			WorkingDir:     workingDir,
			ScrollbackDump: scrollback,
			Notes:          s.Notes,
			SavedAt:        time.Now(),
		})
	}

	return store.SaveActiveSessions(states)
}

// FormatRestoredHistoryHeader formats separator string for restored history buffer
func FormatRestoredHistoryHeader(savedAt time.Time) string {
	return fmt.Sprintf("\r\n\x1b[1;33m--- [Восстановленная история сессии: %s] ---\x1b[0m\r\n\r\n",
		savedAt.Format("2006-01-02 15:04:05"))
}
