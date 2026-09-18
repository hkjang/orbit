package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
)

// MCP 를 개인 키 없이 Keycloak 액세스 토큰으로.
//
// MCP 인가 규격(2025-06-18 이후)은 OAuth 2.1 이고, 이 서버는 그 안에서 리소스
// 서버다. 하는 일은 셋뿐이다 — 인증 서버가 어디인지 알리고(RFC 9728 메타데이터),
// 401 에 그 문서의 주소를 붙이고, 내민 토큰이 Keycloak 이 이 서버를 위해 발급한
// 것인지 검사한다. 토큰 발급·로그인 화면·클라이언트 등록은 전부 Keycloak 의 몫이다.
//
// 개인 키(orb_)는 그대로 둔다. 토큰은 같은 방으로 들어오는 두 번째 문이다:
// 이미 등록된 활성 계정만 열고, 범위는 관리자가 정한 천장을 넘지 않으며, 계정을
// 만들거나 role claim 으로 권한을 올리지 않는다. /mcp 에서만 받는다.

// MCPOAuthSettings 는 settings 표의 mcp:oauth 행이다. 키 이름은 표준
// (mcp.oauth.enabled·resource·audience·scopes)을 그대로 따른다.
type MCPOAuthSettings struct {
	Enabled bool `json:"enabled"`
	// Resource 는 이 서버가 주장하는 리소스 식별자(RFC 8707). 비면 일반 설정의
	// 공개 URL + /mcp 로 만든다.
	Resource string `json:"resource"`
	// Audience 는 관리자가 적는 허용 대상. 토큰의 aud 또는 azp 와 비교한다.
	Audience []string `json:"audience"`
	// Scopes 는 SSO 토큰 주체에게 주는 범위의 천장. 토큰이 이 앱의 범위 어휘를
	// 싣고 오면 그 교집합만 준다.
	Scopes []string `json:"scopes"`
}

// defaultMCPOAuthScopes 는 키 정책의 기본 범위와 같은 읽기 전용 집합이다.
var defaultMCPOAuthScopes = []string{"people:read", "memories:read"}

// mcpOAuth 는 요청 하나를 판단하는 데 필요한 설정을 한데 모은 것이다.
type mcpOAuth struct {
	MCPOAuthSettings
	// Issuer 와 ClientID 는 웹 로그인의 oidc 설정에서 재사용한다.
	Issuer   string
	ClientID string
	// PublicURL 은 일반 설정의 서비스 공개 URL. Resource 가 비었을 때의 근거다.
	PublicURL string
}

func (s *Server) mcpOAuthConfig(ctx context.Context) (mcpOAuth, error) {
	o := mcpOAuth{MCPOAuthSettings: MCPOAuthSettings{Scopes: defaultMCPOAuthScopes}}
	// 행이 없는 설치(마이그레이션 전)는 꺼진 것과 같다.
	if err := s.readSetting(ctx, "mcp", "oauth", &o.MCPOAuthSettings, nil); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return o, err
	}
	var oidcSettings OIDCSettings
	if err := s.readSetting(ctx, "auth", "oidc", &oidcSettings, nil); err != nil {
		return o, err
	}
	o.Issuer = strings.TrimRight(strings.TrimSpace(oidcSettings.IssuerURL), "/")
	o.ClientID = strings.TrimSpace(oidcSettings.ClientID)
	var general struct {
		PublicURL string `json:"public_url"`
	}
	if err := s.readSetting(ctx, "system", "general", &general, nil); err != nil {
		return o, err
	}
	o.PublicURL = strings.TrimSpace(general.PublicURL)
	return o, nil
}

// resource 는 메타데이터가 알리고 토큰의 aud 가 가리켜야 하는 식별자다. 요청의
// Host 헤더로 만들지 않는다 — 누구나 바꿀 수 있는 값이다.
func (o mcpOAuth) resource() string {
	if r := strings.TrimSpace(o.Resource); r != "" {
		return r
	}
	if o.PublicURL == "" {
		return ""
	}
	return strings.TrimRight(o.PublicURL, "/") + "/mcp"
}

