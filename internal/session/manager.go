package session

import (
	"context"
	"strings"
	"sync"

	"github.com/darakcheeff/pac/internal/storage"
)

// SearchMatch represents a found line in global session search
type SearchMatch struct {
	SessionID  string `json:"session_id"`
	HostName   string `json:"host_name"`
	LineNumber int    `json:"line_number"`
	LineText   string `json:"line_text"`
}

// Manager maintains registry of all open active sessions
type Manager struct {
	sessions map[string]*Session
	store    *storage.Store
	mu       sync.RWMutex
}

func NewManager(store *storage.Store) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		store:    store,
	}
}

// Register adds session to manager
func (m *Manager) Register(sess *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[sess.ID] = sess
}

// Unregister removes and closes session
func (m *Manager) Unregister(sessionID string) {
	m.mu.Lock()
	sess, exists := m.sessions[sessionID]
	if exists {
		delete(m.sessions, sessionID)
	}
	m.mu.Unlock()

	if exists && sess != nil {
		_ = sess.Close()
	}
}

// Get returns session by ID
func (m *Manager) Get(sessionID string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[sessionID]
}

// GetAll returns slice of all active sessions
func (m *Manager) GetAll() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Session
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result
}

// BroadcastInput sends input string to all active sessions or filtered by sessionIDs
func (m *Manager) BroadcastInput(input string, sessionIDs []string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	targetMap := make(map[string]bool)
	for _, id := range sessionIDs {
		targetMap[id] = true
	}

	count := 0
	for id, s := range m.sessions {
		if len(targetMap) == 0 || targetMap[id] {
			if err := s.SendInput(input); err == nil {
				count++
			}
		}
	}
	return count
}

// GlobalSearch performs full-text search across scrollback buffers of all active sessions
func (m *Manager) GlobalSearch(ctx context.Context, query string, caseSensitive bool) []SearchMatch {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if query == "" {
		return nil
	}

	var results []SearchMatch
	searchQ := query
	if !caseSensitive {
		searchQ = strings.ToLower(query)
	}

	for _, s := range m.sessions {
		select {
		case <-ctx.Done():
			return results
		default:
		}

		text := s.GetScrollbackText()
		lines := strings.Split(text, "\n")
		for idx, line := range lines {
			cmpLine := line
			if !caseSensitive {
				cmpLine = strings.ToLower(line)
			}

			if strings.Contains(cmpLine, searchQ) {
				results = append(results, SearchMatch{
					SessionID:  s.ID,
					HostName:   s.Title,
					LineNumber: idx + 1,
					LineText:   strings.TrimSpace(line),
				})
			}
		}
	}

	return results
}

// CloseAll closes all open sessions
func (m *Manager) CloseAll() {
	m.mu.Lock()
	sessions := m.sessions
	m.sessions = make(map[string]*Session)
	m.mu.Unlock()

	for _, s := range sessions {
		_ = s.Close()
	}
}
