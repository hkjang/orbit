package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/hkjang/orbit/internal/tracking"
)

func momentoSettings(proxy bool) tracking.Settings {
	s := tracking.Defaults()
	s.Enabled = true
	s.Provider = tracking.ProviderMomento
	s.MomentoURL = "https://momento.corp.example"
	s.MomentoSiteID = "SITE_ORBIT_001"
	s.MomentoProxy = proxy
	return s
}

func TestPagePolicyStaysStrictWhenOff(t *testing.T) {
	policy := pagePolicy(tracking.Defaults(), "/orbit", "n")
	for _, forbidden := range []string{"nonce", "report-uri", "momento"} {
		if strings.Contains(policy, forbidden) {
			t.Fatalf("policy must not contain %q when tracking is off: %s", forbidden, policy)
		}
	}
	if directive(policy, "script-src") != "'self'" || directive(policy, "connect-src") != "'self'" {
		t.Fatalf("policy must stay on origin: %s", policy)
	}
}

// directive 는 정책에서 지시어 하나의 값을 꺼낸다.
func directive(policy, name string) string {
	for _, part := range strings.Split(policy, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, name+" ") {
			return strings.TrimPrefix(part, name+" ")
		}
	}
	return ""
}

func TestPagePolicyAddsOnlyWhatTheSnippetNeeds(t *testing.T) {
	proxied := pagePolicy(momentoSettings(true), "/orbit", "abc")
	if !strings.Contains(proxied, "script-src 'self' 'nonce-abc';") || !strings.Contains(proxied, "connect-src 'self';") {
		t.Fatalf("proxied momento needs only the nonce: %s", proxied)
	}
	if !strings.HasSuffix(proxied, "; report-uri "+cspReportPath) {
		t.Fatalf("active policy must ask for reports: %s", proxied)
	}
	direct := pagePolicy(momentoSettings(false), "/orbit", "abc")
	if !strings.Contains(direct, "script-src 'self' 'nonce-abc' https://momento.corp.example;") || !strings.Contains(direct, "connect-src 'self' https://momento.corp.example;") {
		t.Fatalf("direct momento must allow the collector: %s", direct)
	}
	custom := tracking.Settings{Enabled: true, Provider: tracking.ProviderCustom, CustomSnippet: `<script src="https://t.example/x.js"></script>`, AllowedHosts: "https://px.example"}
	policy := pagePolicy(custom, "/", "abc")
	if !strings.Contains(policy, "script-src 'self' 'nonce-abc' https://t.example https://px.example;") {
		t.Fatalf("custom snippet origins must be allowed: %s", policy)
	}
	for _, active := range []string{proxied, direct, policy} {
		if strings.Contains(directive(active, "script-src"), "unsafe-inline") {
			t.Fatalf("'unsafe-inline' must never reach script-src: %s", active)
		}
	}
	// 관리 화면은 include_admin 이 꺼져 있으면 원래 정책 그대로다.
	if admin := pagePolicy(momentoSettings(true), "/admin", "abc"); admin != pagePolicy(tracking.Defaults(), "/admin", "abc") {
		t.Fatalf("admin must keep the strict policy: %s", admin)
	}
}

var nonceInHeader = regexp.MustCompile(`'nonce-([0-9a-f]+)'`)

func TestServePageInjectsSnippetWithTheHeaderNonce(t *testing.T) {
	index := []byte("<!doctype html><html><head><title>Orbit</title></head><body><div id=\"root\"></div></body></html>")
	for _, placement := range []string{"head", "body"} {
		settings := momentoSettings(true)
		settings.Placement = placement
		recorder := httptest.NewRecorder()
		servePage(recorder, httptest.NewRequest(http.MethodGet, "/orbit", nil), settings, index)
		match := nonceInHeader.FindStringSubmatch(recorder.Header().Get("Content-Security-Policy"))
		if match == nil {
			t.Fatalf("no nonce in policy: %s", recorder.Header().Get("Content-Security-Policy"))
		}
		body := recorder.Body.String()
		if !strings.Contains(body, `<script nonce="`+match[1]+`" async src="/momento/tracker.js"`) {
			t.Fatalf("snippet must carry the header nonce: %s", body)
		}
		marker := "</head>"
		if placement == "body" {
			marker = "</body>"
		}
		if !strings.Contains(body, `data-endpoint="/momento"></script>`+"\n"+marker) {
			t.Fatalf("snippet is not before %s: %s", marker, body)
		}
	}
	// 꺼져 있으면 페이지는 그대로 나간다.
	recorder := httptest.NewRecorder()
	servePage(recorder, httptest.NewRequest(http.MethodGet, "/orbit", nil), tracking.Defaults(), index)
	if recorder.Body.String() != string(index) {
		t.Fatalf("page changed while tracking is off: %s", recorder.Body.String())
	}
	// 관리 화면은 include_admin 이 꺼져 있으면 붙지 않는다.
	recorder = httptest.NewRecorder()
	servePage(recorder, httptest.NewRequest(http.MethodGet, "/admin", nil), momentoSettings(true), index)
	if recorder.Body.String() != string(index) {
		t.Fatalf("admin page changed without include_admin: %s", recorder.Body.String())
	}
}

