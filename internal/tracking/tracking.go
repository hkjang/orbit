// Package tracking 은 관리자가 화면에서 붙이는 방문 추적 스니펫을 다룬다.
//
// Orbit 의 콘텐츠 보안 정책은 스크립트를 앱 오리진에서만 허용하므로 스니펫을
// 그냥 붙이면 브라우저가 조용히 막는다. 이 패키지는 그 답의 두 반쪽을 함께
// 만든다 — 페이지에 넣을 마크업과, 그 마크업이 필요로 하는 정책 출처. 인라인
// 코드는 요청마다 다른 nonce 로 허용하므로 정책 자체는 좁은 채로 남는다.
package tracking

import (
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"
)

const (
	ProviderNone    = "none"
	ProviderMomento = "momento"
	ProviderGA4     = "ga4"
	ProviderGTM     = "gtm"
	ProviderMatomo  = "matomo"
	ProviderCustom  = "custom"

	// MaxSnippetBytes 는 붙여 넣은 스니펫의 상한이다. 추적 로더는 몇백 바이트면
	// 충분하고, 그보다 훨씬 큰 것은 대개 잘못 붙인 것이다.
	MaxSnippetBytes = 8 * 1024

	// ProxyPath 는 Momento 수집기를 같은 오리진으로 넘겨 주는 경로다. 스니펫이
	// 이 경로로 로더를 받고 이벤트를 보내면 외부 출처가 정책에 등장하지 않는다.
	ProxyPath = "/momento"
)

// Settings 는 저장소의 system:tracking 행이다. 기본값은 꺼짐이며, 새로 설치한
// 곳에서는 아무것도 달라지지 않는다.
type Settings struct {
	Enabled       bool   `json:"enabled"`
	Provider      string `json:"provider"`
	MomentoURL    string `json:"momento_url"`
	MomentoSiteID string `json:"momento_site_id"`
	MomentoProxy  bool   `json:"momento_proxy"`
	MeasurementID string `json:"measurement_id"`
	MatomoURL     string `json:"matomo_url"`
	MatomoSiteID  string `json:"matomo_site_id"`
	CustomSnippet string `json:"custom_snippet"`
	AllowedHosts  string `json:"allowed_hosts"`
	IncludeAdmin  bool   `json:"include_admin"`
	Placement     string `json:"placement"`
}

// Defaults 는 설정 행이 없을 때와 새로 설치할 때 쓰는 값이다.
func Defaults() Settings {
	return Settings{Provider: ProviderNone, MomentoProxy: true, Placement: "head"}
}

// Normalized 는 저장하기 전에 다듬은 사본을 돌려준다. 공백을 걷고, 열거형은
// 소문자로 맞추며, 모르는 배치는 head 로 되돌린다.
func (s Settings) Normalized() Settings {
	s.Provider = strings.ToLower(strings.TrimSpace(s.Provider))
	if s.Provider == "" {
		s.Provider = ProviderNone
	}
	s.MomentoURL = strings.TrimRight(strings.TrimSpace(s.MomentoURL), "/")
	s.MomentoSiteID = strings.TrimSpace(s.MomentoSiteID)
	s.MeasurementID = strings.TrimSpace(s.MeasurementID)
	s.MatomoURL = strings.TrimRight(strings.TrimSpace(s.MatomoURL), "/")
	s.MatomoSiteID = strings.TrimSpace(s.MatomoSiteID)
	s.CustomSnippet = strings.TrimSpace(s.CustomSnippet)
	s.AllowedHosts = strings.TrimSpace(s.AllowedHosts)
	s.Placement = strings.ToLower(strings.TrimSpace(s.Placement))
	if s.Placement != "body" {
		s.Placement = "head"
	}
	return s
}

