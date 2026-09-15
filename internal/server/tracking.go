package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/hkjang/orbit/internal/id"
	"github.com/hkjang/orbit/internal/tracking"
)

// cspReportPath 는 브라우저가 정책이 거절한 요청을 신고하는 곳이다. 브라우저는
// 자격 증명 없이 보내므로 인증하지 않으며, 메모리의 작은 출처 목록 말고는
// 아무것도 저장하지 않는다.
const cspReportPath = "/api/v1/tracking/csp-report"

// maxReportBytes 는 인증 없는 끝점에 큰 본문을 밀어 넣지 못하게 한다.
const maxReportBytes = 8 * 1024

// apiPolicy 는 화면이 아닌 응답의 정책이다. JSON 과 스트림에는 스크립트도
// 스타일도 필요 없으므로 전부 막는다.
const apiPolicy = "default-src 'none'; frame-ancestors 'none'"

type cspReport struct {
	Report struct {
		BlockedURI         string `json:"blocked-uri"`
		ViolatedDirective  string `json:"violated-directive"`
		EffectiveDirective string `json:"effective-directive"`
		DocumentURI        string `json:"document-uri"`
	} `json:"csp-report"`
}

// isPagePath 는 이 경로가 사람이 보는 화면(또는 그 정적 자원)인지 말한다.
// API·MCP·헬스·프록시 경로는 화면이 아니므로 스니펫도 붙지 않고 정책도 좁다.
func isPagePath(path string) bool {
	if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, tracking.ProxyPath+"/") {
		return false
	}
	switch path {
	case "/mcp", "/healthz", "/readyz", "/openapi.json":
		return false
	}
	return true
}

// pagePolicy 는 화면의 좁은 정책에 켜진 추적 스니펫이 필요로 하는 것만 더한다.
// 인라인 코드는 요청마다 다른 nonce 로 허용하므로 'unsafe-inline' 은 어디에도
// 들어가지 않는다 — 한 번 풀면 추적을 끈 뒤에도 느슨한 채 남기 때문이다.
func pagePolicy(settings tracking.Settings, path, nonce string) string {
	scripts := []string{"'self'"}
	connects := []string{"'self'"}
	images := []string{"'self'", "data:", "https:"}
	active := settings.Active(path)
	if active {
		extraScripts, extraConnects, extraImages := settings.PolicySources()
		if nonce != "" {
			scripts = append(scripts, "'nonce-"+nonce+"'")
		}
		scripts = append(scripts, extraScripts...)
		connects = append(connects, extraConnects...)
		images = append(images, extraImages...)
	}
	policy := "default-src 'self'; script-src " + strings.Join(scripts, " ") +
		"; img-src " + strings.Join(images, " ") +
		"; style-src 'self' 'unsafe-inline'; connect-src " + strings.Join(connects, " ") +
		"; font-src 'self' data:; frame-ancestors 'none'"
	if active {
		// 추적이 켜진 동안에만 브라우저에게 무엇을 거절했는지 말해 달라고 한다.
		// 그 신고가 콘솔 오류를 한 번 누르면 되는 수정으로 바꾼다.
		policy += "; report-uri " + cspReportPath
	}
	return policy
}

// trackingSettings 는 추적 설정을 읽는다. 읽지 못하면 "추적 없음" 으로 다뤄
// 설정 저장소의 장애가 화면을 깨뜨리지 않게 한다.
func (s *Server) trackingSettings(ctx context.Context) tracking.Settings {
	settings := tracking.Defaults()
	if s.store == nil {
		return settings
	}
	if err := s.readSetting(ctx, "system", "tracking", &settings, nil); err != nil {
		return tracking.Defaults()
	}
	return settings.Normalized()
}