// metadataURL 은 거부된 클라이언트가 인증 서버를 찾으러 가는 곳이다.
func (o mcpOAuth) metadataURL() string {
	u, err := url.Parse(o.resource())
	if err != nil || u.Host == "" {
		return ""
	}
	u.Path = "/.well-known/oauth-protected-resource" + strings.TrimRight(u.Path, "/")
	u.RawPath, u.RawQuery, u.Fragment = "", "", ""
	return u.String()
}

// usable 은 켜 두었어도 조용히 꺼진 것처럼 동작해야 하는 경우를 가른다. 이유는
// 호출자가 로그에 남긴다.
func (o mcpOAuth) usable() (bool, string) {
	switch {
	case !o.Enabled:
		return false, "mcp.oauth.enabled is off"
	case o.Issuer == "":
		return false, "oidc.issuer_url is empty"
	case validateURL(o.resource()) != nil:
		return false, "resource identifier cannot be built: set mcp.oauth.resource or the public URL"
	}
	return true, ""
}

// acceptedAudiences 는 토큰의 aud 또는 azp 가운데 하나가 있어야 하는 목록이다.
// 리소스 식별자(Audience 매퍼를 둔 정식 경로), 관리자가 적은 허용 대상, 그리고
// 웹 로그인 클라이언트 ID(Keycloak 이 그 클라이언트에 발급한 토큰의 azp).
func (o mcpOAuth) acceptedAudiences() []string {
	accepted := []string{o.resource()}
	for _, a := range o.Audience {
		if a = strings.TrimSpace(a); a != "" && !slices.Contains(accepted, a) {
			accepted = append(accepted, a)
		}
	}
	if o.ClientID != "" && !slices.Contains(accepted, o.ClientID) {
		accepted = append(accepted, o.ClientID)
	}
	return accepted
}

// scopeCeiling 은 관리자가 적은 범위 가운데 이 앱이 아는 것만이다.
func (o mcpOAuth) scopeCeiling() []string {
	ceiling := []string{}
	for _, scope := range o.Scopes {
		if validScope(scope) && scope != "mcp:use" && !slices.Contains(ceiling, scope) {
			ceiling = append(ceiling, scope)
		}
	}
	return ceiling
}

// document 는 RFC 9728 보호 리소스 메타데이터다.
func (o mcpOAuth) document(serviceName string) map[string]any {
	return map[string]any{
		"resource":                 o.resource(),
		"authorization_servers":    []string{o.Issuer},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         o.scopeCeiling(),
		"resource_name":            serviceName + " MCP",
	}
}

// oauthRefusal 은 토큰을 거부한 이유를 두 갈래로 나눈다. reason·err 은 운영자가
// 로그에서 보고, message 는 클라이언트가 응답에서 본다. 어느 경로도 둘 중 하나를
// 비워 두지 않는다 — 거부 메시지만 받고 원인을 모르는 일이 없게.
type oauthRefusal struct {
	reason  string
	err     error
	message string
}

func refuse(reason string, err error, message string) *oauthRefusal {
	if err == nil {
		err = errors.New(reason)
	}
	return &oauthRefusal{reason: reason, err: err, message: message}
}

// looksLikeJWT 는 키가 아닌 값 가운데 토큰 검사로 보낼 것을 고르는 값싼 모양
// 검사다. 점 두 개, 세 조각 모두 비어 있지 않음.
func looksLikeJWT(token string) bool {
	if len(token) > 32768 {
		return false
	}
	parts := strings.Split(token, ".")
	return len(parts) == 3 && parts[0] != "" && parts[1] != "" && parts[2] != ""
}

// oauthProviders 는 issuer 별 discovery 를 캐시한다. discovery 는 Keycloak 으로의
// 왕복이고 그 뒤의 JWKS 가 모든 토큰을 검증하므로, 요청마다 하면 Keycloak 의
// 지연이 MCP 호출 하나하나의 앞에 선다. go-oidc 는 모르는 kid 를 만나면 키
// 집합을 다시 받아오므로 키 회전에 캐시를 비울 필요는 없다.
type oauthProviders struct {
	mu       sync.Mutex
	byIssuer map[string]*oidc.Provider
	// nextJWKSFetch 는 위조 토큰을 잇달아 내밀어 JWKS 를 매번 다시 받게 하는
	// 것을 초당 한 번으로 묶는다. 캐시된 키로 검증되는 요청은 기다리지 않는다.
	fetchMu       sync.Mutex
	nextJWKSFetch time.Time
}

