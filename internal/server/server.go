package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/hkjang/orbit/internal/store"
	"github.com/hkjang/orbit/internal/webui"
)

type Server struct {
	store   *store.Store
	version string
	commit  string
	builtAt string
}

func New(st *store.Store, version, commit, builtAt string) http.Handler {
	s := &Server{store: st, version: version, commit: commit, builtAt: builtAt}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer, s.securityHeaders)
	r.Use(middleware.Timeout(11 * time.Minute))
	r.Get("/healthz", s.health)
	r.Get("/readyz", s.ready)
	r.Get("/api/v1/public/config", s.publicConfig)
	r.Post("/api/v1/auth/login", s.localLogin)
	r.Get("/api/v1/auth/oidc/start", s.oidcStart)
	r.Get("/api/v1/auth/oidc/callback", s.oidcCallback)
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(s.authenticate)
		api.Post("/auth/logout", s.logout)
		api.Get("/me", s.me)
		api.Get("/orbit", s.getOrbit)
		api.Route("/people", func(pr chi.Router) {
			pr.Get("/", s.listPeople)
			pr.Post("/", s.createPerson)
			pr.Get("/{personID}", s.getPerson)
			pr.Put("/{personID}", s.updatePerson)
			pr.Delete("/{personID}", s.deletePerson)
			pr.Post("/{personID}/interactions", s.createInteraction)
			pr.Post("/{personID}/anchor", s.setAnchor)
		})
		api.Route("/memories", func(mr chi.Router) {
			mr.Get("/", s.listMemories)
			mr.Post("/", s.createMemory)
		})
		api.Get("/rediscover", s.rediscover)
		api.Get("/approvals", s.listApprovals)
		api.Post("/approvals/{approvalID}/review", s.reviewApproval)
		api.Post("/ai/stream", s.streamAI)
		api.Route("/personal", func(p chi.Router) {
			p.Get("/preferences", s.getPreferences)
			p.Put("/preferences", s.updatePreferences)
			p.Get("/keys", s.listKeys)
			p.Post("/keys/rotate", s.rotateKey)
			p.Get("/api-keys", s.listAPIKeys)
			p.Post("/api-keys", s.createAPIKey)
			p.Delete("/api-keys/{keyID}", s.revokeAPIKey)
		})
		api.Route("/admin", func(a chi.Router) {
			a.Use(s.requireRole("admin"))
			a.Get("/settings", s.getAdminSettings)
			a.Put("/settings/{namespace}", s.updateAdminSettings)
			a.Get("/users", s.listUsers)
			a.Post("/users", s.createUser)
			a.Put("/users/{userID}", s.updateUser)
			a.Get("/audit", s.listAudit)
			a.Get("/key-permissions", s.listKeyPermissions)
			a.Put("/key-permissions/{permissionID}", s.updateKeyPermission)
		})
	})
	r.Handle("/mcp", s.authenticate(http.HandlerFunc(s.mcp)))
	r.Get("/openapi.json", s.openAPI)
	r.Handle("/*", s.spa())
	return r
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; connect-src 'self'; font-src 'self' data:; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.Ping(r.Context()); err != nil {
		writeError(w, 503, "not_ready", "database unavailable")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ready"})
}

func (s *Server) publicConfig(w http.ResponseWriter, r *http.Request) {
	oidc := OIDCSettings{}
	_ = s.readSetting(r.Context(), "auth", "oidc", &oidc, nil)
	serviceName := "Orbit"
	var general struct {
		ServiceName string `json:"service_name"`
	}
	if s.readSetting(r.Context(), "system", "general", &general, nil) == nil && general.ServiceName != "" {
		serviceName = general.ServiceName
	}
	writeJSON(w, 200, map[string]any{
		"service_name": serviceName, "version": s.version, "commit": s.commit, "built_at": s.builtAt,
		"oidc": map[string]any{"enabled": oidc.Enabled, "display_name": oidc.DisplayName},
	})
}

func (s *Server) spa() http.Handler {
	dist, err := fs.Sub(webui.Dist, "dist")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "." {
			if _, err := fs.Stat(dist, name); err == nil {
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			http.Error(w, "UI unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}

func (s *Server) readSetting(ctx context.Context, namespace, key string, dst any, secret *string) error {
	var raw []byte
	var encrypted string
	err := s.store.DB.QueryRow(ctx, `SELECT value, encrypted_value FROM settings WHERE namespace=$1 AND key=$2`, namespace, key).Scan(&raw, &encrypted)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return err
	}
	if secret != nil && encrypted != "" {
		plain, err := s.store.Vault.DecryptSystem(encrypted, namespace+":"+key)
		if err != nil {
			return err
		}
		*secret = plain
	}
	return nil
}

func (s *Server) audit(ctx context.Context, userID, action, resourceType, resourceID, ip string, detail any) {
	raw, _ := json.Marshal(detail)
	if _, err := s.store.DB.Exec(ctx, `INSERT INTO audit_logs(actor_id,action,resource_type,resource_id,ip_address,detail) VALUES(NULLIF($1,'')::uuid,$2,$3,$4,$5,$6::jsonb)`, userID, action, resourceType, resourceID, ip, string(raw)); err != nil {
		slog.Warn("audit write failed", "error", err)
	}
}
