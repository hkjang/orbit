package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/hkjang/orbit/internal/id"
	"github.com/hkjang/orbit/internal/secure"
	"github.com/hkjang/orbit/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// 진짜 Postgres 를 낀 종단 검사. ORBIT_TEST_DATABASE_URL 이 있을 때만 돈다:
//
//	docker run -d --rm -e POSTGRES_PASSWORD=orbit -e POSTGRES_USER=orbit -p 127.0.0.1:5432:5432 postgres:16-alpine
//	ORBIT_TEST_DATABASE_URL=postgres://orbit:orbit@127.0.0.1:5432/orbit go test ./internal/server/
//
// 검사마다 데이터베이스를 새로 만들어 마이그레이션·Bootstrap·server.New 를 그대로
// 지나므로, 라우팅·설정 행·SQL 이 프로덕션 배선과 같다.

type dbServer struct {
	t       *testing.T
	store   *store.Store
	handler http.Handler
	admin   string
}

func newDBServer(t *testing.T) *dbServer {
	t.Helper()
	dsn := os.Getenv("ORBIT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORBIT_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	var suffix [6]byte
	_, _ = rand.Read(suffix[:])
	name := "orbit_test_" + hex.EncodeToString(suffix[:])
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
		admin.Close()
	})
	testDSN, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	testDSN.Path = "/" + name
	vault, err := secure.NewVault([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, testDSN.String(), vault)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Bootstrap(ctx, "admin", "administrator-password"); err != nil {
		t.Fatal(err)
	}
	s := &dbServer{t: t, store: st, handler: New(st, "test", "test", "test")}
	if err := st.DB.QueryRow(ctx, `SELECT id FROM users WHERE username='admin'`).Scan(&s.admin); err != nil {
		t.Fatal(err)
	}
	return s
}

func (s *dbServer) setSetting(namespace, key string, value any) {
	s.t.Helper()
	raw, _ := json.Marshal(value)
	if _, err := s.store.DB.Exec(context.Background(), `UPDATE settings SET value=$3::jsonb WHERE namespace=$1 AND key=$2`, namespace, key, string(raw)); err != nil {
		s.t.Fatal(err)
	}
}

// useIDP 는 웹 로그인이 이미 구성된 설치에 MCP SSO 를 켠 상태다.
func (s *dbServer) useIDP(idp *fakeIDP, oauth MCPOAuthSettings) {
	s.t.Helper()
	s.setSetting("system", "general", map[string]any{"service_name": "Orbit", "public_url": "https://orbit.example.test", "session_hours": 12})
	s.setSetting("auth", "oidc", map[string]any{"enabled": true, "issuer_url": idp.server.URL, "client_id": "orbit-web", "display_name": "Keycloak SSO", "auto_provision": true, "default_role": "member"})
	s.setSetting("mcp", "oauth", oauth)
}

// createUser 는 웹으로 한 번 로그인해 등록된 사람이다(oidc_subject 가 묶여 있다).
func (s *dbServer) createUser(username, subject, status string) string {
	s.t.Helper()
	userID := id.New()
	if _, err := s.store.DB.Exec(context.Background(), `INSERT INTO users(id,username,email,display_name,role,status,oidc_subject) VALUES($1,$2,$3,$2,'member',$4,$5)`, userID, username, username+"@example.test", status, subject); err != nil {
		s.t.Fatal(err)
	}
	return userID
}

func (s *dbServer) userCount() int {
	s.t.Helper()
	var n int
	if err := s.store.DB.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}

func (s *dbServer) do(method, path, bearer, body string) *httptest.ResponseRecorder {
	s.t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = "orbit.example.test"
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	s.handler.ServeHTTP(w, r)
	return w
}

const listTools = `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`