// RoundTrip 은 discovery·JWKS 요청에 한계를 둔다: 리다이렉트 없음, 본문 1MiB,
// JWKS 재요청 초당 한 번.
func (p *oauthProviders) RoundTrip(r *http.Request) (*http.Response, error) {
	if !strings.HasSuffix(r.URL.Path, "/.well-known/openid-configuration") {
		p.fetchMu.Lock()
		delay := time.Until(p.nextJWKSFetch)
		if delay < 0 {
			delay = 0
		}
		p.nextJWKSFetch = time.Now().Add(delay + time.Second)
		p.fetchMu.Unlock()
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-r.Context().Done():
				return nil, r.Context().Err()
			case <-timer.C:
			}
		}
	}
	response, err := http.DefaultTransport.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	response.Body.Close()
	if err != nil {
		return nil, err
	}
	if len(raw) > 1<<20 {
		return nil, errors.New("oauth metadata too large")
	}
	response.Body = io.NopCloser(bytes.NewReader(raw))
	return response, nil
}

func (s *Server) oauthProvider(ctx context.Context, issuer string) (*oidc.Provider, error) {
	s.oauth.mu.Lock()
	defer s.oauth.mu.Unlock()
	if s.oauth.byIssuer == nil {
		s.oauth.byIssuer = map[string]*oidc.Provider{}
	}
	if provider := s.oauth.byIssuer[issuer]; provider != nil {
		return provider, nil
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: &s.oauth, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	// discovery 는 이 요청보다 오래 살아야 한다. provider 가 나중의 키 조회에 이
	// 컨텍스트를 계속 쓴다.
	provider, err := oidc.NewProvider(oidc.ClientContext(context.WithoutCancel(ctx), client), issuer)
	if err != nil {
		return nil, err
	}
	s.oauth.byIssuer[issuer] = provider
	return provider, nil
}

// oauthIdentity 는 검증을 통과한 토큰에서 계정을 찾는 데 쓰는 것만 남긴 것이다.
type oauthIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Audience      []string
	AZP           string
	Scopes        []string
}

var oauthSigningAlgs = []string{oidc.RS256, oidc.RS384, oidc.RS512, oidc.ES256, oidc.ES384, oidc.ES512, oidc.PS256, oidc.PS384, oidc.PS512}

