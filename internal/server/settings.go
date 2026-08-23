package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hkjang/orbit/internal/id"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

func (s *Server) getAdminSettings(w http.ResponseWriter, r *http.Request) {
	result := map[string]any{}
	var general map[string]any
	if err := s.readSetting(r.Context(), "system", "general", &general, nil); err != nil {
		internalError(w, r, err)
		return
	}
	result["system"] = general
	var oidc OIDCSettings
	var oidcSecret string
	if err := s.readSetting(r.Context(), "auth", "oidc", &oidc, &oidcSecret); err != nil {
		internalError(w, r, err)
		return
	}
	oidc.ClientSecret = ""
	result["auth"] = map[string]any{"oidc": oidc, "has_client_secret": oidcSecret != ""}
	var ai AISettings
	var aiKey string
	if err := s.readSetting(r.Context(), "ai", "provider", &ai, &aiKey); err != nil {
		internalError(w, r, err)
		return
	}
	ai.APIKey = ""
	result["ai"] = map[string]any{"provider": ai, "has_api_key": aiKey != ""}
	var workflow ApprovalSettings
	if err := s.readSetting(r.Context(), "workflow", "approval", &workflow, nil); err != nil {
		internalError(w, r, err)
		return
	}
	result["workflow"] = workflow
	var policy KeyPolicySettings
	if err := s.readSetting(r.Context(), "security", "key_policy", &policy, nil); err != nil {
		internalError(w, r, err)
		return
	}
	result["security"] = policy
	writeJSON(w, 200, map[string]any{"settings": result})
}

func (s *Server) updateAdminSettings(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	namespace := chi.URLParam(r, "namespace")
	switch namespace {
	case "system":
		var v struct {
			ServiceName  string `json:"service_name"`
			PublicURL    string `json:"public_url"`
			SessionHours int    `json:"session_hours"`
		}
		if !decodeJSON(w, r, &v) {
			return
		}
		if v.ServiceName == "" {
			v.ServiceName = "Orbit"
		}
		if validateURL(v.PublicURL) != nil || v.SessionHours < 1 || v.SessionHours > 720 {
			writeError(w, 400, "validation_error", "서비스 URL 또는 세션 시간을 확인해 주세요.")
			return
		}
		s.saveSetting(w, r, u, "system", "general", v, "")
	case "auth":
		var v OIDCSettings
		if !decodeJSON(w, r, &v) {
			return
		}
		var current OIDCSettings
		var currentSecret string
		if err := s.readSetting(r.Context(), "auth", "oidc", &current, &currentSecret); err != nil {
			internalError(w, r, err)
			return
		}
		if v.DisplayName == "" {
			v.DisplayName = "Keycloak SSO"
		}
		if v.DefaultRole != "member" && v.DefaultRole != "team_lead" {
			v.DefaultRole = "member"
		}
		if v.Enabled && (validateURL(v.IssuerURL) != nil || v.ClientID == "") {
			writeError(w, 400, "validation_error", "OIDC Issuer URL과 Client ID를 확인해 주세요.")
			return
		}
		secret := v.ClientSecret
		clearSecret := v.ClearClientSecret
		if v.Enabled && (clearSecret || (secret == "" && currentSecret == "")) {
			writeError(w, 400, "validation_error", "활성화된 OIDC에는 Client Secret이 필요합니다.")
			return
		}
		v.ClientSecret = ""
		v.ClearClientSecret = false
		s.saveSetting(w, r, u, "auth", "oidc", v, secret, clearSecret)
	case "ai":
		var v AISettings
		if !decodeJSON(w, r, &v) {
			return
		}
		if v.Provider == "" {
			v.Provider = "openai-compatible"
		}
		if v.MaxOutputTokens < 1 || v.MaxOutputTokens > 262144 {
			writeError(w, 400, "validation_error", "최대 출력 토큰은 1~262,144 범위여야 합니다.")
			return
		}
		if v.RequestTimeoutSeconds < 5 || v.RequestTimeoutSeconds > 600 {
			writeError(w, 400, "validation_error", "AI 제한 시간은 5~600초 범위여야 합니다.")
			return
		}
		if v.Enabled && (validateURL(v.BaseURL) != nil || v.Model == "") {
			writeError(w, 400, "validation_error", "AI Base URL과 모델을 확인해 주세요.")
			return
		}
		secret := v.APIKey
		clearSecret := v.ClearAPIKey
		v.APIKey = ""
		v.ClearAPIKey = false
		s.saveSetting(w, r, u, "ai", "provider", v, secret, clearSecret)
	case "workflow":
		var v ApprovalSettings
		if !decodeJSON(w, r, &v) {
			return
		}
		if v.ReviewerRole != "team_lead" && v.ReviewerRole != "admin" {
			v.ReviewerRole = "team_lead"
		}
		allowed := map[string]bool{"memory": true}
		for _, resource := range v.ResourceTypes {
			if !allowed[resource] {
				writeError(w, 400, "validation_error", "지원하지 않는 승인 대상입니다.")
				return
			}
		}
		s.saveSetting(w, r, u, "workflow", "approval", v, "")
	case "security":
		var v KeyPolicySettings
		if !decodeJSON(w, r, &v) {
			return
		}
		if v.RotationDays < 1 || v.RotationDays > 3650 {
			writeError(w, 400, "validation_error", "키 회전 주기는 1~3,650일 범위여야 합니다.")
			return
		}
		for _, scope := range v.DefaultScopes {
			if !validScope(scope) {
				writeError(w, 400, "validation_error", "API 키 기본 권한을 확인해 주세요.")
				return
			}
		}
		s.saveSetting(w, r, u, "security", "key_policy", v, "")
	default:
		writeError(w, 404, "not_found", "설정 영역을 찾을 수 없습니다.")
	}
}

