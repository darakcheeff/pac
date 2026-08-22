package migration

import (
	"bufio"
	"bytes"
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

// LegacyNode represents a node in asbru/pac config
type LegacyNode struct {
	Name        string `yaml:"title"`
	Parent      string `yaml:"parent"`
	IsFolder    bool   `yaml:"is_folder"`
	Description string `yaml:"description"`
	Method      string `yaml:"method"`
	IP          string `yaml:"ip"`
	Port        int    `yaml:"port"`
	User        string `yaml:"user"`
	Pass        string `yaml:"pass"`
	Key         string `yaml:"auth_key"`
	Passphrase  string `yaml:"passphrase"`
	Terminal    struct {
		Font        string `yaml:"font"`
		ColorScheme string `yaml:"color_scheme"`
		Rows        int    `yaml:"rows"`
		Columns     int    `yaml:"columns"`
	} `yaml:"terminal"`
	Log struct {
		Enabled bool   `yaml:"enabled"`
		File    string `yaml:"file"`
	} `yaml:"log"`
	X11Forward bool `yaml:"x11_forward"`
	AutoSFTP   bool `yaml:"auto_sftp"`
	Notes      string `yaml:"notes"`
}

// MigrateOldConfig scans standard paths for asbru.conf / pac.yml and imports into store
func MigrateOldConfig(store *storage.Store, configPath string) (int, error) {
	if configPath == "" {
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
				configPath = c
				break
			}
		}
	}

	if configPath == "" {
		return 0, nil // No legacy config found
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return 0, err
	}

	// Determine format: Perl Data::Dumper or YAML
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("$VAR1")) {
		return importPerlDataDumper(store, string(data))
	}

	return importYAML(store, data)
}

func importYAML(store *storage.Store, data []byte) (int, error) {
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return 0, fmt.Errorf("failed to parse legacy YAML: %w", err)
	}

	count := 0
	environments, ok := root["environments"].(map[string]interface{})
	if !ok {
		// Try flat map
		environments = root
	}

	for id, val := range environments {
		nodeMap, ok := val.(map[string]interface{})
		if !ok {
			continue
		}

		title, _ := nodeMap["title"].(string)
		if title == "" {
			title = id
		}

		isFolder, _ := nodeMap["is_folder"].(bool)
		parent, _ := nodeMap["parent"].(string)
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
			_ = store.SaveGroup(group)
			count++
			continue
		}

		// It's a host
		method, _ := nodeMap["method"].(string)
		ip, _ := nodeMap["ip"].(string)
		portVal := getInt(nodeMap["port"], 22)
		user, _ := nodeMap["user"].(string)
		pass, _ := nodeMap["pass"].(string)
		key, _ := nodeMap["auth_key"].(string)
		desc, _ := nodeMap["description"].(string)
		notes, _ := nodeMap["notes"].(string)

		proto := storage.ProtoSSH
		switch strings.ToLower(method) {
		case "telnet":
			proto = storage.ProtoTelnet
		case "serial", "cu":
			proto = storage.ProtoSerial
		case "local":
			proto = storage.ProtoLocal
		case "vnc":
			proto = storage.ProtoVNC
		case "rdp":
			proto = storage.ProtoRDP
		}

		host := &storage.Host{
			ID:             id,
			GroupID:        parent,
			Name:           title,
			Description:    desc,
			Protocol:       proto,
			Host:           ip,
			Port:           portVal,
			Username:       user,
			AuthMethod:     storage.AuthPassword,
			Password:       pass,
			KeyPath:        key,
			AutoSFTP:       true,
			TerminalType:   "xterm-256color",
			ScrollbackLines: 10000,
			Notes:          notes,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		if key != "" {
			host.AuthMethod = storage.AuthKey
		}

		if err := store.SaveHost(host); err == nil {
			count++
		}
	}

	return count, nil
}

func importPerlDataDumper(store *storage.Store, content string) (int, error) {
	count := 0
	scanner := bufio.NewScanner(strings.NewReader(content))
	
	var currentID string
	var currentData = make(map[string]string)
	
	nodeRegex := regexp.MustCompile(`'([^']+)'\s*=>\s*\{`)
	kvRegex := regexp.MustCompile(`'([^']+)'\s*=>\s*'([^']*)'`)
	kvNumRegex := regexp.MustCompile(`'([^']+)'\s*=>\s*([0-9]+)`)

	flushNode := func() {
		if currentID == "" || len(currentData) == 0 {
			return
		}
		title := currentData["title"]
		if title == "" {
			title = currentID
		}
		parent := currentData["parent"]
		if parent == "__ROOT__" || parent == "0" || parent == "" {
			parent = "root"
		}

		if currentData["is_folder"] == "1" || currentData["is_group"] == "1" {
			_ = store.SaveGroup(&storage.Group{
				ID:        currentID,
				ParentID:  parent,
				Name:      title,
				Icon:      "folder",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			})
			count++
		} else {
			port, _ := strconv.Atoi(currentData["port"])
			if port == 0 {
				port = 22
			}
			host := &storage.Host{
				ID:          currentID,
				GroupID:     parent,
				Name:        title,
				Description: currentData["description"],
				Protocol:    storage.ProtoSSH,
				Host:        currentData["ip"],
				Port:        port,
				Username:    currentData["user"],
				AuthMethod:  storage.AuthPassword,
				Password:    currentData["pass"],
				KeyPath:     currentData["auth_key"],
				AutoSFTP:    true,
				Notes:       currentData["notes"],
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			if host.KeyPath != "" {
				host.AuthMethod = storage.AuthKey
			}
			if err := store.SaveHost(host); err == nil {
				count++
			}
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
		if matches := kvRegex.FindStringSubmatch(line); len(matches) > 2 {
			currentData[matches[1]] = matches[2]
		} else if matches := kvNumRegex.FindStringSubmatch(line); len(matches) > 2 {
			currentData[matches[1]] = matches[2]
		}
	}
	flushNode()

	return count, nil
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
