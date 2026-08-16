package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Config deliberately exposes only the four runtime environment variables
// supported by Orbit. All mutable service configuration is stored in Postgres.
type Config struct {
	DatabaseURL            string
	BootstrapAdmin         string
	BootstrapAdminPassword string
	EncryptionKey          []byte
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL:            strings.TrimSpace(os.Getenv("DATABASE_URL")),
		BootstrapAdmin:         strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN")),
		BootstrapAdminPassword: os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
	}
	if c.DatabaseURL == "" || c.BootstrapAdmin == "" || c.BootstrapAdminPassword == "" {
		return Config{}, errors.New("DATABASE_URL, BOOTSTRAP_ADMIN and BOOTSTRAP_ADMIN_PASSWORD are required")
	}
	if len(c.BootstrapAdminPassword) < 10 {
		return Config{}, errors.New("BOOTSTRAP_ADMIN_PASSWORD must be at least 10 characters")
	}
	key, err := parseKey(os.Getenv("ENCRYPTION_KEY"))
	if err != nil {
		return Config{}, err
	}
	c.EncryptionKey = key
	return c, nil
}

func parseKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("ENCRYPTION_KEY is required")
	}
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if len([]byte(value)) == 32 {
		return []byte(value), nil
	}
	return nil, fmt.Errorf("ENCRYPTION_KEY must be exactly 32 bytes or base64-encoded 32 bytes")
}
