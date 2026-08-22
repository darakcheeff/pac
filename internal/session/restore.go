package session

import (
	"fmt"
	"log"
	"time"

	"github.com/darakcheeff/pac/internal/storage"
)

// SaveState dumps current active session states into SQLite database
func SaveState(store *storage.Store, sessions []*Session) error {
	var states []storage.SavedSessionState

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
		log.Printf("[STATE] Prepared session to save: ID=%s, Title=%q, HostID=%s, Protocol=%s, ScrollbackBytes=%d",
			st.ID, st.Title, st.HostID, st.Protocol, len(st.ScrollbackDump))
	}

	log.Printf("[STATE] Saving %d active session states to database...", len(states))
	err := store.SaveActiveSessions(states)
	if err != nil {
		log.Printf("[STATE] ERROR saving active sessions: %v", err)
	} else {
		log.Printf("[STATE] Successfully saved %d session states to database.", len(states))
	}
	return err
}

// FormatRestoredHistoryHeader formats separator string for restored history buffer
func FormatRestoredHistoryHeader(savedAt time.Time) string {
	return fmt.Sprintf("\r\n\x1b[1;33m--- [Восстановленная история сессии: %s] ---\x1b[0m\r\n\r\n",
		savedAt.Format("2006-01-02 15:04:05"))
}
