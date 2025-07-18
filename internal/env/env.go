package env

import (
	"os"
	"strings"
)

// FirstDefault returns the value of the first environment variable in keys
// that is set, otherwise it returns defaultValue.
func FirstDefault(defaultValue string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}

	return defaultValue
}
