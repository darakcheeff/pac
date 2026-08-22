package storage

import (
	"time"
)

// Group represents a hierarchical folder for organizing hosts
type Group struct {
	ID        string    `json:"id"`
	ParentID  string    `json:"parent_id"`
	Name      string    `json:"name"`
	Icon      string    `json:"icon"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Protocol defines connection protocol type
type Protocol string

const (
	ProtoSSH    Protocol = "ssh"
	ProtoSFTP   Protocol = "sftp"
	ProtoLocal  Protocol = "local"
	ProtoTelnet Protocol = "telnet"
	ProtoSerial Protocol = "serial"
	ProtoVNC    Protocol = "vnc"
	ProtoRDP    Protocol = "rdp"
	ProtoMosh   Protocol = "mosh"
)

// AuthMethod defines the authentication type
type AuthMethod string

const (
	AuthPassword  AuthMethod = "password"
	AuthKey       AuthMethod = "key"
	AuthAgent     AuthMethod = "agent"
	AuthKeyboard  AuthMethod = "keyboard-interactive"
)

// PortForward represents -L, -R, or -D port forwarding
type PortForward struct {
	Type       string `json:"type"` // "local", "remote", "dynamic"
	LocalPort  int    `json:"local_port"`
	RemoteHost string `json:"remote_host"`
	RemotePort int    `json:"remote_port"`
}

// Host represents a connection profile
type Host struct {
	ID          string      `json:"id"`
	GroupID     string      `json:"group_id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Protocol    Protocol    `json:"protocol"`
	Host        string      `json:"host"`
	Port        int         `json:"port"`
	Username    string      `json:"username"`
	AuthMethod  AuthMethod  `json:"auth_method"`
	Password    string      `json:"password,omitempty"`     // Keyring reference or encrypted
	KeyPath     string      `json:"key_path,omitempty"`
	KeyPass     string      `json:"key_pass,omitempty"`
	
	// SSH advanced options
	X11Forwarding bool          `json:"x11_forwarding"`
	ProxyJumpHost string        `json:"proxy_jump_host"` // HostID or user@bastion:port
	PortForwards  []PortForward `json:"port_forwards"`
	AutoSFTP      bool          `json:"auto_sftp"`

	// Serial options
	SerialPort     string `json:"serial_port"`
	SerialBaudRate int    `json:"serial_baud_rate"`
	SerialDataBits int    `json:"serial_data_bits"`
	SerialStopBits int    `json:"serial_stop_bits"`
	SerialParity   string `json:"serial_parity"`

	// Terminal / Appearance options
	TerminalType    string `json:"terminal_type"` // e.g. "xterm-256color"
	FontName        string `json:"font_name"`
	ColorScheme     string `json:"color_scheme"`
	ScrollbackLines int    `json:"scrollback_lines"`

	// Logging & History
	EnableLogging bool   `json:"enable_logging"`
	LogPathFormat string `json:"log_path_format"`
	LogCleanANSI  bool   `json:"log_clean_ansi"`
	RestoreHistory bool  `json:"restore_history"`

	// Notes
	Notes string `json:"notes"`

	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SavedSessionState represents tab/window state saved for restore on restart
type SavedSessionState struct {
	ID             string    `json:"id"`
	HostID         string    `json:"host_id"`
	Title          string    `json:"title"`
	Protocol       Protocol  `json:"protocol"`
	TabIndex       int       `json:"tab_index"`
	SplitParentID  string    `json:"split_parent_id"`
	SplitDirection string    `json:"split_direction"` // "none", "horizontal", "vertical"
	WorkingDir     string    `json:"working_dir"`
	ScrollbackDump string    `json:"scrollback_dump"` // Last N lines of history
	SavedAt        time.Time `json:"saved_at"`
}

// AppSettings represents global application configuration
type AppSettings struct {
	Theme               string `json:"theme"`                 // "system", "dark", "light"
	DefaultFont         string `json:"default_font"`          // e.g. "Monospace 11"
	DefaultColorScheme  string `json:"default_color_scheme"`   // "mate", "solarized-dark", "dracula", etc.
	DefaultScrollback   int    `json:"default_scrollback"`    // default 10000
	DefaultLogsDir      string `json:"default_logs_dir"`      // ~/logs/sessions
	DefaultEditor       string `json:"default_editor"`        // "xed", "gedit", "code", or system default
	AutoRestoreSessions bool   `json:"auto_restore_sessions"` // true
	MasterPasswordHash  string `json:"master_password_hash"`  // for local AES fallback
	MasterPasswordSalt  string `json:"master_password_salt"`
}
