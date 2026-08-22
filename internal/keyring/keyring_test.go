package keyring

import (
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	password := "MySecretPassword123!@#"
	masterPass := "MasterKey999"

	encrypted, err := EncryptLocal(password, masterPass)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}
	if encrypted == password {
		t.Fatalf("expected encrypted string, got raw")
	}

	decrypted, err := DecryptLocal(encrypted, masterPass)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}
	if decrypted != password {
		t.Fatalf("expected %s, got %s", password, decrypted)
	}

	// Test wrong master password
	_, err = DecryptLocal(encrypted, "WrongMasterKey")
	if err == nil {
		t.Fatalf("expected error on wrong master key, got nil")
	}
}
