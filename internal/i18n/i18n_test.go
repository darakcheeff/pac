package i18n

import (
	"os"
	"testing"
)

func TestDetectLocale(t *testing.T) {
	tests := []struct {
		envKey string
		envVal string
		wantRu bool
	}{
		{"LC_ALL", "ru_RU.UTF-8", true},
		{"LC_ALL", "ru", true},
		{"LC_ALL", "en_US.UTF-8", false},
		{"LC_MESSAGES", "ru_RU.UTF-8", true},
		{"LANGUAGE", "ru:en", true},
		{"LANGUAGE", "en:ru", false},
		{"LANG", "ru_RU.UTF-8", true},
		{"LANG", "C", false},
		{"LANG", "POSIX", false},
	}

	for _, tt := range tests {
		os.Unsetenv("LC_ALL")
		os.Unsetenv("LC_MESSAGES")
		os.Unsetenv("LANGUAGE")
		os.Unsetenv("LANG")

		os.Setenv(tt.envKey, tt.envVal)
		got := checkRussianEnv()
		if got != tt.wantRu {
			t.Errorf("checkRussianEnv() with %s=%s got %v, want %v", tt.envKey, tt.envVal, got, tt.wantRu)
		}
	}
}

func TestT(t *testing.T) {
	SetRussian(true)
	if got := T("Привет", "Hello"); got != "Привет" {
		t.Errorf("T() with Russian=true got %q, want 'Привет'", got)
	}
	if got := Tf("Привет, %s", "Hello, %s", "World"); got != "Привет, World" {
		t.Errorf("Tf() got %q", got)
	}

	SetRussian(false)
	if got := T("Привет", "Hello"); got != "Hello" {
		t.Errorf("T() with Russian=false got %q, want 'Hello'", got)
	}
	if got := Tf("Привет, %s", "Hello, %s", "World"); got != "Hello, World" {
		t.Errorf("Tf() got %q", got)
	}
}