// Validate 는 고른 provider 에 무엇이 빠졌는지 말한다. 꺼져 있으면 무엇이든
// 저장할 수 있다 — 켜기 전에 값을 미리 적어 두는 것이 자연스럽다. 다만 스니펫
// 상한은 켜짐과 무관하게 지킨다.
func (s Settings) Validate() error {
	if len(s.CustomSnippet) > MaxSnippetBytes {
		return fmt.Errorf("추적 코드는 %d바이트를 넘을 수 없습니다.", MaxSnippetBytes)
	}
	switch s.Provider {
	case ProviderNone, ProviderMomento, ProviderGA4, ProviderGTM, ProviderMatomo, ProviderCustom:
	default:
		return errors.New("추적 provider 는 none, momento, ga4, gtm, matomo, custom 중 하나여야 합니다.")
	}
	if !s.Enabled {
		return nil
	}
	switch s.Provider {
	case ProviderMomento:
		if validOrigin(s.MomentoURL) != nil || s.MomentoSiteID == "" {
			return errors.New("Momento 수집기 주소와 사이트 ID를 확인해 주세요.")
		}
	case ProviderGA4, ProviderGTM:
		if s.MeasurementID == "" {
			return errors.New("측정 ID를 입력해 주세요.")
		}
	case ProviderMatomo:
		if validOrigin(s.MatomoURL) != nil || s.MatomoSiteID == "" {
			return errors.New("Matomo 주소와 사이트 ID를 확인해 주세요.")
		}
	case ProviderCustom:
		if s.CustomSnippet == "" {
			return errors.New("붙여 넣을 추적 코드가 비어 있습니다.")
		}
	}
	return nil
}

func validOrigin(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("HTTP(S) URL required")
	}
	return nil
}

// Active 는 이 경로의 화면에 스니펫을 붙일지 말한다. 관리 화면은 관리자가
// 따로 켜지 않는 한 제외한다 — 관리 콘솔의 트래픽은 누구도 보고 싶은 방문
// 데이터가 아니다.
func (s Settings) Active(path string) bool {
	if !s.Enabled || s.Provider == ProviderNone || s.Provider == "" {
		return false
	}
	if !s.IncludeAdmin && (path == "/admin" || strings.HasPrefix(path, "/admin/")) {
		return false
	}
	return strings.TrimSpace(s.Snippet("")) != ""
}

// UsesProxy 는 Momento 를 같은 오리진 프록시로 받을지 말한다.
func (s Settings) UsesProxy() bool {
	return s.Enabled && s.Provider == ProviderMomento && s.MomentoProxy && validOrigin(s.MomentoURL) == nil
}

// ProxyTarget 은 /momento/* 를 넘길 수집기 주소다. 프록시를 쓰지 않으면 nil.
func (s Settings) ProxyTarget() *url.URL {
	if !s.UsesProxy() {
		return nil
	}
	target, err := url.Parse(s.MomentoURL)
	if err != nil {
		return nil
	}
	return target
}

