package utils

import (
	"os"
	"strconv"
	"strings"
)

// EnvString returns the env value or fallback if empty.
func EnvString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// EnvInt returns the env value as int or fallback on parse error/empty.
func EnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// EnvBool returns the env value as bool. Treats "true", "1", "yes", "on" as true;
// "false", "0", "no", "off" as false. Empty/unknown returns fallback.
func EnvBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	switch v {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return fallback
	}
}