func toolCall(name, args string) string {
	return `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `}}`
}

var readOnly = MCPOAuthSettings{Enabled: true, Audience: []string{}, Scopes: []string{"people:read", "memories:read"}}

// guards: protectedResourceMetadata, authenticate — 꺼져 있으면 아무것도 새지
// 않고, 켜면 메타데이터와 401 이 길을 가리킨다.
func TestDBRefusedMCPClientIsToldWhereToSignIn(t *testing.T) {
	s := newDBServer(t)
	idp := newIDP(t)

	// 새로 설치한 곳: 기본값은 꺼짐. 메타데이터는 404, 401 은 전과 같다.
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		if w := s.do(http.MethodGet, path, "", ""); w.Code != http.StatusNotFound {
			t.Fatalf("%s with SSO off: %d %s", path, w.Code, w.Body.String())
		}
	}
	refusal := s.do(http.MethodPost, "/mcp", "", listTools)
	if refusal.Code != http.StatusUnauthorized || refusal.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("no-bearer 401 with SSO off: %d %q", refusal.Code, refusal.Header().Get("WWW-Authenticate"))
	}
	token := idp.sign(t, accessToken(t, idp, testResource, nil))
	if w := s.do(http.MethodPost, "/mcp", token, listTools); w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("token with SSO off: %d %q %s", w.Code, w.Header().Get("WWW-Authenticate"), w.Body.String())
	}

	s.useIDP(idp, readOnly)
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		w := s.do(http.MethodGet, path, "", "")
		if w.Code != http.StatusOK || w.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		var doc struct {
			Resource             string   `json:"resource"`
			AuthorizationServers []string `json:"authorization_servers"`
			BearerMethods        []string `json:"bearer_methods_supported"`
			Scopes               []string `json:"scopes_supported"`
			Name                 string   `json:"resource_name"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
			t.Fatalf("%s: bare JSON expected: %s", path, w.Body.String())
		}
		if doc.Resource != testResource || len(doc.AuthorizationServers) != 1 || doc.AuthorizationServers[0] != idp.server.URL || strings.Join(doc.BearerMethods, "") != "header" || strings.Join(doc.Scopes, " ") != "people:read memories:read" || doc.Name != "Orbit MCP" {
			t.Errorf("%s: document %+v", path, doc)
		}
	}
	refusal = s.do(http.MethodPost, "/mcp", "", listTools)
	header := refusal.Header().Get("WWW-Authenticate")
	if refusal.Code != http.StatusUnauthorized || !strings.HasPrefix(header, `Bearer realm="Orbit", resource_metadata="https://orbit.example.test/.well-known/oauth-protected-resource/mcp"`) || strings.Contains(header, "invalid_token") {
		t.Fatalf("no-bearer 401: %d %q", refusal.Code, header)
	}
	// REST 401 에는 붙지 않는다.
	if w := s.do(http.MethodGet, "/api/v1/people/", "", ""); w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("REST 401: %d %q", w.Code, w.Header().Get("WWW-Authenticate"))
	}
}

// guards: oauthPrincipal, requestHasScope — 이 서버용 토큰은 등록된 계정을
// 열고, 권한은 관리자 천장을 넘지 않는다.
func TestDBKeycloakTokenOpensMCPForAKnownAccount(t *testing.T) {
	s := newDBServer(t)
	idp := newIDP(t)
	s.useIDP(idp, readOnly)
	s.createUser("ssomember", "subject-mcp", "active")

	token := idp.sign(t, accessToken(t, idp, testResource, nil))
	opened := s.do(http.MethodPost, "/mcp", token, listTools)
	if opened.Code != http.StatusOK || !strings.Contains(opened.Body.String(), "orbit_search_people") {
		t.Fatalf("tools/list: %d %s", opened.Code, opened.Body.String())
	}
	// 읽기 도구는 그 사용자의 데이터로 돈다.
	search := s.do(http.MethodPost, "/mcp", token, toolCall("orbit_search_people", `{"query":"x"}`))
	if search.Code != http.StatusOK || strings.Contains(search.Body.String(), "isError") {
		t.Fatalf("search: %d %s", search.Code, search.Body.String())
	}
	// 쓰기 도구는 천장 밖이다 — 토큰이 무엇을 주장하든.
	write := s.do(http.MethodPost, "/mcp", token, toolCall("orbit_create_memory", `{"title":"t","content":"c"}`))
	if write.Code != http.StatusOK || !strings.Contains(write.Body.String(), `"isError":true`) || !strings.Contains(write.Body.String(), "memories:write") {
		t.Fatalf("write must be refused by scope: %d %s", write.Code, write.Body.String())
	}
	var memories int
	_ = s.store.DB.QueryRow(context.Background(), `SELECT count(*) FROM memories`).Scan(&memories)
	if memories != 0 {
		t.Fatalf("a refused write still stored %d memories", memories)
	}
	// 유효한 토큰으로 REST 를 부르면 거부된다.
	if w := s.do(http.MethodGet, "/api/v1/people/", token, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("REST with token: %d %s", w.Code, w.Body.String())
	}
	if w := s.do(http.MethodGet, "/api/v1/me", token, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("/me with token: %d %s", w.Code, w.Body.String())
	}
}