func (s *Server) saveSetting(w http.ResponseWriter, r *http.Request, u User, namespace, key string, value any, newSecret string, clearSecret ...bool) {
	raw, err := json.Marshal(value)
	if err != nil {
		internalError(w, r, err)
		return
	}
	encrypted := ""
	if newSecret != "" {
		encrypted, err = s.store.Vault.EncryptSystem(newSecret, namespace+":"+key)
		if err != nil {
			internalError(w, r, err)
			return
		}
	}
	if len(clearSecret) > 0 && clearSecret[0] {
		_, err = s.store.DB.Exec(r.Context(), `UPDATE settings SET value=$3::jsonb,encrypted_value='',updated_by=$4,updated_at=now() WHERE namespace=$1 AND key=$2`, namespace, key, string(raw), u.ID)
	} else if newSecret == "" {
		_, err = s.store.DB.Exec(r.Context(), `UPDATE settings SET value=$3::jsonb,updated_by=$4,updated_at=now() WHERE namespace=$1 AND key=$2`, namespace, key, string(raw), u.ID)
	} else {
		_, err = s.store.DB.Exec(r.Context(), `UPDATE settings SET value=$3::jsonb,encrypted_value=$4,updated_by=$5,updated_at=now() WHERE namespace=$1 AND key=$2`, namespace, key, string(raw), encrypted, u.ID)
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	s.audit(r.Context(), u.ID, "setting.update", "setting", namespace+":"+key, r.RemoteAddr, map[string]string{"namespace": namespace})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.DB.Query(r.Context(), `SELECT id,username,email,display_name,role,status,last_login_at,created_at FROM users ORDER BY created_at`)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Role, &u.Status, &u.LastLoginAt, &u.CreatedAt); err != nil {
			internalError(w, r, err)
			return
		}
		users = append(users, u)
	}
	writeJSON(w, 200, map[string]any{"users": users})
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	actor := userFromContext(r.Context())
	var in struct {
		Username    string `json:"username"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
		Password    string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.Username == "" || in.DisplayName == "" {
		writeError(w, 400, "validation_error", "사용자 ID와 이름을 입력해 주세요.")
		return
	}
	if in.Role != "admin" && in.Role != "team_lead" && in.Role != "member" {
		writeError(w, 400, "validation_error", "역할을 확인해 주세요.")
		return
	}
	hash := ""
	var err error
	if in.Password != "" {
		if len(in.Password) < 10 {
			writeError(w, 400, "validation_error", "비밀번호는 10자 이상이어야 합니다.")
			return
		}
		raw, e := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		err = e
		hash = string(raw)
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	u := User{ID: id.New(), Username: in.Username, Email: strings.TrimSpace(in.Email), DisplayName: in.DisplayName, Role: in.Role, Status: "active"}
	tx, err := s.store.DB.Begin(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `INSERT INTO users(id,username,email,display_name,password_hash,role) VALUES($1,$2,$3,$4,$5,$6)`, u.ID, u.Username, u.Email, u.DisplayName, hash, u.Role); err != nil {
		writeError(w, 409, "user_exists", "같은 사용자 ID가 이미 있습니다.")
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO user_preferences(user_id) VALUES($1)`, u.ID); err != nil {
		internalError(w, r, err)
		return
	}
	key, err := s.store.Vault.NewDataKey()
	if err != nil {
		internalError(w, r, err)
		return
	}
	wrapped, err := s.store.Vault.WrapKey(key)
	if err != nil {
		internalError(w, r, err)
		return
	}
	keyID := id.New()
	if _, err = tx.Exec(r.Context(), `INSERT INTO user_key_versions(id,user_id,version,wrapped_key,rotated_by) VALUES($1,$2,1,$3,$4)`, keyID, u.ID, wrapped, actor.ID); err != nil {
		internalError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO key_permissions(id,key_version_id,principal_type,principal_id,permissions) VALUES($1,$2,'owner',$3,'["decrypt","encrypt","rotate","delegate"]')`, id.New(), keyID, u.ID); err != nil {
		internalError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internalError(w, r, err)
		return
	}
	s.audit(r.Context(), actor.ID, "user.create", "user", u.ID, r.RemoteAddr, map[string]string{"role": u.Role})
	writeJSON(w, 201, map[string]string{"id": u.ID})
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	actor := userFromContext(r.Context())
	userID := chi.URLParam(r, "userID")
	var in struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
		Status      string `json:"status"`
		Password    string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.DisplayName == "" || (in.Role != "admin" && in.Role != "team_lead" && in.Role != "member") || (in.Status != "active" && in.Status != "disabled") {
		writeError(w, 400, "validation_error", "사용자 정보를 확인해 주세요.")
		return
	}
	if actor.ID == userID && (in.Role != "admin" || in.Status != "active") {
		writeError(w, 409, "self_lockout", "자신의 관리자 권한을 제거하거나 계정을 비활성화할 수 없습니다.")
		return
	}
	var hash *string
	if in.Password != "" {
		if len(in.Password) < 10 {
			writeError(w, 400, "validation_error", "비밀번호는 10자 이상이어야 합니다.")
			return
		}
		raw, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			internalError(w, r, err)
			return
		}
		v := string(raw)
		hash = &v
	}
	tag, err := s.store.DB.Exec(r.Context(), `UPDATE users SET email=$2,display_name=$3,role=$4,status=$5,password_hash=coalesce($6,password_hash),updated_at=now() WHERE id=$1`, userID, in.Email, in.DisplayName, in.Role, in.Status, hash)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "사용자를 찾을 수 없습니다.")
		return
	}
	s.audit(r.Context(), actor.ID, "user.update", "user", userID, r.RemoteAddr, map[string]string{"role": in.Role, "status": in.Status})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	query := escapeLike(strings.TrimSpace(r.URL.Query().Get("q")))
	rows, err := s.store.DB.Query(r.Context(), `SELECT a.id,a.actor_id,coalesce(u.display_name,'시스템'),a.action,a.resource_type,a.resource_id,a.ip_address,a.detail,a.created_at FROM audit_logs a LEFT JOIN users u ON u.id=a.actor_id WHERE ($1='' OR a.action=$1) AND ($2='' OR a.action ILIKE '%'||$2||'%' ESCAPE '\' OR a.resource_type ILIKE '%'||$2||'%' ESCAPE '\' OR a.resource_id ILIKE '%'||$2||'%' ESCAPE '\' OR a.ip_address ILIKE '%'||$2||'%' ESCAPE '\' OR coalesce(u.display_name,'') ILIKE '%'||$2||'%' ESCAPE '\') ORDER BY a.created_at DESC LIMIT 500`, action, query)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var row struct {
			ID                                                     int64
			ActorID                                                *string
			ActorName, Action, ResourceType, ResourceID, IPAddress string
			Detail                                                 []byte
			CreatedAt                                              time.Time
		}
		if err := rows.Scan(&row.ID, &row.ActorID, &row.ActorName, &row.Action, &row.ResourceType, &row.ResourceID, &row.IPAddress, &row.Detail, &row.CreatedAt); err != nil {
			internalError(w, r, err)
			return
		}
		var detail any
		_ = json.Unmarshal(row.Detail, &detail)
		items = append(items, map[string]any{"id": row.ID, "actor_id": row.ActorID, "actor_name": row.ActorName, "action": row.Action, "resource_type": row.ResourceType, "resource_id": row.ResourceID, "ip_address": row.IPAddress, "detail": detail, "created_at": row.CreatedAt})
	}
	if err := rows.Err(); err != nil {
		internalError(w, r, err)
		return
	}
	// 고를 수 있는 액션 목록은 로그에 실제로 있는 것만 내려준다. 화면이 종류를
	// 하드코딩하면 새 감사 액션이 생겨도 거를 방법이 없다.
	actionRows, err := s.store.DB.Query(r.Context(), `SELECT DISTINCT action FROM audit_logs ORDER BY action`)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer actionRows.Close()
	actions := []string{}
	for actionRows.Next() {
		var name string
		if err := actionRows.Scan(&name); err != nil {
			internalError(w, r, err)
			return
		}
		actions = append(actions, name)
	}
	if err := actionRows.Err(); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"audit_logs": items, "actions": actions})
}

func validScope(scope string) bool {
	switch scope {
	case "people:read", "people:write", "memories:read", "memories:write", "orbit:read", "ai:invoke", "mcp:use":
		return true
	}
	return false
}

var _ = context.Background
var _ = errors.Is
var _ = fmt.Sprintf
var _ = pgx.ErrNoRows