// Snippet 은 페이지에 넣을 마크업이다. 스니펫의 모든 script 태그에 nonce 를
// 달아 정책을 좁힌 채로 실행되게 한다.
func (s Settings) Snippet(nonce string) string {
	switch s.Provider {
	case ProviderMomento:
		site := html.EscapeString(strings.TrimSpace(s.MomentoSiteID))
		base := strings.TrimRight(strings.TrimSpace(s.MomentoURL), "/")
		if site == "" || base == "" {
			return ""
		}
		if s.MomentoProxy {
			return withNonce(fmt.Sprintf(`<script async src="%s/tracker.js" data-site-id="%s" data-environment="prd" data-contract-version="1" data-endpoint="%s"></script>`, ProxyPath, site, ProxyPath), nonce)
		}
		return withNonce(fmt.Sprintf(`<script async src="%s/tracker.js" data-site-id="%s" data-environment="prd" data-contract-version="1"></script>`, html.EscapeString(base), site), nonce)
	case ProviderGA4:
		id := html.EscapeString(strings.TrimSpace(s.MeasurementID))
		if id == "" {
			return ""
		}
		return withNonce(fmt.Sprintf(`<script async src="https://www.googletagmanager.com/gtag/js?id=%s"></script>
<script>window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}gtag('js',new Date());gtag('config','%s');</script>`, id, id), nonce)
	case ProviderGTM:
		id := html.EscapeString(strings.TrimSpace(s.MeasurementID))
		if id == "" {
			return ""
		}
		return withNonce(fmt.Sprintf(`<script>(function(w,d,s,l,i){w[l]=w[l]||[];w[l].push({'gtm.start':new Date().getTime(),event:'gtm.js'});var f=d.getElementsByTagName(s)[0],j=d.createElement(s),dl=l!='dataLayer'?'&l='+l:'';j.async=true;j.src='https://www.googletagmanager.com/gtm.js?id='+i+dl;f.parentNode.insertBefore(j,f);})(window,document,'script','dataLayer','%s');</script>`, id), nonce)
	case ProviderMatomo:
		base := strings.TrimRight(strings.TrimSpace(s.MatomoURL), "/")
		site := html.EscapeString(strings.TrimSpace(s.MatomoSiteID))
		if base == "" || site == "" {
			return ""
		}
		return withNonce(fmt.Sprintf(`<script>var _paq=window._paq=window._paq||[];_paq.push(['trackPageView']);_paq.push(['enableLinkTracking']);(function(){var u="%s/";_paq.push(['setTrackerUrl',u+'matomo.php']);_paq.push(['setSiteId','%s']);var d=document,g=d.createElement('script'),s=d.getElementsByTagName('script')[0];g.async=true;g.src=u+'matomo.js';s.parentNode.insertBefore(g,s);})();</script>`, html.EscapeString(base), site), nonce)
	case ProviderCustom:
		return withNonce(strings.TrimSpace(s.CustomSnippet), nonce)
	}
	return ""
}

// PolicySources 는 스니펫이 필요로 하는 추가 출처다. provider 가 정해진 것은
// 정책을 몰라도 되게 여기서 채우고, 붙여 넣은 스니펫은 안에 적힌 주소를 읽는다.
func (s Settings) PolicySources() (scripts, connects, images []string) {
	add := func(origin string) {
		scripts = append(scripts, origin)
		connects = append(connects, origin)
		images = append(images, origin)
	}
	switch s.Provider {
	case ProviderMomento:
		// 프록시를 쓰면 로더도 이벤트도 앱 오리진으로 가므로 더할 출처가 없다.
		if !s.MomentoProxy {
			if origin := originOf(s.MomentoURL); origin != "" {
				add(origin)
			}
		}
	case ProviderGA4, ProviderGTM:
		scripts = append(scripts, "https://www.googletagmanager.com")
		connects = append(connects, "https://www.google-analytics.com", "https://analytics.google.com", "https://*.google-analytics.com")
		images = append(images, "https://www.google-analytics.com", "https://www.googletagmanager.com")
	case ProviderMatomo:
		if origin := originOf(s.MatomoURL); origin != "" {
			add(origin)
		}
	case ProviderCustom:
		for _, origin := range SnippetOrigins(s.CustomSnippet) {
			add(origin)
		}
	}
	for _, host := range splitHosts(s.AllowedHosts) {
		add(host)
	}
	return scripts, connects, images
}

// SnippetOrigins 는 스니펫에 적힌 http(s) 출처를 모두 찾는다. 추적 도구는
// 로더 안에 자기 주소를 적어 두므로, 여기서 읽으면 관리자가 콘솔의 정책 오류를
// 호스트 이름으로 번역해 손으로 넣지 않아도 된다.
func SnippetOrigins(snippet string) []string {
	origins := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	for index := 0; index < len(snippet); {
		start := indexFold(snippet[index:], "http")
		if start < 0 {
			break
		}
		start += index
		end := start
		for end < len(snippet) && !isURLBoundary(snippet[end]) {
			end++
		}
		index = end
		origin := originOf(snippet[start:end])
		if origin == "" || !hasPrefixFold(origin, "http") {
			continue
		}
		if _, duplicate := seen[origin]; duplicate {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	return origins
}

// AddAllowedHost 는 허용 목록에 출처 하나를 더한다. 이미 있으면 그대로 둔다.
func AddAllowedHost(existing, origin string) string {
	origin = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(origin), "/"))
	if origin == "" {
		return existing
	}
	for _, host := range splitHosts(existing) {
		if strings.EqualFold(host, origin) {
			return existing
		}
	}
	if strings.TrimSpace(existing) == "" {
		return origin
	}
	return strings.TrimSpace(existing) + ", " + origin
}