// servePage 는 단일 페이지 셸을 내보낸다. 정책 헤더와 넣은 스니펫이 같은
// nonce 를 쓰도록 여기서 한 번에 만든다.
func servePage(w http.ResponseWriter, r *http.Request, settings tracking.Settings, index []byte) {
	nonce := id.Token(16)
	w.Header().Set("Content-Security-Policy", pagePolicy(settings, r.URL.Path, nonce))
	if settings.Active(r.URL.Path) {
		if snippet := settings.Snippet(nonce); snippet != "" {
			index = tracking.Inject(index, snippet, settings.Placement)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(index)
}

// receiveCSPReport 는 브라우저가 거절한 것을 적는다. 잘못된 페이지가 우리에게서
// 오류를 보는 일이 없도록 언제나 204 로 답한다.
func (s *Server) receiveCSPReport(w http.ResponseWriter, r *http.Request) {
	defer w.WriteHeader(http.StatusNoContent)
	if s.violations == nil {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxReportBytes))
	if err != nil || len(body) == 0 {
		return
	}
	var report cspReport
	if json.Unmarshal(body, &report) != nil {
		return
	}
	directive := report.Report.EffectiveDirective
	if directive == "" {
		directive = report.Report.ViolatedDirective
	}
	s.violations.Record(report.Report.BlockedURI, directive, report.Report.DocumentURI)
}

// listTrackingViolations 는 정책이 막고 있는 주소를 관리자에게 보여 준다.
// 브라우저 콘솔을 읽지 않고도 스니펫을 고칠 수 있게 하는 것이다.
func (s *Server) listTrackingViolations(w http.ResponseWriter, r *http.Request) {
	items := []tracking.Violation{}
	if s.violations != nil {
		items = s.violations.List(s.trackingSettings(r.Context()))
	}
	writeJSON(w, 200, map[string]any{"violations": items})
}

// clearTrackingViolations 는 기록을 비운다. 고친 뒤 아직 막히는 것이 있는지
// 확인하는 방법이다.
func (s *Server) clearTrackingViolations(w http.ResponseWriter, r *http.Request) {
	if s.violations != nil {
		s.violations.Forget()
	}
	w.WriteHeader(http.StatusNoContent)
}

// allowTrackingHost 는 막힌 출처 하나를 허용 목록에 더한다. 위 목록의 "한 번
// 눌러 허용" 이다.
func (s *Server) allowTrackingHost(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	var in struct {
		Origin string `json:"origin"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	origin := strings.TrimSpace(in.Origin)
	if origin == "" || !strings.HasPrefix(strings.ToLower(origin), "http") {
		writeError(w, 400, "validation_error", "허용할 주소가 올바르지 않습니다.")
		return
	}
	settings := s.trackingSettings(r.Context())
	settings.AllowedHosts = tracking.AddAllowedHost(settings.AllowedHosts, origin)
	s.saveSetting(w, r, u, "system", "tracking", settings, "")
}

// momentoProxy 는 /momento/* 를 Momento 수집기로 넘긴다. 로더와 이벤트가 앱
// 오리진으로 오가므로 정책에 외부 출처가 등장하지 않는다. 추적이 꺼져 있거나
// 프록시를 쓰지 않으면 이 경로는 없는 것이다.
func (s *Server) momentoProxy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyMomento(w, r, s.trackingSettings(r.Context()).ProxyTarget())
	})
}

func proxyMomento(w http.ResponseWriter, r *http.Request, target *url.URL) {
	rest := strings.TrimPrefix(r.URL.Path, tracking.ProxyPath)
	// 수집기 콘솔까지 열어 주지 않는다 — 추적기 로더와 수집 끝점만 넘긴다.
	if target == nil || (rest != "/tracker.js" && !strings.HasPrefix(rest, "/collect/")) {
		http.NotFound(w, r)
		return
	}
	proxy := &httputil.ReverseProxy{Rewrite: func(pr *httputil.ProxyRequest) {
		pr.Out.URL.Path, pr.Out.URL.RawPath = rest, ""
		pr.SetURL(target)
		pr.SetXForwarded()
		// Orbit 세션은 수집기의 것이 아니다.
		pr.Out.Header.Del("Cookie")
		pr.Out.Header.Del("Authorization")
	}}
	proxy.ServeHTTP(w, r)
}
