package server

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// MCP 를 개인 키 없이 Keycloak 토큰으로.
//
// PKCE·리다이렉트·코드 교환은 Keycloak 과 클라이언트의 일이다. 이 서버의 몫은
// 리소스 서버 절반이고, 여기서 붙드는 것도 그것이다: 인증 서버가 어디인지 말하고,
// 401 을 그리로 가는 안내로 바꾸고, 그 서버가 이 리소스를 위해 발급한 토큰만,
// Orbit 이 이미 아는 사람에게, 키가 가졌을 만큼의 권한으로 받는다.

// fakeIDP 는 진짜 RSA 키 쌍으로 JWT 를 서명하고 discovery·JWKS 를 서빙한다.
type fakeIDP struct {
	server *httptest.Server
	key    *rsa.PrivateKey
}

func newIDP(t *testing.T) *fakeIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	idp := &fakeIDP{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                idp.server.URL,
			"authorization_endpoint":                idp.server.URL + "/authorize",
			"token_endpoint":                        idp.server.URL + "/token",
			"jwks_uri":                              idp.server.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		pub := key.Public().(*rsa.PublicKey)
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": "test", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})
	idp.server = httptest.NewServer(mux)
	t.Cleanup(idp.server.Close)
	return idp
}

func jwtSegment(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// sign 은 realm 키로 서명한 RS256 토큰이다 — Keycloak 이 주는 것과 같은 모양.
func (idp *fakeIDP) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	input := jwtSegment(t, map[string]string{"alg": "RS256", "typ": "JWT", "kid": "test"}) + "." + jwtSegment(t, claims)
	digest := sha256.Sum256([]byte(input))
	signature, err := rsa.SignPKCS1v15(rand.Reader, idp.key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(signature)
}

// signHS 는 대칭 키 서명이다. 공개 JWKS 로는 검증할 수 없으니 거부되어야 한다.
func (idp *fakeIDP) signHS(t *testing.T, claims map[string]any) string {
	t.Helper()
	input := jwtSegment(t, map[string]string{"alg": "HS256", "typ": "JWT", "kid": "test"}) + "." + jwtSegment(t, claims)
	mac := hmac.New(sha256.New, []byte("shared-secret"))
	mac.Write([]byte(input))
	return input + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// accessToken 은 사람이 로그인한 뒤 Keycloak 이 MCP 클라이언트에 건네는 것이다:
// realm 키 서명, 이 issuer, 어떤 대상.
func accessToken(t *testing.T, idp *fakeIDP, audience any, extra map[string]any) map[string]any {
	t.Helper()
	claims := map[string]any{
		"iss": idp.server.URL, "aud": audience, "sub": "subject-mcp",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
		"typ": "Bearer", "preferred_username": "ssomember", "email": "sso@example.test", "email_verified": true,
	}
	for k, v := range extra {
		claims[k] = v
	}
	return claims
}

const testResource = "https://orbit.example.test/mcp"

func testOAuth(issuer string) mcpOAuth {
	return mcpOAuth{
		MCPOAuthSettings: MCPOAuthSettings{Enabled: true, Scopes: []string{"people:read", "memories:read"}},
		Issuer:           issuer,
		ClientID:         "orbit-web",
		PublicURL:        "https://orbit.example.test",
	}
}

func TestLooksLikeJWT(t *testing.T) {
	for token, want := range map[string]bool{"a.b.c": true, "orb_abc": false, "a.b": false, "a..c": false, ".b.c": false, "": false} {
		if got := looksLikeJWT(token); got != want {
			t.Errorf("%q: got %v want %v", token, got, want)
		}
	}
}

func TestMCPOAuthResourceAndMetadataURL(t *testing.T) {
	o := mcpOAuth{PublicURL: "https://orbit.example.test/"}
	if got := o.resource(); got != testResource {
		t.Errorf("resource from public URL: %q", got)
	}
	if got := o.metadataURL(); got != "https://orbit.example.test/.well-known/oauth-protected-resource/mcp" {
		t.Errorf("metadata URL: %q", got)
	}
	o.Resource = "https://mcp.example.test/orbit/mcp"
	if got := o.metadataURL(); got != "https://mcp.example.test/.well-known/oauth-protected-resource/orbit/mcp" {
		t.Errorf("metadata URL with explicit resource: %q", got)
	}
	// 켜는 조건 셋: 스위치, issuer, 리소스 식별자.
	for name, o := range map[string]mcpOAuth{
		"off":         {MCPOAuthSettings: MCPOAuthSettings{Enabled: false}, Issuer: "https://kc", PublicURL: "https://o"},
		"no issuer":   {MCPOAuthSettings: MCPOAuthSettings{Enabled: true}, PublicURL: "https://o"},
		"no resource": {MCPOAuthSettings: MCPOAuthSettings{Enabled: true}, Issuer: "https://kc"},
	} {
		if ok, why := o.usable(); ok || why == "" {
			t.Errorf("%s: usable=%v reason=%q", name, ok, why)
		}
	}
	if ok, _ := (mcpOAuth{MCPOAuthSettings: MCPOAuthSettings{Enabled: true}, Issuer: "https://kc", PublicURL: "https://o"}).usable(); !ok {
		t.Error("fully configured must be usable")
	}
}

func TestMCPOAuthAcceptedAudiences(t *testing.T) {
	o := testOAuth("https://kc")
	o.Audience = []string{" claude-mcp ", "", "claude-mcp"}
	got := o.acceptedAudiences()
	want := []string{testResource, "claude-mcp", "orbit-web"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("accepted %v want %v", got, want)
	}
}

// guards: verifyMCPToken — 서명·iss·exp·nbf·typ·cnf·sub·대상 을 하나씩.
func TestVerifyMCPTokenAcceptsOnlyTokensForThisResource(t *testing.T) {
	idp := newIDP(t)
	other := newIDP(t)
	s := &Server{}
	o := testOAuth(idp.server.URL)

	accepted := map[string]string{
		"aud names the resource":           idp.sign(t, accessToken(t, idp, testResource, nil)),
		"aud list includes the resource":   idp.sign(t, accessToken(t, idp, []string{"account", testResource}, nil)),
		"azp is the web client":            idp.sign(t, accessToken(t, idp, "account", map[string]any{"azp": "orbit-web"})),
		"nbf in the past and typ Bearer":   idp.sign(t, accessToken(t, idp, testResource, map[string]any{"nbf": time.Now().Add(-time.Minute).Unix()})),
		"scope carries the app vocabulary": idp.sign(t, accessToken(t, idp, testResource, map[string]any{"scope": "openid people:read"})),
	}
	for name, token := range accepted {
		identity, refusal := s.verifyMCPToken(context.Background(), o, token)
		if refusal != nil {
			t.Errorf("%s: refused: %s / %v", name, refusal.reason, refusal.err)
			continue
		}
		if identity.Subject != "subject-mcp" || identity.Email != "sso@example.test" || !identity.EmailVerified {
			t.Errorf("%s: identity %+v", name, identity)
		}
	}

	refused := map[string]struct {
		token  string
		reason string
	}{
		"other audience":   {idp.sign(t, accessToken(t, idp, "account", map[string]any{"azp": "claude-mcp"})), "audience not accepted"},
		"no audience":      {idp.sign(t, accessToken(t, idp, nil, nil)), "audience not accepted"},
		"expired":          {idp.sign(t, accessToken(t, idp, testResource, map[string]any{"exp": time.Now().Add(-time.Minute).Unix()})), "token rejected"},
		"other issuer":     {other.sign(t, accessToken(t, other, testResource, nil)), "token rejected"},
		"issuer forged":    {other.sign(t, accessToken(t, idp, testResource, nil)), "token rejected"},
		"HS256":            {idp.signHS(t, accessToken(t, idp, testResource, nil)), "token rejected"},
		"ID token":         {idp.sign(t, accessToken(t, idp, testResource, map[string]any{"typ": "ID"})), "token type not access"},
		"refresh token":    {idp.sign(t, accessToken(t, idp, testResource, map[string]any{"typ": "Refresh"})), "token type not access"},
		"cnf bound":        {idp.sign(t, accessToken(t, idp, testResource, map[string]any{"cnf": map[string]string{"jkt": "x"}})), "token bound to key (cnf)"},
		"nbf in future":    {idp.sign(t, accessToken(t, idp, testResource, map[string]any{"nbf": time.Now().Add(time.Minute).Unix()})), "token not yet valid"},
		"subject missing":  {idp.sign(t, accessToken(t, idp, testResource, map[string]any{"sub": ""})), "subject missing"},
		"not a jwt at all": {"not.a.jwt", "token rejected"},
	}
	for name, tc := range refused {
		_, refusal := s.verifyMCPToken(context.Background(), o, tc.token)
		if refusal == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if refusal.reason != tc.reason {
			t.Errorf("%s: reason %q want %q (%v)", name, refusal.reason, tc.reason, refusal.err)
		}
		// 어느 거부도 로그용 원인과 클라이언트용 메시지 중 하나를 비우지 않는다.
		if refusal.err == nil || refusal.message == "" {
			t.Errorf("%s: refusal lacks err or message: %+v", name, refusal)
		}
	}

	// 대상 거부는 본 값과 고칠 값을 말한다 — 운영자는 이 메시지 하나로 설정을 끝낸다.
	_, refusal := s.verifyMCPToken(context.Background(), o, refused["other audience"].token)
	for _, want := range []string{`aud=[account]`, `azp="claude-mcp"`, `"claude-mcp"`, testResource, "mcp.oauth.audience"} {
		if !strings.Contains(refusal.message, want) {
			t.Errorf("audience refusal %q lacks %q", refusal.message, want)
		}
	}
	// 관리자가 azp 를 허용 대상에 적으면 매퍼 없이 통과한다.
	o.Audience = []string{"claude-mcp"}
	if _, refusal := s.verifyMCPToken(context.Background(), o, refused["other audience"].token); refusal != nil {
		t.Errorf("azp in mcp.oauth.audience still refused: %v", refusal.err)
	}
}

func TestVerifyMCPTokenWhenDiscoveryFails(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	s := &Server{}
	o := testOAuth(dead.URL)
	_, refusal := s.verifyMCPToken(context.Background(), o, "a.b.c")
	if refusal == nil || refusal.reason != "discovery failed" || refusal.err == nil || refusal.message == "" {
		t.Fatalf("refusal %+v", refusal)
	}
}

// guards: oauthEffectiveScopes — 관리자 천장과 토큰 scope 의 교집합, 비면 거부.
func TestOAuthEffectiveScopes(t *testing.T) {
	o := mcpOAuth{MCPOAuthSettings: MCPOAuthSettings{Scopes: []string{"people:read", "memories:read", "mcp:use", "bogus"}}}
	scopes, refusal := oauthEffectiveScopes(o, []string{"openid", "profile", "email"})
	if refusal != nil || !scopes["people:read"] || !scopes["memories:read"] || !scopes["mcp:use"] || scopes["bogus"] || scopes["memories:write"] {
		t.Fatalf("no app vocabulary in token: scopes %v refusal %+v", scopes, refusal)
	}
	scopes, refusal = oauthEffectiveScopes(o, []string{"openid", "people:read", "memories:write"})
	if refusal != nil || !scopes["people:read"] || scopes["memories:read"] || scopes["memories:write"] || !scopes["mcp:use"] {
		t.Fatalf("intersection: scopes %v refusal %+v", scopes, refusal)
	}
	if scopes, refusal = oauthEffectiveScopes(o, []string{"memories:write"}); refusal == nil || refusal.reason != "token scope outside ceiling" || scopes != nil {
		t.Fatalf("disjoint must refuse, not return an empty list: %v %+v", scopes, refusal)
	}
	if scopes, refusal = oauthEffectiveScopes(mcpOAuth{}, nil); refusal == nil || refusal.reason != "scope ceiling empty" || scopes != nil {
		t.Fatalf("empty ceiling must refuse: %v %+v", scopes, refusal)
	}
	// mcp:use 만 적은 천장은 데이터 범위가 없으니 비어 있는 것과 같다.
	if _, refusal = oauthEffectiveScopes(mcpOAuth{MCPOAuthSettings: MCPOAuthSettings{Scopes: []string{"mcp:use"}}}, nil); refusal == nil {
		t.Fatal("mcp:use alone is not a data scope")
	}
}

// guards: requestHasScope, authInfo.external — 빈 값이 곧 '세션·무제한' 으로
// 읽히던 자리. SSO 주체가 그 분기로 떨어지면 관리자 천장이 무의미해진다.
func TestScopeGateTreatsOnlySessionsAsUnlimited(t *testing.T) {
	cases := map[string]struct {
		info authInfo
		want bool
	}{
		"session without scopes": {authInfo{Kind: authSession}, true},
		"key with scope":         {authInfo{Kind: authAPIKey, Scopes: map[string]bool{"people:read": true}}, true},
		"key without scope":      {authInfo{Kind: authAPIKey, Scopes: map[string]bool{}}, false},
		"oauth with scope":       {authInfo{Kind: authOAuth, Scopes: map[string]bool{"people:read": true}}, true},
		"oauth without scope":    {authInfo{Kind: authOAuth, Scopes: map[string]bool{}}, false},
		"unknown kind":           {authInfo{}, false},
	}
	for name, tc := range cases {
		r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		r = r.WithContext(context.WithValue(r.Context(), authContextKey, tc.info))
		if got := requestHasScope(r, "people:read"); got != tc.want {
			t.Errorf("%s: got %v want %v", name, got, tc.want)
		}
	}
	if requestHasScope(httptest.NewRequest(http.MethodPost, "/mcp", nil), "people:read") {
		t.Error("a request with no auth info must not be treated as a session")
	}
}

func TestWWWAuthenticateHeader(t *testing.T) {
	o := testOAuth("https://kc.example.test/realms/orbit")
	if got := wwwAuthenticate(mcpOAuth{}, nil); got != "" {
		t.Errorf("header with SSO off: %q", got)
	}
	got := wwwAuthenticate(o, nil)
	if !strings.HasPrefix(got, `Bearer realm="Orbit", resource_metadata="https://orbit.example.test/.well-known/oauth-protected-resource/mcp"`) || strings.Contains(got, "invalid_token") {
		t.Errorf("no-token header: %q", got)
	}
	got = wwwAuthenticate(o, refuse("audience not accepted", nil, "대상이 다르다"))
	if !strings.HasSuffix(got, `, error="invalid_token"`) || strings.Contains(got, "대상") {
		t.Errorf("refused-token header must stay ASCII and flag invalid_token: %q", got)
	}
}

// guards: authenticate — SSO 토큰은 /mcp 밖에서 받지 않는다. REST 401 에는
// resource_metadata 가 붙지 않고, DB 를 건드리기 전에 끝난다(store 가 nil).
func TestAuthenticateIgnoresJWTOutsideMCP(t *testing.T) {
	idp := newIDP(t)
	s := &Server{}
	token := idp.sign(t, accessToken(t, idp, testResource, nil))
	called := false
	handler := s.authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	for _, path := range []string{"/api/v1/people/", "/api/v1/admin/users", "/api/v1/me"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if called || w.Code != http.StatusUnauthorized {
			t.Errorf("%s: code %d called %v", path, w.Code, called)
		}
		if h := w.Header().Get("WWW-Authenticate"); h != "" {
			t.Errorf("%s: REST 401 carried %q", path, h)
		}
		if !strings.Contains(w.Body.String(), "로그인이 필요합니다.") {
			t.Errorf("%s: body %s", path, w.Body.String())
		}
	}
}

func TestNormalizeMCPOAuthSettings(t *testing.T) {
	v, err := normalizeMCPOAuthSettings(MCPOAuthSettings{Enabled: true, Resource: " https://orbit.example.test/mcp ", Audience: []string{"claude-mcp cursor", "claude-mcp"}, Scopes: []string{"people:read memories:read", "people:read"}})
	if err != nil {
		t.Fatal(err)
	}
	if v.Resource != testResource || strings.Join(v.Audience, ",") != "claude-mcp,cursor" || strings.Join(v.Scopes, ",") != "people:read,memories:read" {
		t.Fatalf("normalized %+v", v)
	}
	for name, in := range map[string]MCPOAuthSettings{
		"bad resource":    {Resource: "orbit.example.test/mcp"},
		"bad audience":    {Audience: []string{`claude"mcp`}},
		"unknown scope":   {Scopes: []string{"people:admin"}},
		"enabled, empty":  {Enabled: true, Scopes: nil},
		"enabled mcp:use": {Enabled: true, Scopes: []string{"mcp:use"}},
	} {
		if _, err := normalizeMCPOAuthSettings(in); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if v, err := normalizeMCPOAuthSettings(MCPOAuthSettings{Enabled: false}); err != nil || v.Audience == nil || v.Scopes == nil {
		t.Errorf("disabled with nothing set must be fine and serialize as empty lists: %+v %v", v, err)
	}
}
