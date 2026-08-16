package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/hkjang/orbit/internal/id"
	"github.com/hkjang/orbit/internal/secure"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

type contextKey string

const userContextKey contextKey = "orbit-user"
const authContextKey contextKey = "orbit-auth"

type authInfo struct {
	APIKey bool
	Scopes map[string]bool
}

func userFromContext(ctx context.Context) User {
	u, _ := ctx.Value(userContextKey).(User)
	return u
}

func (s *Server) localLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	var u User
	var passwordHash string
	err := s.store.DB.QueryRow(r.Context(), `SELECT id,username,email,display_name,role,status,password_hash,created_at FROM users WHERE lower(username)=lower($1)`, strings.TrimSpace(body.Username)).Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Role, &u.Status, &passwordHash, &u.CreatedAt)
	valid := err == nil && u.Status == "active" && passwordHash != "" && bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(body.Password)) == nil
	if !valid {
		// Keep timing less distinguishable for unknown and OIDC-only accounts.
		if err != nil || passwordHash == "" {
			_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$7EqJtq98hPqEX7fNZaFWoO5gJH.j8N2z3QeXGgfZgD4dO6tA0nL0m"), []byte(body.Password))
		}
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "아이디 또는 비밀번호를 확인해 주세요.")
		return
	}
	if err := s.issueSession(w, r, u.ID); err != nil {
		internalError(w, r, err)
		return
	}
	_, _ = s.store.DB.Exec(r.Context(), `UPDATE users SET last_login_at=now() WHERE id=$1`, u.ID)
	s.audit(r.Context(), u.ID, "auth.login", "session", "", r.RemoteAddr, map[string]any{"method": "local"})
	writeJSON(w, 200, map[string]any{"user": u})
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, userID string) error {
	token := id.Token(32)
	hours := 12
	secureCookie := r.TLS != nil
	var general struct {
		SessionHours int    `json:"session_hours"`
		PublicURL    string `json:"public_url"`
	}
	if s.readSetting(r.Context(), "system", "general", &general, nil) == nil {
		if general.SessionHours >= 1 && general.SessionHours <= 720 {
			hours = general.SessionHours
		}
		secureCookie = secureCookie || strings.HasPrefix(strings.ToLower(general.PublicURL), "https://")
	}
	expires := time.Now().Add(time.Duration(hours) * time.Hour)
	if _, err := s.store.DB.Exec(r.Context(), `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)`, secure.SHA256(token), userID, expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: "orbit_session", Value: token, Path: "/", HttpOnly: true, Secure: secureCookie, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: hours * 3600})
	return nil
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("orbit_session"); err == nil {
		_, _ = s.store.DB.Exec(r.Context(), `DELETE FROM sessions WHERE token_hash=$1`, secure.SHA256(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: "orbit_session", Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var u User
		info := authInfo{Scopes: map[string]bool{}}
		var err error
		if bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); bearer != "" {
			u, info.Scopes, err = s.userByAPIKey(r.Context(), bearer)
			info.APIKey = true
		} else if cookie, cookieErr := r.Cookie("orbit_session"); cookieErr == nil {
			u, err = s.userBySession(r.Context(), cookie.Value)
		} else {
			err = errors.New("missing credentials")
		}
		if err != nil || u.Status != "active" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "로그인이 필요합니다.")
			return
		}
		if info.APIKey {
			required := requiredScope(r.Method, r.URL.Path)
			if required == "session-only" || (required != "" && !info.Scopes[required]) {
				writeError(w, http.StatusForbidden, "insufficient_scope", "API 키의 권한 범위가 부족합니다.")
				return
			}
		}
		ctx := context.WithValue(r.Context(), userContextKey, u)
		ctx = context.WithValue(ctx, authContextKey, info)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) userBySession(ctx context.Context, token string) (User, error) {
	var u User
	err := s.store.DB.QueryRow(ctx, `SELECT u.id,u.username,u.email,u.display_name,u.role,u.status,u.last_login_at,u.created_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now()`, secure.SHA256(token)).Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Role, &u.Status, &u.LastLoginAt, &u.CreatedAt)
	return u, err
}

func (s *Server) userByAPIKey(ctx context.Context, token string) (User, map[string]bool, error) {
	if !strings.HasPrefix(token, "orb_") || len(token) < 20 {
		return User{}, nil, errors.New("invalid key")
	}
	prefix := token[:12]
	rows, err := s.store.DB.Query(ctx, `SELECT k.id,k.secret_hash,k.scopes,u.id,u.username,u.email,u.display_name,u.role,u.status,u.last_login_at,u.created_at FROM api_keys k JOIN users u ON u.id=k.user_id WHERE k.prefix=$1 AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now())`, prefix)
	if err != nil {
		return User{}, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var keyID, hash string
		var u User
		var raw []byte
		if err := rows.Scan(&keyID, &hash, &raw, &u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Role, &u.Status, &u.LastLoginAt, &u.CreatedAt); err != nil {
			return User{}, nil, err
		}
		if subtle.ConstantTimeCompare([]byte(hash), []byte(secure.SHA256(token))) == 1 {
			_, _ = s.store.DB.Exec(ctx, `UPDATE api_keys SET last_used_at=now() WHERE id=$1`, keyID)
			var scopes []string
			_ = json.Unmarshal(raw, &scopes)
			allowed := map[string]bool{}
			for _, scope := range scopes {
				allowed[scope] = true
			}
			return u, allowed, nil
		}
	}
	return User{}, nil, errors.New("invalid key")
}

