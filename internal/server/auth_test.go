package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/hkjang/orbit/internal/secure"
	"github.com/hkjang/orbit/internal/store"
)

func TestSilentOIDCRequestedNeedsAutoLogin(t *testing.T) {
	none := url.Values{"prompt": {"none"}}
	// auto_login이 꺼져 있으면 ?prompt=none을 붙여도 조용한 시도가 아니다.
	if silentOIDCRequested(OIDCSettings{Enabled: true}, none) {
		t.Fatal("prompt=none must be ignored while auto_login is off")
	}
	if !silentOIDCRequested(OIDCSettings{Enabled: true, AutoLogin: true}, none) {
		t.Fatal("prompt=none must be honoured when auto_login is on")
	}
	if silentOIDCRequested(OIDCSettings{Enabled: true, AutoLogin: true}, url.Values{}) {
		t.Fatal("a plain start must not become silent just because auto_login is on")
	}
}

func TestSafeReturnTo(t *testing.T) {
	cases := map[string]bool{
		"/":                     true,
		"/people/abc?tab=links": true,
		"":                      false,
		"//evil.example":        false,
		"/\\evil.example":       false,
		"https://evil.example":  false,
		"people":                false,
		"/x\r\nSet-Cookie: a=b": false,
	}
	for value, want := range cases {
		if got := safeReturnTo(value); got != want {
			t.Errorf("safeReturnTo(%q) = %v, want %v", value, got, want)
		}
	}
}

func testOIDCServer(t *testing.T) *Server {
	t.Helper()
	vault, err := secure.NewVault(bytes.Repeat([]byte("k"), 32))
	if err != nil {
		t.Fatal(err)
	}
	return &Server{store: &store.Store{Vault: vault}}
}

func sealedOIDCState(t *testing.T, s *Server, data map[string]string) *http.Cookie {
	t.Helper()
	payload, _ := json.Marshal(data)
	sealed, err := s.store.Vault.EncryptSystem(string(payload), "oidc-state")
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "orbit_oidc_state", Value: sealed}
}

