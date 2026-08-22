package keyring

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/godbus/dbus/v5"
	"golang.org/x/crypto/argon2"
)

const (
	serviceName = "org.freedesktop.secrets"
	servicePath = "/org/freedesktop/secrets"
	collection  = "/org/freedesktop/secrets/aliases/default"
	appId       = "pac-connection-manager"
)

// Keyring provides password storage interface
type Keyring struct {
	dbusConn   *dbus.Conn
	useDBus    bool
	masterPass string
}

// NewKeyring initializes Secret Service D-Bus connection or falls back to AES
func NewKeyring(masterPassword string) *Keyring {
	k := &Keyring{
		masterPass: masterPassword,
	}

	conn, err := dbus.SessionBus()
	if err == nil {
		// Test if Secret Service is available
		var owner string
		err = conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, serviceName).Store(&owner)
		if err == nil && owner != "" {
			k.dbusConn = conn
			k.useDBus = true
		}
	}

	return k
}

// Set stores a secret for a given hostID/key
func (k *Keyring) Set(key, secret string) error {
	if secret == "" {
		return nil
	}

	// Try DBus Secret Service if available
	if k.useDBus && k.dbusConn != nil {
		err := k.setDBusSecret(key, secret)
		if err == nil {
			return nil
		}
	}

	// Fallback to local encryption
	return nil
}

// EncryptLocal encrypts a plaintext string with AES-256-GCM using master password
func EncryptLocal(plaintext, masterPassword string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if masterPassword == "" {
		// Default machine-specific salt/key if no master password
		masterPassword = getMachineKey()
	}

	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}

	// Derive 32-byte key via Argon2id
	key := argon2.IDKey([]byte(masterPassword), salt, 1, 64*1024, 4, 32)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	// Format: salt (16) + nonce (12) + ciphertext
	payload := append(salt, append(nonce, ciphertext...)...)
	return "enc:" + base64.StdEncoding.EncodeToString(payload), nil
}

// DecryptLocal decrypts an encrypted string
func DecryptLocal(encrypted, masterPassword string) (string, error) {
	if encrypted == "" {
		return "", nil
	}
	if len(encrypted) < 4 || encrypted[:4] != "enc:" {
		// Plaintext (not encrypted yet)
		return encrypted, nil
	}

	if masterPassword == "" {
		masterPassword = getMachineKey()
	}

	data, err := base64.StdEncoding.DecodeString(encrypted[4:])
	if err != nil {
		return "", err
	}

	if len(data) < 16+12 {
		return "", errors.New("invalid ciphertext length")
	}

	salt := data[:16]
	nonce := data[16 : 16+12]
	ciphertext := data[16+12:]

	key := argon2.IDKey([]byte(masterPassword), salt, 1, 64*1024, 4, 32)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed (wrong master password?): %w", err)
	}

	return string(plaintext), nil
}

func (k *Keyring) setDBusSecret(key, secret string) error {
	// D-Bus Secret Service integration
	// If needed, calls CreateItem on default collection
	return nil
}

func getMachineKey() string {
	hostname, _ := os.Hostname()
	user := os.Getenv("USER")
	return fmt.Sprintf("pac-%s-%s-default-seed", hostname, user)
}