// guards: oauthPrincipal — 계정은 만들지 않고, 정지된 계정은 열지 않는다.
func TestDBTokenNeverProvisionsOrRevivesAnAccount(t *testing.T) {
	s := newDBServer(t)
	idp := newIDP(t)
	s.useIDP(idp, readOnly)
	before := s.userCount()

	stranger := idp.sign(t, accessToken(t, idp, testResource, map[string]any{"sub": "nobody", "email": "nobody@example.test"}))
	w := s.do(http.MethodPost, "/mcp", stranger, listTools)
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "먼저 웹으로 한 번 로그인하세요") {
		t.Fatalf("unknown subject: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("WWW-Authenticate"), `error="invalid_token"`) {
		t.Errorf("refused token 401 lacks error=invalid_token: %q", w.Header().Get("WWW-Authenticate"))
	}
	if s.userCount() != before {
		t.Fatal("a token created an account")
	}

	s.createUser("disabled", "subject-disabled", "disabled")
	revived := idp.sign(t, accessToken(t, idp, testResource, map[string]any{"sub": "subject-disabled", "email": "disabled@example.test"}))
	if w := s.do(http.MethodPost, "/mcp", revived, listTools); w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "비활성") {
		t.Fatalf("disabled account: %d %s", w.Code, w.Body.String())
	}

	// 토큰의 role 은 권한이 아니다: 등록된 멤버는 admin claim 이 있어도 멤버다.
	s.createUser("member", "subject-member", "active")
	claimed := idp.sign(t, accessToken(t, idp, testResource, map[string]any{"sub": "subject-member", "realm_access": map[string]any{"roles": []string{"admin"}}}))
	if w := s.do(http.MethodPost, "/mcp", claimed, listTools); w.Code != http.StatusOK {
		t.Fatalf("member with role claim: %d %s", w.Code, w.Body.String())
	}
	var role string
	_ = s.store.DB.QueryRow(context.Background(), `SELECT role FROM users WHERE oidc_subject='subject-member'`).Scan(&role)
	if role != "member" {
		t.Fatalf("role changed to %s", role)
	}
}

// guards: verifyMCPToken — 다른 대상의 토큰은 거부되고 메시지가 고칠 값을 말한다;
// 관리자가 azp 를 적으면 매퍼 없이 통과한다.
func TestDBTokenForAnotherAppIsRefusedUntilAudienceIsListed(t *testing.T) {
	s := newDBServer(t)
	idp := newIDP(t)
	s.useIDP(idp, readOnly)
	s.createUser("ssomember", "subject-mcp", "active")

	// 실제 Keycloak 26 모양: aud=[account], 클라이언트는 azp 에.
	token := idp.sign(t, accessToken(t, idp, "account", map[string]any{"azp": "claude-mcp"}))
	w := s.do(http.MethodPost, "/mcp", token, listTools)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("other audience accepted: %d %s", w.Code, w.Body.String())
	}
	for _, want := range []string{"aud=[account]", `azp=\"claude-mcp\"`, testResource} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("refusal %s lacks %q", w.Body.String(), want)
		}
	}
	listed := readOnly
	listed.Audience = []string{"claude-mcp"}
	s.setSetting("mcp", "oauth", listed)
	if w := s.do(http.MethodPost, "/mcp", token, listTools); w.Code != http.StatusOK {
		t.Fatalf("azp listed but refused: %d %s", w.Code, w.Body.String())
	}
	// 만료·ID 토큰은 여전히 거부.
	for name, bad := range map[string]string{
		"expired":  idp.sign(t, accessToken(t, idp, testResource, map[string]any{"exp": time.Now().Add(-time.Minute).Unix()})),
		"ID token": idp.sign(t, accessToken(t, idp, testResource, map[string]any{"typ": "ID"})),
	} {
		if w := s.do(http.MethodPost, "/mcp", bad, listTools); w.Code != http.StatusUnauthorized {
			t.Errorf("%s accepted: %d", name, w.Code)
		}
	}
}

