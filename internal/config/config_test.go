package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadRequiresExactlySupportedValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://orbit:test@db/orbit")
	t.Setenv("BOOTSTRAP_ADMIN", "admin")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "a-long-password")
	t.Setenv("ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.EncryptionKey) != 32 || cfg.BootstrapAdmin != "admin" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestLoadRejectsWeakMasterKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://db/orbit")
	t.Setenv("BOOTSTRAP_ADMIN", "admin")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "password")
	t.Setenv("ENCRYPTION_KEY", "short")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid encryption key error")
	}
}