// verifyMCPToken 은 토큰을 믿을지 정한다: 서명·iss·exp·nbf·typ·cnf·sub·대상.
// 계정은 보지 않는다 — 그건 DB 가 있어야 하는 다음 단계다.
func (s *Server) verifyMCPToken(ctx context.Context, o mcpOAuth, token string) (oauthIdentity, *oauthRefusal) {
	provider, err := s.oauthProvider(ctx, o.Issuer)
	if err != nil {
		return oauthIdentity{}, refuse("discovery failed", err, "Keycloak 발급자 정보를 읽지 못해 SSO 토큰을 확인할 수 없습니다. 잠시 후 다시 시도하거나 관리자에게 알리세요.")
	}
	// 대상은 아래에서 직접 본다. 라이브러리는 aud 하나만 비교하고 azp 를 모른다.
	verified, err := provider.Verifier(&oidc.Config{SkipClientIDCheck: true, SupportedSigningAlgs: oauthSigningAlgs}).Verify(ctx, token)
	if err != nil {
		return oauthIdentity{}, refuse("token rejected", err, "SSO 액세스 토큰이 유효하지 않습니다(서명·발급자·만료). 클라이언트에서 다시 로그인하세요.")
	}
	var claims struct {
		Typ               string          `json:"typ"`
		AZP               string          `json:"azp"`
		Scope             string          `json:"scope"`
		NotBefore         int64           `json:"nbf"`
		Confirmation      json.RawMessage `json:"cnf"`
		Email             string          `json:"email"`
		EmailVerified     bool            `json:"email_verified"`
		PreferredUsername string          `json:"preferred_username"`
	}
	if err := verified.Claims(&claims); err != nil {
		return oauthIdentity{}, refuse("claims unreadable", err, "SSO 토큰의 사용자 정보를 읽을 수 없습니다.")
	}
	// ID 토큰은 로그인 증거지 API 자격이 아니다. Keycloak 은 액세스 토큰에 Bearer,
	// ID 토큰에 ID, 리프레시 토큰에 Refresh/Offline 을 적는다.
	switch strings.ToLower(claims.Typ) {
	case "", "bearer", "jwt", "at+jwt":
	default:
		return oauthIdentity{}, refuse("token type not access", fmt.Errorf("typ=%q", claims.Typ), fmt.Sprintf("SSO 토큰의 종류(typ=%s)가 액세스 토큰이 아닙니다. ID 토큰이 아니라 액세스 토큰을 보내세요.", claims.Typ))
	}
	if claims.NotBefore > time.Now().Unix() {
		return oauthIdentity{}, refuse("token not yet valid", fmt.Errorf("nbf=%d", claims.NotBefore), "SSO 토큰이 아직 유효하지 않습니다(nbf). 서버와 Keycloak 의 시각을 확인하세요.")
	}
	// cnf 는 검증할 수 없는 소지자 증명(DPoP·mTLS)이 묶인 토큰이다.
	if cnf := bytes.TrimSpace(claims.Confirmation); len(cnf) > 0 && !bytes.Equal(cnf, []byte("null")) {
		return oauthIdentity{}, refuse("token bound to key (cnf)", nil, "SSO 토큰에 소지자 증명(cnf)이 묶여 있어 이 서버가 받을 수 없습니다. 일반 Bearer 토큰을 발급받으세요.")
	}
	subject := strings.TrimSpace(verified.Subject)
	if subject == "" {
		return oauthIdentity{}, refuse("subject missing", nil, "SSO 토큰에 사용자 식별 정보(sub)가 없습니다.")
	}
	// 이 토큰이 이 서버를 위한 것인가. 실제 Keycloak 26 은 액세스 토큰의 aud 에
	// account 만 싣고 클라이언트 ID 는 azp 에 담는다. 그래서 "aud 가 우리를
	// 가리키거나, 우리가 믿는 클라이언트(azp)에 발급된 것" 이면 된다.
	accepted := o.acceptedAudiences()
	bound := append(slices.Clone(verified.Audience), claims.AZP)
	if !slices.ContainsFunc(bound, func(v string) bool { return v != "" && slices.Contains(accepted, v) }) {
		return oauthIdentity{}, refuse("audience not accepted",
			fmt.Errorf("aud=%v azp=%q accepted=%v", verified.Audience, claims.AZP, accepted),
			fmt.Sprintf("SSO 토큰이 이 서버를 위해 발급된 것이 아닙니다(aud=%v, azp=%q). 관리자가 허용 대상(mcp.oauth.audience)에 %q 를 더하거나, Keycloak 클라이언트에 Audience 매퍼로 %q 를 넣어야 합니다.", verified.Audience, claims.AZP, claims.AZP, o.resource()))
	}
	return oauthIdentity{Subject: subject, Email: strings.TrimSpace(claims.Email), EmailVerified: claims.EmailVerified, Audience: verified.Audience, AZP: claims.AZP, Scopes: strings.Fields(claims.Scope)}, nil
}

// oauthEffectiveScopes 는 관리자 천장과 토큰 scope 의 교집합이다. 토큰이 이 앱의
// 범위 어휘를 싣지 않았으면(보통 openid profile email 뿐) 천장 그대로다.
// 결과가 비면 빈 목록을 돌려주지 않고 거부한다 — 소비자가 빈 목록을 어떻게
// 읽든 상관없이.
func oauthEffectiveScopes(o mcpOAuth, tokenScopes []string) (map[string]bool, *oauthRefusal) {
	ceiling := o.scopeCeiling()
	if len(ceiling) == 0 {
		return nil, refuse("scope ceiling empty", nil, "관리자가 SSO 주체에게 주는 범위(mcp.oauth.scopes)가 비어 있습니다. 관리자에게 알리세요.")
	}
	vocabulary := []string{}
	for _, scope := range tokenScopes {
		if validScope(scope) && scope != "mcp:use" {
			vocabulary = append(vocabulary, scope)
		}
	}
	effective := ceiling
	if len(vocabulary) > 0 {
		effective = []string{}
		for _, scope := range ceiling {
			if slices.Contains(vocabulary, scope) {
				effective = append(effective, scope)
			}
		}
		if len(effective) == 0 {
			return nil, refuse("token scope outside ceiling", fmt.Errorf("token=%v ceiling=%v", vocabulary, ceiling),
				fmt.Sprintf("SSO 토큰의 범위 %v 가 관리자 허용 범위 %v 와 겹치지 않습니다.", vocabulary, ceiling))
		}
	}
	// mcp:use 는 문 자체다. 토큰은 /mcp 에서만 받으므로 언제나 들어 있다.
	scopes := map[string]bool{"mcp:use": true}
	for _, scope := range effective {
		scopes[scope] = true
	}
	return scopes, nil
}