func TestOIDCCallbackRefusal(t *testing.T) {
	s := testOIDCServer(t)
	cases := []struct {
		name   string
		state  map[string]string
		query  string
		want   string
		cookie bool
	}{
		// 조용한 시도가 login_required를 받으면 로그인 화면으로 가고, 주소에
		// 다시 시도하지 말라는 표시가 붙는다.
		{"silent login_required", map[string]string{"state": "s1", "silent": "true"}, "error=login_required&state=s1", "/login?sso=none", true},
		{"silent interaction_required", map[string]string{"state": "s1", "silent": "true"}, "error=interaction_required&state=s1", "/login?sso=none", true},
		// 평범한 로그인 중 제공자 오류는 오류 표시로 간다.
		{"plain error", map[string]string{"state": "s1"}, "error=access_denied&state=s1", "/login?sso=error", true},
		// state가 맞지 않으면 silent 표시를 믿지 않는다.
		{"state mismatch", map[string]string{"state": "s1", "silent": "true"}, "error=login_required&state=other", "/login?sso=error", true},
		{"no cookie", nil, "error=login_required&state=s1", "/login?sso=error", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?"+tc.query, nil)
			if tc.cookie {
				req.AddCookie(sealedOIDCState(t, s, tc.state))
			}
			rec := httptest.NewRecorder()
			s.oidcCallback(rec, req)
			if rec.Code != http.StatusFound {
				t.Fatalf("status %d, want 302: %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Location"); got != tc.want {
				t.Fatalf("redirected to %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOIDCCallbackRejectsBadStateWithoutProviderError(t *testing.T) {
	s := testOIDCServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=abc&state=s1", nil)
	req.AddCookie(sealedOIDCState(t, s, map[string]string{"state": "other"}))
	rec := httptest.NewRecorder()
	s.oidcCallback(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

// fakeOIDCProvider는 Discovery 문서만 답하고 토큰 교환은 거절하는 제공자다.
// 콜백이 코드 교환 실패를 어떻게 답하는지 보는 데 쓴다.
func fakeOIDCProvider(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeJSON(w, 200, map[string]any{
				"issuer":                 srv.URL,
				"authorization_endpoint": srv.URL + "/auth",
				"token_endpoint":         srv.URL + "/token",
				"jwks_uri":               srv.URL + "/keys",
			})
		case "/token":
			writeJSON(w, 400, map[string]any{"error": "invalid_grant"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// 조용한 시도는 사람이 아무 것도 누르지 않은 흐름이다. 그 시작이 Discovery
// 실패를 만나면 JSON 500이 아니라 로그인 화면으로 돌려보내야 아이디/비밀번호
// 로그인에 닿을 수 있다. 사람이 버튼을 눌러 시작한 흐름은 그대로 오류를 낸다.
func TestOIDCStartDiscoveryFailure(t *testing.T) {
	s := testOIDCServer(t)
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	settings := OIDCSettings{Enabled: true, AutoLogin: true, IssuerURL: dead.URL, ClientID: "orbit", ClientSecret: "x"}

	silent := httptest.NewRecorder()
	s.beginOIDC(silent, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/start?prompt=none&return_to=%2Fpeople", nil), settings, "http://orbit.example/api/v1/auth/oidc/callback")
	if silent.Code != http.StatusFound || silent.Header().Get("Location") != "/login?sso=error" {
		t.Fatalf("silent start on discovery failure: status %d location %q, want 302 to /login?sso=error: %s", silent.Code, silent.Header().Get("Location"), silent.Body.String())
	}

	plain := httptest.NewRecorder()
	s.beginOIDC(plain, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/start", nil), settings, "http://orbit.example/api/v1/auth/oidc/callback")
	if plain.Code != http.StatusInternalServerError {
		t.Fatalf("plain start on discovery failure: status %d, want 500", plain.Code)
	}
}

// 콜백에서 코드 교환이 실패했을 때도 같다: 조용한 시도는 로그인 화면으로,
// 사람이 시작한 흐름은 401 JSON으로.
func TestOIDCCallbackExchangeFailure(t *testing.T) {
	s := testOIDCServer(t)
	provider := fakeOIDCProvider(t)
	settings := OIDCSettings{Enabled: true, AutoLogin: true, IssuerURL: provider.URL, ClientID: "orbit", ClientSecret: "x"}
	redirectURL := "http://orbit.example/api/v1/auth/oidc/callback"

	silent := httptest.NewRecorder()
	s.completeOIDC(silent, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=abc&state=s1", nil), settings, redirectURL, map[string]string{"state": "s1", "silent": "true"})
	if silent.Code != http.StatusFound || silent.Header().Get("Location") != "/login?sso=error" {
		t.Fatalf("silent callback on exchange failure: status %d location %q, want 302 to /login?sso=error: %s", silent.Code, silent.Header().Get("Location"), silent.Body.String())
	}

	plain := httptest.NewRecorder()
	s.completeOIDC(plain, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=abc&state=s1", nil), settings, redirectURL, map[string]string{"state": "s1"})
	if plain.Code != http.StatusUnauthorized {
		t.Fatalf("plain callback on exchange failure: status %d, want 401: %s", plain.Code, plain.Body.String())
	}
}

// 콜백에서 Discovery가 실패하면 — 시작과 콜백 사이에 제공자가 죽은 경우 —
// 조용한 시도는 역시 로그인 화면으로 간다.
func TestOIDCCallbackDiscoveryFailure(t *testing.T) {
	s := testOIDCServer(t)
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	settings := OIDCSettings{Enabled: true, AutoLogin: true, IssuerURL: dead.URL, ClientID: "orbit", ClientSecret: "x"}
	rec := httptest.NewRecorder()
	s.completeOIDC(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=abc&state=s1", nil), settings, "http://orbit.example/api/v1/auth/oidc/callback", map[string]string{"state": "s1", "silent": "true"})
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login?sso=error" {
		t.Fatalf("status %d location %q, want 302 to /login?sso=error", rec.Code, rec.Header().Get("Location"))
	}
}

// 사용자 단계의 거절(미등록·비활성)도 조용한 시도에서는 JSON이 아니라 로그인
// 화면이다. 커밋이 약속한 "로그인하지 않은 브라우저에서는 로그인 화면이 뜬다"가
// 이 경우에도 지켜져야 한다.
func TestOIDCFailureAnswersSilentFlowWithRedirect(t *testing.T) {
	for _, code := range []string{"provisioning_disabled", "account_disabled", "invalid_id_token"} {
		rec := httptest.NewRecorder()
		oidcFailure(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback", nil), true, 403, code, "x", nil)
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login?sso=error" {
			t.Fatalf("%s silent: status %d location %q, want 302 to /login?sso=error", code, rec.Code, rec.Header().Get("Location"))
		}
	}
	rec := httptest.NewRecorder()
	oidcFailure(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback", nil), false, 403, "account_disabled", "x", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("plain: status %d, want 403", rec.Code)
	}
}