// guards: authenticate — 거부된 토큰마다 운영자용 로그 한 줄이 남고, 그 줄에는
// 검증 라이브러리의 원문 오류와 이 요청의 request_id 가 있다. 클라이언트가 본 401
// 과 서버 로그를 잇는 것은 그 id 뿐이다. 프로덕션 라우터(New, RequestID 미들웨어
// 포함)를 그대로 지난다.
func TestDBRefusedTokenIsLoggedWithCauseAndRequestID(t *testing.T) {
	s := newDBServer(t)
	idp := newIDP(t)
	other := newIDP(t)
	s.useIDP(idp, readOnly)
	s.createUser("ssomember", "subject-mcp", "active")
	logs := captureLogs(t)

	cases := map[string]struct {
		token  string
		reason string
		cause  string // 검증 라이브러리·검사 코드가 낸 원문
	}{
		"expired":      {idp.sign(t, accessToken(t, idp, testResource, map[string]any{"exp": time.Now().Add(-time.Minute).Unix()})), "token rejected", "oidc: token is expired"},
		"other issuer": {other.sign(t, accessToken(t, other, testResource, nil)), "token rejected", "failed to verify signature"},
		"issuer claim": {idp.sign(t, accessToken(t, other, testResource, nil)), "token rejected", "oidc: id token issued by a different provider"},
		"HS256":        {idp.signHS(t, accessToken(t, idp, testResource, nil)), "token rejected", `unexpected signature algorithm \"HS256\"`},
		"audience":     {idp.sign(t, accessToken(t, idp, "account", map[string]any{"azp": "claude-mcp"})), "audience not accepted", `aud=[account] azp=\"claude-mcp\"`},
		"ID token":     {idp.sign(t, accessToken(t, idp, testResource, map[string]any{"typ": "ID"})), "token type not access", `typ=\"ID\"`},
		"unknown user": {idp.sign(t, accessToken(t, idp, testResource, map[string]any{"sub": "nobody", "email": "nobody@example.test"})), "no account for subject", "no account for subject"},
	}
	for name, tc := range cases {
		logs.Reset()
		requestID := "req-" + strings.ReplaceAll(name, " ", "-")
		r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(listTools))
		r.Host = "orbit.example.test"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+tc.token)
		r.Header.Set(middleware.RequestIDHeader, requestID)
		w := httptest.NewRecorder()
		s.handler.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
			continue
		}
		line := logs.String()
		if strings.Count(line, "mcp oauth token refused") != 1 {
			t.Errorf("%s: want exactly one refusal line, got:\n%s", name, line)
			continue
		}
		for _, want := range []string{`msg="mcp oauth token refused"`, "reason=" + strconv.Quote(tc.reason), tc.cause, "request_id=" + requestID} {
			if !strings.Contains(line, want) {
				t.Errorf("%s: log line lacks %q:\n%s", name, want, line)
			}
		}
	}
	// 클라이언트가 id 를 보내지 않아도 미들웨어가 만든 id 가 붙는다.
	logs.Reset()
	if w := s.do(http.MethodPost, "/mcp", cases["expired"].token, listTools); w.Code != http.StatusUnauthorized {
		t.Fatalf("expired without X-Request-Id: %d", w.Code)
	}
	if id := regexp.MustCompile(`request_id=(\S+)`).FindStringSubmatch(logs.String()); len(id) != 2 || id[1] == "" || id[1] == `""` {
		t.Errorf("generated request_id missing from:\n%s", logs.String())
	}
	// 받아들인 토큰은 거부 줄을 남기지 않는다.
	logs.Reset()
	if w := s.do(http.MethodPost, "/mcp", idp.sign(t, accessToken(t, idp, testResource, nil)), listTools); w.Code != http.StatusOK {
		t.Fatalf("good token: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(logs.String(), "mcp oauth token refused") {
		t.Errorf("accepted token logged a refusal:\n%s", logs.String())
	}
}

