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
