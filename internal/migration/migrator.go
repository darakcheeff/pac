package migration

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/darakcheeff/pac/internal/storage"
	"gopkg.in/yaml.v3"
)

// DecodePACPassword handles various obfuscations used by PAC/Ásbrú
func DecodePACPassword(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// 1. Prefix __PAC__B64__
	if strings.HasPrefix(raw, "__PAC__B64__") {
		b64 := strings.TrimPrefix(raw, "__PAC__B64__")
		if dec, err := base64.StdEncoding.DecodeString(b64); err == nil {
			return string(dec)
		}
	}

	// 2. Prefix __PAC__ENC__
	if strings.HasPrefix(raw, "__PAC__ENC__") {
		b64 := strings.TrimPrefix(raw, "__PAC__ENC__")
		if dec, err := base64.StdEncoding.DecodeString(b64); err == nil {
			return string(dec)
		}
	}

	// 3. Raw Base64 string check
	if len(raw)%4 == 0 && regexp.MustCompile(`^[A-Za-z0-9+/]+={0,2}$`).MatchString(raw) && len(raw) >= 8 {
		if dec, err := base64.StdEncoding.DecodeString(raw); err == nil && isPrintable(dec) {
			return string(dec)
		}
	}

	return raw
}

func isPrintable(data []byte) bool {
	for _, b := range data {
		if b < 32 && b != '\t' && b != '\n' && b != '\r' {
			return false
		}
	}
	return true
}

// FindLegacyConfigPath looks for standard legacy Ásbrú / PAC config files on disk
func FindLegacyConfigPath() string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".config", "asbru", "asbru.conf"),
		filepath.Join(home, ".config", "asbru", "asbru.yml"),
		filepath.Join(home, ".pac", "asbru.yml"),
		filepath.Join(home, ".pac", "pac.yml"),
		filepath.Join(home, ".pac", "pac.nfreeze"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// ParseLegacyData parses raw YAML or Perl DataDumper content into hosts and groups
func ParseLegacyData(data []byte) ([]*storage.Host, []*storage.Group, error) {
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("$VAR1")) {
		return parsePerlDataDumper(string(data))
	}
	return parseYAML(data)
}

// ParseLegacyConfigFile reads and parses a legacy config file into hosts and groups
func ParseLegacyConfigFile(configPath string) ([]*storage.Host, []*storage.Group, error) {
	if configPath == "" {
		configPath = FindLegacyConfigPath()
	}
	if configPath == "" {
		return nil, nil, fmt.Errorf("legacy configuration file not found")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, err
	}

	return ParseLegacyData(data)
}

// MigrateOldConfig scans standard paths for asbru.conf / pac.yml and imports into store
func MigrateOldConfig(store *storage.Store, configPath string) (int, error) {
	hosts, groups, err := ParseLegacyConfigFile(configPath)
	if err != nil {
		return 0, err
	}
	for _, g := range groups {
		_ = store.SaveGroup(g)
	}
	count := len(groups)
	for _, h := range hosts {
		if err := store.SaveHost(h); err == nil {
			count++
		}
	}
	return count, nil
}

