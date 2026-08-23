package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStore initializes SQLite database with WAL mode
func NewStore(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	db.SetMaxOpenConns(1) // Avoid database locked errors with SQLite WAL

	store := &Store{db: db}
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to init db schema: %w", err)
	}

	return store, nil
}

// DB returns the underlying *sql.DB connection
func (s *Store) DB() *sql.DB {
	return s.db
}

// Close closes the database connection cleanly
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		log.Printf("[DB] Closing SQLite database.")
		return s.db.Close()
	}
	return nil
}

func (s *Store) initSchema() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS groups (
			id TEXT PRIMARY KEY,
			parent_id TEXT,
			name TEXT NOT NULL,
			icon TEXT,
			sort_order INTEGER DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS hosts (
			id TEXT PRIMARY KEY,
			group_id TEXT,
			name TEXT NOT NULL,
			description TEXT,
			protocol TEXT NOT NULL,
			host TEXT,
			port INTEGER NOT NULL,
			username TEXT,
			auth_method TEXT,
			password TEXT,
			key_path TEXT,
			key_pass TEXT,
			x11_forwarding BOOLEAN DEFAULT 0,
			proxy_jump_host TEXT,
			port_forwards TEXT,
			auto_sftp BOOLEAN DEFAULT 1,
			serial_port TEXT,
			serial_baud_rate INTEGER DEFAULT 115200,
			serial_data_bits INTEGER DEFAULT 8,
			serial_stop_bits INTEGER DEFAULT 1,
			serial_parity TEXT DEFAULT "N",
			terminal_type TEXT DEFAULT "xterm-256color",
			font_name TEXT,
			color_scheme TEXT,
			scrollback_lines INTEGER DEFAULT 10000,
			enable_logging BOOLEAN DEFAULT 0,
			log_path_format TEXT,
			log_clean_ansi BOOLEAN DEFAULT 1,
			restore_history BOOLEAN DEFAULT 1,
			notes TEXT,
			sort_order INTEGER DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS notes (
			host_id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			updated_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS saved_sessions (
			id TEXT PRIMARY KEY,
			host_id TEXT,
			title TEXT NOT NULL,
			protocol TEXT NOT NULL,
			tab_index INTEGER DEFAULT 0,
			split_parent_id TEXT,
			split_direction TEXT DEFAULT "none",
			working_dir TEXT,
			scrollback_dump TEXT,
			notes TEXT,
			saved_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_hosts_group ON hosts(group_id);`,
		`CREATE INDEX IF NOT EXISTS idx_groups_parent ON groups(parent_id);`,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return fmt.Errorf("schema query error (%s): %w", query, err)
		}
	}

	// Automatic schema migration for existing SQLite database files
	s.migrateColumn("saved_sessions", "notes", "TEXT")
	s.migrateColumn("saved_sessions", "split_parent_id", "TEXT")
	s.migrateColumn("saved_sessions", "split_direction", "TEXT")
	s.migrateColumn("saved_sessions", "scrollback_dump", "TEXT")
	s.migrateColumn("saved_sessions", "working_dir", "TEXT")
	s.migrateColumn("hosts", "notes", "TEXT")
	s.migrateColumn("hosts", "proxy_jump_host", "TEXT")
	s.migrateColumn("hosts", "port_forwards", "TEXT")

	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM groups WHERE id = 'root'").Scan(&count)
	if count == 0 {
		now := time.Now()
		s.db.Exec("INSERT INTO groups (id, parent_id, name, icon, sort_order, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			"root", "", "Все подключения", "folder", 0, now, now)
	}

	return nil
}

func (s *Store) migrateColumn(table, column, colType string) {
	query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colType)
	_, _ = s.db.Exec(query)
}

// --- Groups CRUD ---

func (s *Store) GetAllGroups() ([]Group, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query("SELECT id, COALESCE(parent_id, ''), name, COALESCE(icon, ''), sort_order, created_at, updated_at FROM groups ORDER BY sort_order, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.ParentID, &g.Name, &g.Icon, &g.SortOrder, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func (s *Store) SaveGroup(g *Group) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if g.CreatedAt.IsZero() {
		g.CreatedAt = now
	}
	g.UpdatedAt = now

	query := `INSERT INTO groups (id, parent_id, name, icon, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			parent_id = excluded.parent_id,
			name = excluded.name,
			icon = excluded.icon,
			sort_order = excluded.sort_order,
			updated_at = excluded.updated_at`

	_, err := s.db.Exec(query, g.ID, g.ParentID, g.Name, g.Icon, g.SortOrder, g.CreatedAt, g.UpdatedAt)
	return err
}

func (s *Store) DeleteGroup(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM groups WHERE id = ?", id)
	return err
}

// --- Hosts CRUD ---

const hostSelectCols = `id, COALESCE(group_id, 'root'), name, COALESCE(description, ''), protocol, COALESCE(host, ''), COALESCE(port, 22), COALESCE(username, ''), COALESCE(auth_method, 'password'),
	COALESCE(password, ''), COALESCE(key_path, ''), COALESCE(key_pass, ''), COALESCE(x11_forwarding, 0), COALESCE(proxy_jump_host, ''), COALESCE(port_forwards, '[]'), COALESCE(auto_sftp, 1),
	COALESCE(serial_port, ''), COALESCE(serial_baud_rate, 115200), COALESCE(serial_data_bits, 8), COALESCE(serial_stop_bits, 1), COALESCE(serial_parity, 'N'),
	COALESCE(terminal_type, 'xterm-256color'), COALESCE(font_name, 'Monospace 11'), COALESCE(color_scheme, 'mate'), COALESCE(scrollback_lines, 10000), COALESCE(enable_logging, 0), COALESCE(log_path_format, ''),
	COALESCE(log_clean_ansi, 1), COALESCE(restore_history, 1), COALESCE(notes, ''), COALESCE(sort_order, 0), created_at, updated_at`

func (s *Store) GetAllHosts() ([]Host, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query("SELECT " + hostSelectCols + " FROM hosts ORDER BY sort_order, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hosts []Host
	for rows.Next() {
		var h Host
		var portForwardsJSON string
		err := rows.Scan(
			&h.ID, &h.GroupID, &h.Name, &h.Description, &h.Protocol, &h.Host, &h.Port, &h.Username, &h.AuthMethod,
			&h.Password, &h.KeyPath, &h.KeyPass, &h.X11Forwarding, &h.ProxyJumpHost, &portForwardsJSON, &h.AutoSFTP,
			&h.SerialPort, &h.SerialBaudRate, &h.SerialDataBits, &h.SerialStopBits, &h.SerialParity,
			&h.TerminalType, &h.FontName, &h.ColorScheme, &h.ScrollbackLines, &h.EnableLogging, &h.LogPathFormat,
			&h.LogCleanANSI, &h.RestoreHistory, &h.Notes, &h.SortOrder, &h.CreatedAt, &h.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if portForwardsJSON != "" {
			_ = json.Unmarshal([]byte(portForwardsJSON), &h.PortForwards)
		}
		hosts = append(hosts, h)
	}
	return hosts, nil
}

func (s *Store) GetHost(id string) (*Host, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var h Host
	var portForwardsJSON string
	row := s.db.QueryRow("SELECT " + hostSelectCols + " FROM hosts WHERE id = ?", id)

	err := row.Scan(
		&h.ID, &h.GroupID, &h.Name, &h.Description, &h.Protocol, &h.Host, &h.Port, &h.Username, &h.AuthMethod,
		&h.Password, &h.KeyPath, &h.KeyPass, &h.X11Forwarding, &h.ProxyJumpHost, &portForwardsJSON, &h.AutoSFTP,
		&h.SerialPort, &h.SerialBaudRate, &h.SerialDataBits, &h.SerialStopBits, &h.SerialParity,
		&h.TerminalType, &h.FontName, &h.ColorScheme, &h.ScrollbackLines, &h.EnableLogging, &h.LogPathFormat,
		&h.LogCleanANSI, &h.RestoreHistory, &h.Notes, &h.SortOrder, &h.CreatedAt, &h.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if portForwardsJSON != "" {
		_ = json.Unmarshal([]byte(portForwardsJSON), &h.PortForwards)
	}
	return &h, nil
}

func (s *Store) SaveHost(h *Host) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if h.CreatedAt.IsZero() {
		h.CreatedAt = now
	}
	h.UpdatedAt = now

	portForwardsJSON, _ := json.Marshal(h.PortForwards)

	query := `INSERT INTO hosts (
		id, group_id, name, description, protocol, host, port, username, auth_method,
		password, key_path, key_pass, x11_forwarding, proxy_jump_host, port_forwards, auto_sftp,
		serial_port, serial_baud_rate, serial_data_bits, serial_stop_bits, serial_parity,
		terminal_type, font_name, color_scheme, scrollback_lines, enable_logging, log_path_format,
		log_clean_ansi, restore_history, notes, sort_order, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		group_id = excluded.group_id,
		name = excluded.name,
		description = excluded.description,
		protocol = excluded.protocol,
		host = excluded.host,
		port = excluded.port,
		username = excluded.username,
		auth_method = excluded.auth_method,
		password = excluded.password,
		key_path = excluded.key_path,
		key_pass = excluded.key_pass,
		x11_forwarding = excluded.x11_forwarding,
		proxy_jump_host = excluded.proxy_jump_host,
		port_forwards = excluded.port_forwards,
		auto_sftp = excluded.auto_sftp,
		serial_port = excluded.serial_port,
		serial_baud_rate = excluded.serial_baud_rate,
		serial_data_bits = excluded.serial_data_bits,
		serial_stop_bits = excluded.serial_stop_bits,
		serial_parity = excluded.serial_parity,
		terminal_type = excluded.terminal_type,
		font_name = excluded.font_name,
		color_scheme = excluded.color_scheme,
		scrollback_lines = excluded.scrollback_lines,
		enable_logging = excluded.enable_logging,
		log_path_format = excluded.log_path_format,
		log_clean_ansi = excluded.log_clean_ansi,
		restore_history = excluded.restore_history,
		notes = excluded.notes,
		sort_order = excluded.sort_order,
		updated_at = excluded.updated_at`

	_, err := s.db.Exec(query,
		h.ID, h.GroupID, h.Name, h.Description, h.Protocol, h.Host, h.Port, h.Username, h.AuthMethod,
		h.Password, h.KeyPath, h.KeyPass, h.X11Forwarding, h.ProxyJumpHost, string(portForwardsJSON), h.AutoSFTP,
		h.SerialPort, h.SerialBaudRate, h.SerialDataBits, h.SerialStopBits, h.SerialParity,
		h.TerminalType, h.FontName, h.ColorScheme, h.ScrollbackLines, h.EnableLogging, h.LogPathFormat,
		h.LogCleanANSI, h.RestoreHistory, h.Notes, h.SortOrder, h.CreatedAt, h.UpdatedAt,
	)
	return err
}

func (s *Store) DeleteHost(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM hosts WHERE id = ?", id)
	return err
}

// --- Notes ---

func (s *Store) GetNotes(hostID string) (string, error) {
	return s.GetNote(hostID)
}

func (s *Store) GetNote(hostID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var content string
	err := s.db.QueryRow("SELECT content FROM notes WHERE host_id = ?", hostID).Scan(&content)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return content, err
}

func (s *Store) SaveNotes(hostID, content string) error {
	return s.SaveNote(hostID, content)
}

func (s *Store) SaveNote(hostID, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT INTO notes (host_id, content, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(host_id) DO UPDATE SET
			content = excluded.content,
			updated_at = excluded.updated_at`

	_, err := s.db.Exec(query, hostID, content, time.Now())
	return err
}

// --- Saved Sessions (State Restore) ---

func (s *Store) GetSavedSessions() ([]SavedSessionState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT id, host_id, title, protocol, tab_index,
		COALESCE(split_parent_id, ''),
		COALESCE(split_direction, 'none'),
		COALESCE(working_dir, ''),
		COALESCE(scrollback_dump, ''),
		COALESCE(notes, ''),
		saved_at
		FROM saved_sessions ORDER BY tab_index`)
	if err != nil {
		log.Printf("[DB] ERROR querying saved_sessions: %v", err)
		return nil, err
	}
	defer rows.Close()

	var sessions []SavedSessionState
	for rows.Next() {
		var state SavedSessionState
		if err := rows.Scan(
			&state.ID, &state.HostID, &state.Title, &state.Protocol, &state.TabIndex,
			&state.SplitParentID, &state.SplitDirection, &state.WorkingDir,
			&state.ScrollbackDump, &state.Notes, &state.SavedAt,
		); err != nil {
			log.Printf("[DB] ERROR scanning saved_sessions row: %v", err)
			return nil, err
		}
		sessions = append(sessions, state)
	}
	log.Printf("[DB] GetSavedSessions loaded %d session state records from SQLite.", len(sessions))
	return sessions, nil
}

func (s *Store) SaveActiveSessions(states []SavedSessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM saved_sessions"); err != nil {
		return err
	}

	stmt, err := tx.Prepare(`INSERT INTO saved_sessions (
		id, host_id, title, protocol, tab_index, split_parent_id, split_direction,
		working_dir, scrollback_dump, notes, saved_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, st := range states {
		if _, err := stmt.Exec(
			st.ID, st.HostID, st.Title, st.Protocol, st.TabIndex, st.SplitParentID,
			st.SplitDirection, st.WorkingDir, st.ScrollbackDump, st.Notes, time.Now(),
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// --- App Settings ---

func (s *Store) GetSettings() (*AppSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	defaults := &AppSettings{
		Theme:               "system",
		DefaultFont:         "Monospace 11",
		DefaultColorScheme:  "mate",
		DefaultScrollback:   10000,
		DefaultLogsDir:      filepath.Join(os.Getenv("HOME"), "logs", "sessions"),
		DefaultEditor:       "xed",
		AutoRestoreSessions: true,
	}

	rows, err := s.db.Query("SELECT key, value FROM app_settings")
	if err != nil {
		return defaults, nil
	}
	defer rows.Close()

	for rows.Next() {
		var key, val string
		if err := rows.Scan(&key, &val); err == nil {
			switch key {
			case "theme":
				defaults.Theme = val
			case "default_font":
				defaults.DefaultFont = val
			case "default_color_scheme":
				defaults.DefaultColorScheme = val
			case "default_editor":
				defaults.DefaultEditor = val
			case "default_logs_dir":
				defaults.DefaultLogsDir = val
			}
		}
	}

	return defaults, nil
}

func (s *Store) SaveSetting(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT INTO app_settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`

	_, err := s.db.Exec(query, key, value)
	return err
}