func splitHosts(list string) []string {
	fields := strings.FieldsFunc(list, func(letter rune) bool {
		return letter == ',' || letter == ' ' || letter == '\n' || letter == '\r' || letter == '\t'
	})
	hosts := fields[:0]
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			hosts = append(hosts, trimmed)
		}
	}
	return hosts
}

// withNonce 는 아직 nonce 가 없는 모든 script 태그에 nonce 를 단다. 붙여 넣은
// 스니펫을 고치지 않고도 좁은 정책 아래서 실행되게 하는 장치다.
func withNonce(snippet, nonce string) string {
	if nonce == "" || snippet == "" {
		return snippet
	}
	var builder strings.Builder
	remaining := snippet
	for {
		index := indexFold(remaining, "<script")
		if index < 0 {
			builder.WriteString(remaining)
			return builder.String()
		}
		end := index + len("<script")
		builder.WriteString(remaining[:end])
		closing := strings.Index(remaining[end:], ">")
		tag := remaining[end:]
		if closing >= 0 {
			tag = remaining[end : end+closing]
		}
		if !containsFold(tag, "nonce=") {
			builder.WriteString(fmt.Sprintf(` nonce="%s"`, html.EscapeString(nonce)))
		}
		remaining = remaining[end:]
	}
}

// Inject 는 스니펫을 닫는 태그 바로 앞에 넣는다. 태그가 없으면 문서 끝에 붙인다.
func Inject(page []byte, snippet, placement string) []byte {
	marker := "</head>"
	if placement == "body" {
		marker = "</body>"
	}
	text := string(page)
	index := lastIndexFold(text, marker)
	if index < 0 {
		return []byte(text + "\n" + snippet + "\n")
	}
	return []byte(text[:index] + snippet + "\n" + text[index:])
}

// lastIndexFold 는 indexFold 와 같되 마지막 것을 찾는다.
func lastIndexFold(s, sub string) int {
	for i := len(s) - len(sub); i >= 0; i-- {
		if indexFold(s[i:i+len(sub)], sub) == 0 {
			return i
		}
	}
	return -1
}

// indexFold 는 ASCII 대소문자만 무시하고 sub 를 찾아 s 안의 인덱스를 돌려준다.
//
// strings.ToLower 로 접은 사본에서 얻은 인덱스를 원본에 쓰면 안 된다. 접을 때
// 바이트 길이가 바뀌는 글자가 있다 — U+212A KELVIN SIGN 은 3바이트가 1바이트
// 'k' 로, U+0130 'İ' 는 2바이트가 3바이트로 — 그런 글자가 하나만 있어도 뒤
// 위치가 전부 어긋나 nonce 가 태그 이름 한가운데 박힌다. 찾는 문자열은 모두
// ASCII 이므로 ASCII 만 접으면 모든 바이트가 제자리에 있다.
func indexFold(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if foldASCII(s[i+j]) != foldASCII(sub[j]) {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func containsFold(s, sub string) bool { return indexFold(s, sub) >= 0 }

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && indexFold(s[:len(prefix)], prefix) == 0
}

func foldASCII(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// isURLBoundary 는 HTML 이나 자바스크립트 안에 적힌 URL 이 끝나는 글자다.
func isURLBoundary(letter byte) bool {
	switch letter {
	case '"', '\'', '`', '<', '>', ' ', '\t', '\n', '\r', ')', ',', ';', '\\', '+':
		return true
	}
	return false
}

func originOf(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "" {
		scheme = "https"
	}
	// 호스트는 대소문자를 가리지 않으므로 소문자로 맞춰 같은 출처가 두 번
	// 나오지 않게 한다.
	return scheme + "://" + strings.ToLower(parsed.Host)
}