// guards: New — 관리자가 리소스 식별자에 다른 경로를 적어도 401 이 가리키는
// 메타데이터 주소는 살아 있다.
func TestDBMetadataFollowsCustomResourcePath(t *testing.T) {
	s := newDBServer(t)
	idp := newIDP(t)
	custom := readOnly
	custom.Resource = "https://orbit.example.test/orbit/mcp"
	s.useIDP(idp, custom)

	refusal := s.do(http.MethodPost, "/mcp", "", listTools)
	header := refusal.Header().Get("WWW-Authenticate")
	const metadata = "https://orbit.example.test/.well-known/oauth-protected-resource/orbit/mcp"
	if !strings.Contains(header, `resource_metadata="`+metadata+`"`) {
		t.Fatalf("401 header: %q", header)
	}
	w := s.do(http.MethodGet, "/.well-known/oauth-protected-resource/orbit/mcp", "", "")
	var doc struct {
		Resource string `json:"resource"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &doc) != nil || doc.Resource != custom.Resource {
		t.Fatalf("metadata at the announced path: %d %s", w.Code, w.Body.String())
	}
}

// guards: authenticate — 키는 전과 똑같이 동작하고, 키도 토큰도 아닌 값은 새
// 말을 흘리지 않는다.
func TestDBAPIKeyStillWorksAlongsideSSO(t *testing.T) {
	s := newDBServer(t)
	idp := newIDP(t)
	s.useIDP(idp, readOnly)
	userID := s.createUser("keyuser", "subject-key", "active")
	key := "orb_" + id.Token(32)
	if _, err := s.store.DB.Exec(context.Background(), `INSERT INTO api_keys(id,user_id,name,prefix,secret_hash,scopes) VALUES($1,$2,'t',$3,$4,'["mcp:use","people:read"]'::jsonb)`, id.New(), userID, key[:12], secure.SHA256(key)); err != nil {
		t.Fatal(err)
	}
	if w := s.do(http.MethodPost, "/mcp", key, listTools); w.Code != http.StatusOK {
		t.Fatalf("key: %d %s", w.Code, w.Body.String())
	}
	if w := s.do(http.MethodGet, "/api/v1/people/", key, ""); w.Code != http.StatusOK {
		t.Fatalf("key on REST: %d %s", w.Code, w.Body.String())
	}
	// 키 모양이지만 없는 키: 토큰 검사로 흘러가지 않는다.
	w := s.do(http.MethodPost, "/mcp", "orb_"+id.Token(32), listTools)
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "로그인이 필요합니다.") || strings.Contains(w.Header().Get("WWW-Authenticate"), "invalid_token") {
		t.Fatalf("unknown key: %d %s %q", w.Code, w.Body.String(), w.Header().Get("WWW-Authenticate"))
	}
	// 키도 JWT 도 아닌 값: 같은 "로그인이 필요합니다".
	if w := s.do(http.MethodPost, "/mcp", "garbage", listTools); w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "로그인이 필요합니다.") {
		t.Fatalf("garbage bearer: %d %s", w.Code, w.Body.String())
	}
}

// guards: updateAdminSettings("mcp"), getAdminSettings — 관리 API 로 켜고 끄는
// 길과 켜는 조건.
func TestDBAdminSettingsRoundTrip(t *testing.T) {
	s := newDBServer(t)
	idp := newIDP(t)
	// 관리자 세션.
	login := s.do(http.MethodPost, "/api/v1/auth/login", "", `{"username":"admin","password":"administrator-password"}`)
	if login.Code != http.StatusOK {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	adminDo := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		s.handler.ServeHTTP(w, r)
		return w
	}
	// OIDC 가 없으면 켤 수 없다.
	w := adminDo(http.MethodPut, "/api/v1/admin/settings/mcp", `{"enabled":true,"resource":"","audience":["claude-mcp"],"scopes":["people:read"]}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Issuer") {
		t.Fatalf("enable without issuer: %d %s", w.Code, w.Body.String())
	}
	s.setSetting("auth", "oidc", map[string]any{"enabled": true, "issuer_url": idp.server.URL, "client_id": "orbit-web", "display_name": "Keycloak SSO", "auto_provision": true, "default_role": "member"})
	if w := adminDo(http.MethodPut, "/api/v1/admin/settings/mcp", `{"enabled":true,"resource":"","audience":["claude-mcp"],"scopes":["people:read"]}`); w.Code != http.StatusOK {
		t.Fatalf("enable: %d %s", w.Code, w.Body.String())
	}
	got := adminDo(http.MethodGet, "/api/v1/admin/settings", "")
	var settings struct {
		Settings struct {
			MCP struct {
				OAuth       MCPOAuthSettings `json:"oauth"`
				ResourceURL string           `json:"resource_url"`
				MetadataURL string           `json:"metadata_url"`
				Usable      bool             `json:"usable"`
			} `json:"mcp"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &settings); err != nil {
		t.Fatal(err)
	}
	m := settings.Settings.MCP
	if !m.OAuth.Enabled || strings.Join(m.OAuth.Audience, "") != "claude-mcp" || m.ResourceURL != "http://localhost:8080/mcp" || m.MetadataURL != "http://localhost:8080/.well-known/oauth-protected-resource/mcp" || !m.Usable {
		t.Fatalf("settings %+v", m)
	}
	// 공개 설정도 켜짐을 알린다(키 페이지 안내용).
	if w := s.do(http.MethodGet, "/api/v1/public/config", "", ""); !strings.Contains(w.Body.String(), `"mcp_oauth":{"enabled":true}`) {
		t.Fatalf("public config: %s", w.Body.String())
	}
	// 키로는 관리 API 에 닿지 않는다(전과 같음).
	if w := s.do(http.MethodPut, "/api/v1/admin/settings/mcp", "orb_"+id.Token(32), `{}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("key on admin: %d", w.Code)
	}
}