// oauthPrincipal 은 Bearer 액세스 토큰을 Orbit 사용자와 범위로 바꾸거나, 왜
// 안 되는지 정확히 말한다. 계정을 만들지 않고, 정지된 계정을 열지 않고, 토큰의
// role 을 보지 않는다.
func (s *Server) oauthPrincipal(ctx context.Context, o mcpOAuth, token string) (User, map[string]bool, *oauthRefusal) {
	if ok, why := o.usable(); !ok {
		return User{}, nil, refuse("sso tokens unavailable: "+why, nil, "이 서버는 SSO 액세스 토큰을 받지 않습니다. 개인 API 키(orb_)를 쓰거나, 관리자가 MCP SSO(OAuth) 인증을 켜야 합니다.")
	}
	identity, refusal := s.verifyMCPToken(ctx, o, token)
	if refusal != nil {
		return User{}, nil, refusal
	}
	scopes, refusal := oauthEffectiveScopes(o, identity.Scopes)
	if refusal != nil {
		return User{}, nil, refusal
	}
	// 웹 로그인이 계정을 찾는 규칙(subject, 아니면 확인된 이메일)과 같다. 등록하는
	// 절반만 없다.
	var u User
	err := s.store.DB.QueryRow(ctx, `SELECT id,username,email,display_name,role,status,last_login_at,created_at FROM users WHERE oidc_subject=$1 OR ($3 AND email<>'' AND lower(email)=lower($2)) ORDER BY (oidc_subject=$1) DESC LIMIT 1`, identity.Subject, identity.Email, identity.EmailVerified).Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Role, &u.Status, &u.LastLoginAt, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, nil, refuse("no account for subject", nil, "이 SSO 계정은 Orbit 에 등록되지 않았습니다. 먼저 웹으로 한 번 로그인하세요.")
	}
	if err != nil {
		return User{}, nil, refuse("account lookup failed", err, "사용자 정보를 확인하지 못했습니다. 잠시 후 다시 시도하세요.")
	}
	if u.Status != "active" {
		return User{}, nil, refuse("account not active", fmt.Errorf("status=%s", u.Status), "이 Orbit 계정은 비활성 상태입니다. 관리자에게 문의하세요.")
	}
	return u, scopes, nil
}

// wwwAuthenticate 는 401 을 초대장으로 바꾸는 헤더다. 클라이언트가
// resource_metadata 를 읽고 거기서 OAuth 흐름을 시작한다. MCP 경로에서만 붙인다.
func wwwAuthenticate(o mcpOAuth, refusal *oauthRefusal) string {
	if ok, _ := o.usable(); !ok {
		return ""
	}
	// 이유는 응답 본문(JSON)에 있다. 헤더에는 ASCII 만 둔다.
	header := fmt.Sprintf(`Bearer realm="Orbit", resource_metadata=%q`, o.metadataURL())
	if refusal != nil {
		header += `, error="invalid_token"`
	}
	return header
}

// protectedResourceMetadata 는 RFC 9728 문서다. 인증 없이, 제품 봉투가 아니라
// 맨 JSON 으로 — 읽는 쪽은 OAuth 클라이언트 라이브러리다.
func (s *Server) protectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	o, err := s.mcpOAuthConfig(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	if ok, why := o.usable(); !ok {
		if o.Enabled {
			slog.Warn("mcp oauth enabled but unusable", "reason", why)
		}
		writeError(w, http.StatusNotFound, "mcp_oauth_disabled", "이 서버의 MCP 는 SSO 토큰을 받지 않습니다. 개인 API 키(orb_)를 사용하세요.")
		return
	}
	serviceName := "Orbit"
	var general struct {
		ServiceName string `json:"service_name"`
	}
	if s.readSetting(r.Context(), "system", "general", &general, nil) == nil && general.ServiceName != "" {
		serviceName = general.ServiceName
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(o.document(serviceName))
}
