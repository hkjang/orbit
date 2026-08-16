package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hkjang/orbit/internal/id"
	"github.com/hkjang/orbit/internal/secure"
	"github.com/jackc/pgx/v5"
)

type Preferences struct {
	Theme               string    `json:"theme"`
	Locale              string    `json:"locale"`
	FontScale           float64   `json:"font_scale"`
	ReduceMotion        bool      `json:"reduce_motion"`
	RediscoverFrequency string    `json:"rediscover_frequency"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (s *Server) getPreferences(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	var p Preferences
	if err := s.store.DB.QueryRow(r.Context(), `SELECT theme,locale,font_scale,reduce_motion,rediscover_frequency,updated_at FROM user_preferences WHERE user_id=$1`, u.ID).Scan(&p.Theme, &p.Locale, &p.FontScale, &p.ReduceMotion, &p.RediscoverFrequency, &p.UpdatedAt); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"preferences": p})
}
func (s *Server) updatePreferences(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	var p Preferences
	if !decodeJSON(w, r, &p) {
		return
	}
	if p.Theme != "dark" && p.Theme != "light" && p.Theme != "system" {
		writeError(w, 400, "validation_error", "테마를 확인해 주세요.")
		return
	}
	if p.FontScale < .9 || p.FontScale > 1.4 {
		writeError(w, 400, "validation_error", "글자 크기는 90~140% 범위여야 합니다.")
		return
	}
	if p.RediscoverFrequency != "off" && p.RediscoverFrequency != "daily" && p.RediscoverFrequency != "weekly" {
		writeError(w, 400, "validation_error", "Rediscover 주기를 확인해 주세요.")
		return
	}
	_, err := s.store.DB.Exec(r.Context(), `UPDATE user_preferences SET theme=$2,locale=$3,font_scale=$4,reduce_motion=$5,rediscover_frequency=$6,updated_at=now() WHERE user_id=$1`, u.ID, p.Theme, p.Locale, p.FontScale, p.ReduceMotion, p.RediscoverFrequency)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) listKeys(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	rows, err := s.store.DB.Query(r.Context(), `SELECT id,version,status,created_at,retired_at FROM user_key_versions WHERE user_id=$1 ORDER BY version DESC`, u.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var keyID, status string
		var version int
		var created time.Time
		var retired *time.Time
		if err := rows.Scan(&keyID, &version, &status, &created, &retired); err != nil {
			internalError(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": keyID, "version": version, "status": status, "created_at": created, "retired_at": retired})
	}
	var policy KeyPolicySettings
	_ = s.readSetting(r.Context(), "security", "key_policy", &policy, nil)
	writeJSON(w, 200, map[string]any{"keys": items, "policy": policy})
}

type encryptedPersonRow struct{ ID, Email, Phone, Note string }
type encryptedContentRow struct{ ID, Value string }

func (s *Server) rotateKey(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	var policy KeyPolicySettings
	if err := s.readSetting(r.Context(), "security", "key_policy", &policy, nil); err != nil {
		internalError(w, r, err)
		return
	}
	if !policy.AllowUserRotation && u.Role != "admin" {
		writeError(w, 403, "rotation_disabled", "관리자가 개인 키 회전을 허용하지 않았습니다.")
		return
	}
	tx, err := s.store.DB.Begin(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var oldID, wrapped string
	var oldVersion int
	if err = tx.QueryRow(r.Context(), `SELECT k.id,k.wrapped_key,k.version FROM user_key_versions k JOIN key_permissions p ON p.key_version_id=k.id AND p.principal_type='owner' AND p.principal_id=k.user_id::text AND p.permissions ? 'rotate' WHERE k.user_id=$1 AND k.status='active' FOR UPDATE OF k`, u.ID).Scan(&oldID, &wrapped, &oldVersion); err != nil {
		internalError(w, r, err)
		return
	}
	oldKey, err := s.store.Vault.UnwrapKey(wrapped)
	if err != nil {
		internalError(w, r, err)
		return
	}
	newKey, err := s.store.Vault.NewDataKey()
	if err != nil {
		internalError(w, r, err)
		return
	}
	newWrapped, err := s.store.Vault.WrapKey(newKey)
	if err != nil {
		internalError(w, r, err)
		return
	}
	people, err := readEncryptedPeople(r.Context(), tx, u.ID, oldVersion)
	if err != nil {
		internalError(w, r, err)
		return
	}
	interactions, err := readEncryptedRows(r.Context(), tx, `SELECT id,summary_cipher FROM interactions WHERE user_id=$1 AND key_version=$2`, u.ID, oldVersion)
	if err != nil {
		internalError(w, r, err)
		return
	}
	memories, err := readEncryptedRows(r.Context(), tx, `SELECT id,content_cipher FROM memories WHERE user_id=$1 AND key_version=$2`, u.ID, oldVersion)
	if err != nil {
		internalError(w, r, err)
		return
	}
	newVersion := oldVersion + 1
	if _, err = tx.Exec(r.Context(), `UPDATE user_key_versions SET status='retired',retired_at=now() WHERE id=$1`, oldID); err != nil {
		internalError(w, r, err)
		return
	}
	newID := id.New()
	if _, err = tx.Exec(r.Context(), `INSERT INTO user_key_versions(id,user_id,version,wrapped_key,rotated_by) VALUES($1,$2,$3,$4,$2)`, newID, u.ID, newVersion, newWrapped); err != nil {
		internalError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO key_permissions(id,key_version_id,principal_type,principal_id,permissions) VALUES($1,$2,'owner',$3,'["decrypt","encrypt","rotate","delegate"]')`, id.New(), newID, u.ID); err != nil {
		internalError(w, r, err)
		return
	}
	for _, row := range people {
		emailCipher, fieldErr := s.reencryptField(oldKey, newKey, row.Email, "person:"+row.ID+":email")
		if fieldErr != nil {
			internalError(w, r, fieldErr)
			return
		}
		phoneCipher, fieldErr := s.reencryptField(oldKey, newKey, row.Phone, "person:"+row.ID+":phone")
		if fieldErr != nil {
			internalError(w, r, fieldErr)
			return
		}
		noteCipher, fieldErr := s.reencryptField(oldKey, newKey, row.Note, "person:"+row.ID+":note")
		if fieldErr != nil {
			internalError(w, r, fieldErr)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE people SET email_cipher=$2,phone_cipher=$3,note_cipher=$4,key_version=$5 WHERE id=$1`, row.ID, emailCipher, phoneCipher, noteCipher, newVersion); err != nil {
			internalError(w, r, err)
			return
		}
	}
	for _, row := range interactions {
		plain, e := s.store.Vault.Decrypt(oldKey, row.Value, "interaction:"+row.ID+":summary")
		if e != nil {
			internalError(w, r, e)
			return
		}
		cipher, e := s.store.Vault.Encrypt(newKey, plain, "interaction:"+row.ID+":summary")
		if e != nil {
			internalError(w, r, e)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE interactions SET summary_cipher=$2,key_version=$3 WHERE id=$1`, row.ID, cipher, newVersion); err != nil {
			internalError(w, r, err)
			return
		}
	}
	for _, row := range memories {
		plain, e := s.store.Vault.Decrypt(oldKey, row.Value, "memory:"+row.ID+":content")
		if e != nil {
			internalError(w, r, e)
			return
		}
		cipher, e := s.store.Vault.Encrypt(newKey, plain, "memory:"+row.ID+":content")
		if e != nil {
			internalError(w, r, e)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE memories SET content_cipher=$2,key_version=$3 WHERE id=$1`, row.ID, cipher, newVersion); err != nil {
			internalError(w, r, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		internalError(w, r, err)
		return
	}
	s.audit(r.Context(), u.ID, "key.rotate", "user_key", newID, r.RemoteAddr, map[string]int{"version": newVersion})
	writeJSON(w, 201, map[string]any{"id": newID, "version": newVersion, "reencrypted": map[string]int{"people": len(people), "interactions": len(interactions), "memories": len(memories)}})
}

func (s *Server) reencryptField(oldKey, newKey []byte, value, context string) (string, error) {
	plain, err := s.store.Vault.Decrypt(oldKey, value, context)
	if err != nil {
		return "", err
	}
	return s.store.Vault.Encrypt(newKey, plain, context)
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readEncryptedPeople(ctx context.Context, q queryer, userID string, version int) ([]encryptedPersonRow, error) {
	rows, err := q.Query(ctx, `SELECT id,email_cipher,phone_cipher,note_cipher FROM people WHERE user_id=$1 AND key_version=$2`, userID, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []encryptedPersonRow{}
	for rows.Next() {
		var row encryptedPersonRow
		if err := rows.Scan(&row.ID, &row.Email, &row.Phone, &row.Note); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
func readEncryptedRows(ctx context.Context, q queryer, query, userID string, version int) ([]encryptedContentRow, error) {
	rows, err := q.Query(ctx, query, userID, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []encryptedContentRow{}
	for rows.Next() {
		var row encryptedContentRow
		if err := rows.Scan(&row.ID, &row.Value); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

type APIKeyView struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	rows, err := s.store.DB.Query(r.Context(), `SELECT id,name,prefix,scopes,expires_at,last_used_at,revoked_at,created_at FROM api_keys WHERE user_id=$1 ORDER BY created_at DESC`, u.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer rows.Close()
	items := []APIKeyView{}
	for rows.Next() {
		var item APIKeyView
		var scopes []byte
		if err := rows.Scan(&item.ID, &item.Name, &item.Prefix, &scopes, &item.ExpiresAt, &item.LastUsedAt, &item.RevokedAt, &item.CreatedAt); err != nil {
			internalError(w, r, err)
			return
		}
		_ = json.Unmarshal(scopes, &item.Scopes)
		items = append(items, item)
	}
	writeJSON(w, 200, map[string]any{"api_keys": items})
}
func (s *Server) createAPIKey(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	var in struct {
		Name      string     `json:"name"`
		Scopes    []string   `json:"scopes"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeError(w, 400, "validation_error", "키 이름을 입력해 주세요.")
		return
	}
	for _, scope := range in.Scopes {
		if !validScope(scope) {
			writeError(w, 400, "validation_error", "API 권한 범위를 확인해 주세요.")
			return
		}
	}
	if len(in.Scopes) == 0 {
		var policy KeyPolicySettings
		_ = s.readSetting(r.Context(), "security", "key_policy", &policy, nil)
		in.Scopes = policy.DefaultScopes
	}
	token := "orb_" + id.Token(32)
	prefix := token[:12]
	raw, _ := json.Marshal(in.Scopes)
	keyID := id.New()
	if _, err := s.store.DB.Exec(r.Context(), `INSERT INTO api_keys(id,user_id,name,prefix,secret_hash,scopes,expires_at) VALUES($1,$2,$3,$4,$5,$6::jsonb,$7)`, keyID, u.ID, in.Name, prefix, secure.SHA256(token), string(raw), in.ExpiresAt); err != nil {
		internalError(w, r, err)
		return
	}
	s.audit(r.Context(), u.ID, "api_key.create", "api_key", keyID, r.RemoteAddr, map[string]any{"scopes": in.Scopes})
	writeJSON(w, 201, map[string]any{"id": keyID, "token": token, "prefix": prefix, "warning": "이 키는 다시 표시되지 않습니다."})
}
func (s *Server) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	keyID := chi.URLParam(r, "keyID")
	tag, err := s.store.DB.Exec(r.Context(), `UPDATE api_keys SET revoked_at=now() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, keyID, u.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "활성 API 키를 찾을 수 없습니다.")
		return
	}
	s.audit(r.Context(), u.ID, "api_key.revoke", "api_key", keyID, r.RemoteAddr, nil)
	w.WriteHeader(204)
}

func (s *Server) listKeyPermissions(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.DB.Query(r.Context(), `SELECT kp.id,kp.key_version_id,k.user_id,u.display_name,k.version,k.status,kp.principal_type,kp.principal_id,kp.permissions,kp.created_at FROM key_permissions kp JOIN user_key_versions k ON k.id=kp.key_version_id JOIN users u ON u.id=k.user_id ORDER BY u.display_name,k.version DESC`)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var permissionID, keyID, userID, displayName, keyStatus, principalType, principalID string
		var version int
		var raw []byte
		var created time.Time
		if err := rows.Scan(&permissionID, &keyID, &userID, &displayName, &version, &keyStatus, &principalType, &principalID, &raw, &created); err != nil {
			internalError(w, r, err)
			return
		}
		var permissions []string
		_ = json.Unmarshal(raw, &permissions)
		items = append(items, map[string]any{"id": permissionID, "key_version_id": keyID, "user_id": userID, "user_name": displayName, "key_version": version, "key_status": keyStatus, "principal_type": principalType, "principal_id": principalID, "permissions": permissions, "created_at": created})
	}
	writeJSON(w, 200, map[string]any{"permissions": items})
}
func (s *Server) updateKeyPermission(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	permissionID := chi.URLParam(r, "permissionID")
	var in struct {
		Permissions []string `json:"permissions"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	allowed := map[string]bool{"decrypt": true, "encrypt": true, "rotate": true, "delegate": true}
	for _, p := range in.Permissions {
		if !allowed[p] {
			writeError(w, 400, "validation_error", "키 권한을 확인해 주세요.")
			return
		}
	}
	if len(in.Permissions) == 0 {
		writeError(w, 400, "validation_error", "권한은 한 개 이상 필요합니다.")
		return
	}
	raw, _ := json.Marshal(in.Permissions)
	tag, err := s.store.DB.Exec(r.Context(), `UPDATE key_permissions SET permissions=$2::jsonb WHERE id=$1`, permissionID, string(raw))
	if err != nil {
		internalError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "키 권한을 찾을 수 없습니다.")
		return
	}
	s.audit(r.Context(), u.ID, "key_permission.update", "key_permission", permissionID, r.RemoteAddr, map[string]any{"permissions": in.Permissions})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

var _ = errors.Is