func requiredScope(method, path string) string {
	if strings.HasPrefix(path, "/api/v1/admin/") || strings.HasPrefix(path, "/api/v1/personal/") || path == "/api/v1/auth/logout" {
		return "session-only"
	}
	if path == "/mcp" {
		return "mcp:use"
	}
	if strings.HasPrefix(path, "/api/v1/people") {
		if method == http.MethodGet {
			return "people:read"
		}
		return "people:write"
	}
	if strings.HasPrefix(path, "/api/v1/memories") {
		if method == http.MethodGet {
			return "memories:read"
		}
		return "memories:write"
	}
	if strings.HasPrefix(path, "/api/v1/orbit") || strings.HasPrefix(path, "/api/v1/rediscover") {
		return "orbit:read"
	}
	if strings.HasPrefix(path, "/api/v1/ai/") {
		return "ai:invoke"
	}
	return ""
}

func (s *Server) requireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, role := range roles {
		allowed[role] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !allowed[userFromContext(r.Context()).Role] {
				writeError(w, http.StatusForbidden, "forbidden", "이 작업을 수행할 권한이 없습니다.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"user": userFromContext(r.Context()), "version": s.version})
}

func (s *Server) oidcConfig(ctx context.Context) (OIDCSettings, string, error) {
	var settings OIDCSettings
	var secret string
	if err := s.readSetting(ctx, "auth", "oidc", &settings, &secret); err != nil {
		return settings, "", err
	}
	settings.ClientSecret = secret
	if !settings.Enabled || settings.IssuerURL == "" || settings.ClientID == "" || secret == "" {
		return settings, "", errors.New("OIDC is not configured")
	}
	var general struct {
		PublicURL string `json:"public_url"`
	}
	if err := s.readSetting(ctx, "system", "general", &general, nil); err != nil {
		return settings, "", err
	}
	return settings, strings.TrimRight(general.PublicURL, "/") + "/api/v1/auth/oidc/callback", nil
}

func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	settings, redirectURL, err := s.oidcConfig(r.Context())
	if err != nil {
		writeError(w, 503, "oidc_unavailable", "SSO가 설정되지 않았습니다.")
		return
	}
	provider, err := oidc.NewProvider(r.Context(), settings.IssuerURL)
	if err != nil {
		internalError(w, r, fmt.Errorf("OIDC discovery: %w", err))
		return
	}
	state := id.Token(24)
	nonce := id.Token(24)
	payload, _ := json.Marshal(map[string]string{"state": state, "nonce": nonce})
	sealed, err := s.store.Vault.EncryptSystem(string(payload), "oidc-state")
	if err != nil {
		internalError(w, r, err)
		return
	}
	secureCookie := r.TLS != nil || strings.HasPrefix(strings.ToLower(strings.TrimSpace(redirectURL)), "https://")
	http.SetCookie(w, &http.Cookie{Name: "orbit_oidc_state", Value: sealed, Path: "/api/v1/auth/oidc/callback", HttpOnly: true, Secure: secureCookie, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	conf := oauth2.Config{ClientID: settings.ClientID, ClientSecret: settings.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: redirectURL, Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}
	http.Redirect(w, r, conf.AuthCodeURL(state, oidc.Nonce(nonce)), http.StatusFound)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("orbit_oidc_state")
	if err != nil {
		writeError(w, 400, "invalid_oidc_state", "SSO 요청이 만료되었습니다.")
		return
	}
	plain, err := s.store.Vault.DecryptSystem(cookie.Value, "oidc-state")
	if err != nil {
		writeError(w, 400, "invalid_oidc_state", "SSO 요청을 확인할 수 없습니다.")
		return
	}
	var stateData map[string]string
	if json.Unmarshal([]byte(plain), &stateData) != nil || subtle.ConstantTimeCompare([]byte(stateData["state"]), []byte(r.URL.Query().Get("state"))) != 1 {
		writeError(w, 400, "invalid_oidc_state", "SSO 상태가 일치하지 않습니다.")
		return
	}
	settings, redirectURL, err := s.oidcConfig(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	provider, err := oidc.NewProvider(r.Context(), settings.IssuerURL)
	if err != nil {
		internalError(w, r, err)
		return
	}
	conf := oauth2.Config{ClientID: settings.ClientID, ClientSecret: settings.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: redirectURL, Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}
	token, err := conf.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		writeError(w, 401, "oidc_exchange_failed", "SSO 인증을 완료하지 못했습니다.")
		return
	}
	rawID, _ := token.Extra("id_token").(string)
	idToken, err := provider.Verifier(&oidc.Config{ClientID: settings.ClientID}).Verify(r.Context(), rawID)
	if err != nil {
		writeError(w, 401, "invalid_id_token", "SSO 토큰을 확인하지 못했습니다.")
		return
	}
	var claims struct {
		Subject           string `json:"sub"`
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
		Nonce             string `json:"nonce"`
	}
	if idToken.Claims(&claims) != nil || claims.Subject == "" || claims.Nonce != stateData["nonce"] {
		writeError(w, 401, "invalid_id_token", "SSO 사용자 정보를 확인하지 못했습니다.")
		return
	}
	u, err := s.findOrProvisionOIDCUser(r.Context(), settings, claims.Subject, claims.Email, claims.EmailVerified, claims.PreferredUsername, claims.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 403, "provisioning_disabled", "관리자에게 사용자 등록을 요청해 주세요.")
			return
		}
		internalError(w, r, err)
		return
	}
	if u.Status != "active" {
		writeError(w, 403, "account_disabled", "비활성화된 사용자입니다.")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "orbit_oidc_state", Value: "", Path: "/api/v1/auth/oidc/callback", HttpOnly: true, Secure: strings.HasPrefix(strings.ToLower(strings.TrimSpace(redirectURL)), "https://"), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	if err = s.issueSession(w, r, u.ID); err != nil {
		internalError(w, r, err)
		return
	}
	s.audit(r.Context(), u.ID, "auth.login", "session", "", r.RemoteAddr, map[string]any{"method": "oidc"})
	http.Redirect(w, r, "/orbit", http.StatusFound)
}

func (s *Server) findOrProvisionOIDCUser(ctx context.Context, settings OIDCSettings, subject, email string, emailVerified bool, username, name string) (User, error) {
	var u User
	err := s.store.DB.QueryRow(ctx, `SELECT id,username,email,display_name,role,status,last_login_at,created_at FROM users WHERE oidc_subject=$1 OR ($3 AND email<>'' AND lower(email)=lower($2)) ORDER BY (oidc_subject=$1) DESC LIMIT 1`, subject, email, emailVerified).Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Role, &u.Status, &u.LastLoginAt, &u.CreatedAt)
	if err == nil {
		_, err = s.store.DB.Exec(ctx, `UPDATE users SET oidc_subject=$1,last_login_at=now(),updated_at=now() WHERE id=$2`, subject, u.ID)
		return u, err
	}
	if !errors.Is(err, pgx.ErrNoRows) || !settings.AutoProvision {
		return User{}, err
	}
	if username == "" {
		username = email
	}
	if username == "" {
		username = "oidc-" + subject[:min(8, len(subject))]
	}
	if name == "" {
		name = username
	}
	var usernameExists bool
	if err = s.store.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE lower(username)=lower($1))`, username).Scan(&usernameExists); err != nil {
		return User{}, err
	}
	if usernameExists {
		username += "-" + subject[:min(8, len(subject))]
	}
	u = User{ID: id.New(), Username: username, Email: email, DisplayName: name, Role: settings.DefaultRole, Status: "active", CreatedAt: time.Now()}
	if u.Role != "member" && u.Role != "team_lead" {
		u.Role = "member"
	}
	tx, err := s.store.DB.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO users(id,username,email,display_name,role,oidc_subject,last_login_at) VALUES($1,$2,$3,$4,$5,$6,now())`, u.ID, u.Username, u.Email, u.DisplayName, u.Role, subject); err != nil {
		return User{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO user_preferences(user_id) VALUES($1)`, u.ID); err != nil {
		return User{}, err
	}
	key, err := s.store.Vault.NewDataKey()
	if err != nil {
		return User{}, err
	}
	wrapped, err := s.store.Vault.WrapKey(key)
	if err != nil {
		return User{}, err
	}
	keyID := id.New()
	if _, err = tx.Exec(ctx, `INSERT INTO user_key_versions(id,user_id,version,wrapped_key,rotated_by) VALUES($1,$2,1,$3,$2)`, keyID, u.ID, wrapped); err != nil {
		return User{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO key_permissions(id,key_version_id,principal_type,principal_id,permissions) VALUES($1,$2,'owner',$3,'["decrypt","encrypt","rotate","delegate"]')`, id.New(), keyID, u.ID); err != nil {
		return User{}, err
	}
	return u, tx.Commit(ctx)
}

func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("HTTP(S) URL required")
	}
	return nil
}