func TestSecurityHeadersNarrowNonPagePaths(t *testing.T) {
	handler := New(nil, "test", "abc", "now")
	cases := map[string]string{
		"/healthz":               apiPolicy,
		"/api/v1/me":             apiPolicy,
		"/momento/tracker.js":    apiPolicy,
		"/":                      pagePolicy(tracking.Defaults(), "/", ""),
		"/assets/missing.js":     pagePolicy(tracking.Defaults(), "/assets/missing.js", ""),
		"/momento/admin/console": apiPolicy,
	}
	for path, want := range cases {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if got := recorder.Header().Get("Content-Security-Policy"); got != want {
			t.Errorf("%s: policy %q want %q", path, got, want)
		}
		if strings.HasPrefix(path, "/momento/") && recorder.Code != http.StatusNotFound {
			t.Errorf("%s: proxy must be absent while tracking is off, got %d", path, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/orbit", nil))
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "tracker.js") {
		t.Fatalf("fresh install must serve the page untouched: %d %s", recorder.Code, recorder.Body.String())
	}
	// 신고 끝점은 브라우저가 자격 증명 없이 부르므로 인증 뒤에 있으면 안 된다.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, cspReportPath, strings.NewReader(`{"csp-report":{"blocked-uri":"https://x.example/a","effective-directive":"script-src-elem"}}`)))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("report endpoint must accept anonymous reports: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestReceiveCSPReportRecordsOriginAndDirective(t *testing.T) {
	s := &Server{violations: tracking.NewRecorder()}
	body := `{"csp-report":{"blocked-uri":"https://momento.corp.example/collect/v1/events","effective-directive":"connect-src","document-uri":"https://orbit.example/orbit"}}`
	for i := 0; i < 2; i++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, cspReportPath, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/csp-report")
		s.receiveCSPReport(recorder, request)
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("status %d", recorder.Code)
		}
	}
	items := s.violations.List(tracking.Settings{})
	if len(items) != 1 || items[0].Origin != "https://momento.corp.example" || items[0].Directive != "connect-src" || items[0].Count != 2 {
		t.Fatalf("items = %+v", items)
	}
	broken := httptest.NewRecorder()
	s.receiveCSPReport(broken, httptest.NewRequest(http.MethodPost, cspReportPath, strings.NewReader("not json")))
	if broken.Code != http.StatusNoContent || len(s.violations.List(tracking.Settings{})) != 1 {
		t.Fatalf("broken report must be ignored quietly: %d", broken.Code)
	}
}

func TestMomentoProxyForwardsTrackerAndCollectOnly(t *testing.T) {
	var seenPath, seenCookie string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath, seenCookie = r.URL.Path, r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = io.WriteString(w, "tracker")
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL + "/base")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/momento/tracker.js", nil)
	request.Header.Set("Cookie", "orbit_session=secret")
	proxyMomento(recorder, request, target)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "tracker" || seenPath != "/base/tracker.js" {
		t.Fatalf("tracker: %d %q path=%q", recorder.Code, recorder.Body.String(), seenPath)
	}
	if seenCookie != "" {
		t.Fatalf("session cookie leaked to the collector: %q", seenCookie)
	}

	recorder = httptest.NewRecorder()
	proxyMomento(recorder, httptest.NewRequest(http.MethodPost, "/momento/collect/v1/events", strings.NewReader("{}")), target)
	if recorder.Code != http.StatusOK || seenPath != "/base/collect/v1/events" {
		t.Fatalf("collect: %d path=%q", recorder.Code, seenPath)
	}

	seenPath = ""
	recorder = httptest.NewRecorder()
	proxyMomento(recorder, httptest.NewRequest(http.MethodGet, "/momento/admin/sites", nil), target)
	if recorder.Code != http.StatusNotFound || seenPath != "" {
		t.Fatalf("console path must not be proxied: %d path=%q", recorder.Code, seenPath)
	}
	recorder = httptest.NewRecorder()
	proxyMomento(recorder, httptest.NewRequest(http.MethodGet, "/momento/tracker.js", nil), nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("no target must mean no proxy: %d", recorder.Code)
	}
}
