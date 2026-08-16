package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hkjang/orbit/internal/id"
	"github.com/hkjang/orbit/internal/secure"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	DB    *pgxpool.Pool
	Vault *secure.Vault
}

func Open(ctx context.Context, dsn string, vault *secure.Vault) (*Store, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres DSN: %w", err)
	}
	config.MaxConns = 20
	config.MinConns = 2
	config.MaxConnLifetime = time.Hour
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := db.Ping(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	s := &Store{DB: db, Vault: vault}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() { s.DB.Close() }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.DB.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		part := strings.SplitN(entry.Name(), "_", 2)[0]
		version, err := strconv.Atoi(part)
		if err != nil {
			return fmt.Errorf("invalid migration name %s", entry.Name())
		}
		var exists bool
		if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(body)); err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, version)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Bootstrap(ctx context.Context, username, password string) error {
	var count int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM users WHERE role='admin'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	userID := id.New()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO users(id,username,display_name,password_hash,role) VALUES($1,$2,$2,$3,'admin')`, userID, username, string(hash)); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO user_preferences(user_id) VALUES($1)`, userID); err != nil {
		return err
	}
	dataKey, err := s.Vault.NewDataKey()
	if err != nil {
		return err
	}
	wrapped, err := s.Vault.WrapKey(dataKey)
	if err != nil {
		return err
	}
	keyID := id.New()
	if _, err = tx.Exec(ctx, `INSERT INTO user_key_versions(id,user_id,version,wrapped_key,rotated_by) VALUES($1,$2,1,$3,$2)`, keyID, userID, wrapped); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO key_permissions(id,key_version_id,principal_type,principal_id,permissions) VALUES($1,$2,'owner',$3,'["decrypt","encrypt","rotate","delegate"]')`, id.New(), keyID, userID); err != nil {
		return err
	}
	defaults := []struct{ ns, key, value string }{
		{"system", "general", `{"service_name":"Orbit","public_url":"http://localhost:8080","session_hours":12}`},
		{"auth", "oidc", `{"enabled":false,"issuer_url":"","client_id":"","display_name":"Keycloak SSO","auto_provision":true,"default_role":"member"}`},
		{"ai", "provider", `{"enabled":false,"provider":"openai-compatible","base_url":"","model":"","max_output_tokens":8192,"request_timeout_seconds":120,"system_prompt":"답변은 제공된 관계 기록에 근거하고, 모르는 내용은 추측하지 마세요."}`},
		{"workflow", "approval", `{"enabled":false,"resource_types":["memory"],"reviewer_role":"team_lead"}`},
		{"security", "key_policy", `{"rotation_days":90,"allow_user_rotation":true,"default_scopes":["people:read","memories:read"]}`},
	}
	for _, d := range defaults {
		if _, err = tx.Exec(ctx, `INSERT INTO settings(namespace,key,value,updated_by) VALUES($1,$2,$3::jsonb,$4) ON CONFLICT DO NOTHING`, d.ns, d.key, d.value, userID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

var ErrNotFound = errors.New("not found")
