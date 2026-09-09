package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

var (
	mu        sync.RWMutex
	isRussian bool
)

func init() {
	detectLocale()
}

// detectLocale inspects standard POSIX / GNU environment variables
// to determine if the user interface should be in Russian.
func detectLocale() {
	mu.Lock()
	defer mu.Unlock()
	isRussian = checkRussianEnv()
}

func checkRussianEnv() bool {
	// Standard priority order for message catalog translation:
	// LC_ALL -> LC_MESSAGES -> LANGUAGE -> LANG
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANGUAGE", "LANG"} {
		val := strings.TrimSpace(os.Getenv(env))
		if val == "" {
			continue
		}
		val = strings.ToLower(val)
		// Tokens can be separated by ':', '.', '_', '@' (e.g. ru_RU.UTF-8, ru:en, ru_RU:ru)
		parts := strings.FieldsFunc(val, func(r rune) bool {
			return r == ':' || r == '.' || r == '_' || r == '@'
		})
		if len(parts) > 0 {
			if parts[0] == "ru" {
				return true
			}
			// If explicitly specified as another language (e.g. en_US, C, POSIX, de_DE),
			// then it is not Russian.
			return false
		}
	}
	return false
}

// SetRussian allows explicitly overriding the detected locale.
func SetRussian(ru bool) {
	mu.Lock()
	defer mu.Unlock()
	isRussian = ru
}

// IsRussian returns true if Russian localization is active.
func IsRussian() bool {
	mu.RLock()
	defer mu.RUnlock()
	return isRussian
}

// T returns ru if Russian locale is active, otherwise en.
func T(ru, en string) string {
	if IsRussian() {
		return ru
	}
	return en
}

// Tf formats the translated string with fmt.Sprintf.
func Tf(ru, en string, args ...interface{}) string {
	return fmt.Sprintf(T(ru, en), args...)
}