func getString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func getMapString(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func resolveProtocol(method string) storage.Protocol {
	switch strings.ToLower(method) {
	case "telnet":
		return storage.ProtoTelnet
	case "serial", "cu":
		return storage.ProtoSerial
	case "local":
		return storage.ProtoLocal
	case "vnc":
		return storage.ProtoVNC
	case "rdp":
		return storage.ProtoRDP
	default:
		return storage.ProtoSSH
	}
}

func resolveAuth(authType, keyPath, pass, passphrase string) (storage.AuthMethod, string, string, string) {
	// Clean key path if .pub was given and private key exists without .pub
	if keyPath != "" && strings.HasSuffix(keyPath, ".pub") {
		privCandidate := strings.TrimSuffix(keyPath, ".pub")
		if _, err := os.Stat(privCandidate); err == nil {
			keyPath = privCandidate
		}
	}

	authMethod := storage.AuthPassword
	authTypeLower := strings.ToLower(authType)

	if authTypeLower == "publickey" || authTypeLower == "key" || authTypeLower == "pubkey" {
		authMethod = storage.AuthKey
	} else if authTypeLower == "agent" || authTypeLower == "ssh-agent" {
		authMethod = storage.AuthAgent
	} else if authTypeLower == "manual" || authTypeLower == "interactive" || authTypeLower == "keyboard-interactive" {
		authMethod = storage.AuthKeyboard
	} else if keyPath != "" && authTypeLower != "userpass" && authTypeLower != "password" {
		authMethod = storage.AuthKey
	}

	keyPass := passphrase
	if authMethod == storage.AuthKey && keyPass == "" && pass != "" {
		keyPass = pass
	}

	return authMethod, keyPath, pass, keyPass
}

func importYAML(store *storage.Store, data []byte) (int, error) {
	hosts, groups, err := parseYAML(data)
	if err != nil {
		return 0, err
	}
	for _, g := range groups {
		_ = store.SaveGroup(g)
	}
	count := len(groups)
	for _, h := range hosts {
		if err := store.SaveHost(h); err == nil {
			count++
		}
	}
	return count, nil
}

func parseYAML(data []byte) ([]*storage.Host, []*storage.Group, error) {
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("failed to parse legacy YAML: %w", err)
	}

	environments, ok := root["environments"].(map[string]interface{})
	if !ok {
		environments = root
	}

	var hosts []*storage.Host
	var groups []*storage.Group

	for id, val := range environments {
		nodeMap, ok := val.(map[string]interface{})
		if !ok {
			continue
		}

		title := getString(nodeMap, "title", "name")
		if title == "" {
			title = id
		}

		isFolder, _ := nodeMap["is_folder"].(bool)
		if !isFolder {
			isFolder, _ = nodeMap["is_group"].(bool)
		}
		parent := getString(nodeMap, "parent")
		if parent == "__ROOT__" || parent == "0" || parent == "" {
			parent = "root"
		}

		if isFolder {
			group := &storage.Group{
				ID:        id,
				ParentID:  parent,
				Name:      title,
				Icon:      "folder",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			groups = append(groups, group)
			continue
		}

		// Host
		method := getString(nodeMap, "method", "protocol")
		ip := getString(nodeMap, "ip", "host", "hostname")
		portVal := getInt(nodeMap["port"], 22)
		user := getString(nodeMap, "user", "username")

		passRaw := getString(nodeMap, "pass", "password", "pwd")
		pass := DecodePACPassword(passRaw)

		passphraseRaw := getString(nodeMap, "passphrase", "key_passphrase", "passphrase user")
		passphrase := DecodePACPassword(passphraseRaw)

		rawKeyPath := getString(nodeMap, "public key", "public_key", "auth_key", "key", "key_path", "identity_file", "identity")
		rawAuthType := getString(nodeMap, "auth type", "auth_type", "auth", "authentication")
		desc := getString(nodeMap, "description", "desc")
		notes := getString(nodeMap, "notes", "comments")

		proto := resolveProtocol(method)
		authMethod, keyPath, finalPass, finalKeyPass := resolveAuth(rawAuthType, rawKeyPath, pass, passphrase)

		host := &storage.Host{
			ID:              id,
			GroupID:         parent,
			Name:            title,
			Description:     desc,
			Protocol:        proto,
			Host:            ip,
			Port:            portVal,
			Username:        user,
			AuthMethod:      authMethod,
			Password:        finalPass,
			KeyPath:         keyPath,
			KeyPass:         finalKeyPass,
			AutoSFTP:        true,
			TerminalType:    "xterm-256color",
			ScrollbackLines: 10000,
			LogCleanANSI:    true,
			RestoreHistory:  true,
			Notes:           notes,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		hosts = append(hosts, host)
	}

	return hosts, groups, nil
}

func importPerlDataDumper(store *storage.Store, content string) (int, error) {
	hosts, groups, err := parsePerlDataDumper(content)
	if err != nil {
		return 0, err
	}
	for _, g := range groups {
		_ = store.SaveGroup(g)
	}
	count := len(groups)
	for _, h := range hosts {
		if err := store.SaveHost(h); err == nil {
			count++
		}
	}
	return count, nil
}

func parsePerlDataDumper(content string) ([]*storage.Host, []*storage.Group, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))

	var currentID string
	var currentData = make(map[string]string)

	nodeRegex := regexp.MustCompile(`\x27([^\x27]+)\x27\s*=>\s*\{`)
	kvSingleRegex := regexp.MustCompile(`\x27([^\x27]+)\x27\s*=>\s*\x27((?:\\\x27|[^\x27])*)\x27`)
	kvDoubleRegex := regexp.MustCompile(`\x27([^\x27]+)\x27\s*=>\s*"((?:\\"|[^"])*)"`)
	kvNumRegex := regexp.MustCompile(`\x27([^\x27]+)\x27\s*=>\s*([0-9]+)`)

	var hosts []*storage.Host
	var groups []*storage.Group

	flushNode := func() {
		if currentID == "" || len(currentData) == 0 {
			return
		}
		title := getMapString(currentData, "title", "name")
		if title == "" {
			title = currentID
		}
		parent := getMapString(currentData, "parent")
		if parent == "__ROOT__" || parent == "0" || parent == "" {
			parent = "root"
		}

		if currentData["is_folder"] == "1" || currentData["is_group"] == "1" {
			groups = append(groups, &storage.Group{
				ID:        currentID,
				ParentID:  parent,
				Name:      title,
				Icon:      "folder",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			})
		} else {
			port, _ := strconv.Atoi(currentData["port"])
		if port == 0 {
			port = 22
		}

		method := getMapString(currentData, "method", "protocol")
		proto := resolveProtocol(method)

		rawPass := getMapString(currentData, "pass", "password", "pwd")
		pass := DecodePACPassword(rawPass)

		rawPassphrase := getMapString(currentData, "passphrase", "key_passphrase", "passphrase user")
		passphrase := DecodePACPassword(rawPassphrase)

		rawKeyPath := getMapString(currentData, "public key", "public_key", "auth_key", "key", "key_path", "identity_file", "identity")
		rawAuthType := getMapString(currentData, "auth type", "auth_type", "auth", "authentication")

		authMethod, keyPath, finalPass, finalKeyPass := resolveAuth(rawAuthType, rawKeyPath, pass, passphrase)

		host := &storage.Host{
			ID:              currentID,
			GroupID:         parent,
			Name:            title,
			Description:     getMapString(currentData, "description", "desc"),
			Protocol:        proto,
			Host:            getMapString(currentData, "ip", "host", "hostname"),
			Port:            port,
			Username:        getMapString(currentData, "user", "username"),
			AuthMethod:      authMethod,
			Password:        finalPass,
			KeyPath:         keyPath,
			KeyPass:         finalKeyPass,
			AutoSFTP:        true,
			TerminalType:    "xterm-256color",
			ScrollbackLines: 10000,
			LogCleanANSI:    true,
			RestoreHistory:  true,
			Notes:           getMapString(currentData, "notes", "comments"),
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		hosts = append(hosts, host)
	}
	currentID = ""
	currentData = make(map[string]string)
}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if matches := nodeRegex.FindStringSubmatch(line); len(matches) > 1 {
			flushNode()
			currentID = matches[1]
			continue
		}
		if matches := kvSingleRegex.FindStringSubmatch(line); len(matches) > 2 {
			val := strings.ReplaceAll(matches[2], string([]byte{92, 39}), string([]byte{39}))
			currentData[matches[1]] = val
		} else if matches := kvDoubleRegex.FindStringSubmatch(line); len(matches) > 2 {
			val := strings.ReplaceAll(matches[2], string([]byte{92, 34}), string([]byte{34}))
			currentData[matches[1]] = val
		} else if matches := kvNumRegex.FindStringSubmatch(line); len(matches) > 2 {
			currentData[matches[1]] = matches[2]
		}
	}
	flushNode()

	return hosts, groups, nil
}

func getInt(val interface{}, def int) int {
	if val == nil {
		return def
	}
	switch v := val.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		i, err := strconv.Atoi(v)
		if err == nil {
			return i
		}
	}
	return def
}
