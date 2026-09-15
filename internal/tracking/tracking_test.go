package tracking

import (
	"strings"
	"testing"
	"time"
)

func momento(proxy bool) Settings {
	s := Defaults()
	s.Enabled = true
	s.Provider = ProviderMomento
	s.MomentoURL = "https://momento.corp.example/"
	s.MomentoSiteID = "SITE_ORBIT_001"
	s.MomentoProxy = proxy
	return s.Normalized()
}

func TestDefaultsAreOff(t *testing.T) {
	s := Defaults()
	if s.Enabled || s.Active("/orbit") || s.Snippet("n") != "" {
		t.Fatalf("defaults must not track anything: %+v", s)
	}
	scripts, connects, images := s.PolicySources()
	if len(scripts)+len(connects)+len(images) != 0 {
		t.Fatalf("defaults must add no policy sources: %v %v %v", scripts, connects, images)
	}
	if s.ProxyTarget() != nil {
		t.Fatal("defaults must not proxy")
	}
}

func TestMomentoProxyKeepsPolicyOnOrigin(t *testing.T) {
	s := momento(true)
	snippet := s.Snippet("abc")
	for _, want := range []string{`src="/momento/tracker.js"`, `data-endpoint="/momento"`, `data-site-id="SITE_ORBIT_001"`, `nonce="abc"`, `data-contract-version="1"`} {
		if !strings.Contains(snippet, want) {
			t.Fatalf("snippet lacks %s: %s", want, snippet)
		}
	}
	if strings.Contains(snippet, "momento.corp.example") {
		t.Fatalf("proxied snippet must not name the collector: %s", snippet)
	}
	scripts, connects, images := s.PolicySources()
	if len(scripts)+len(connects)+len(images) != 0 {
		t.Fatalf("proxied momento must add no external origin: %v %v %v", scripts, connects, images)
	}
	if target := s.ProxyTarget(); target == nil || target.Host != "momento.corp.example" {
		t.Fatalf("proxy target = %v", target)
	}
}

func TestMomentoDirectNamesCollector(t *testing.T) {
	s := momento(false)
	snippet := s.Snippet("abc")
	if !strings.Contains(snippet, `src="https://momento.corp.example/tracker.js"`) || strings.Contains(snippet, "data-endpoint") {
		t.Fatalf("direct snippet: %s", snippet)
	}
	scripts, connects, _ := s.PolicySources()
	if len(scripts) != 1 || scripts[0] != "https://momento.corp.example" || len(connects) != 1 {
		t.Fatalf("direct momento must allow the collector: %v %v", scripts, connects)
	}
	if s.ProxyTarget() != nil {
		t.Fatal("direct momento must not proxy")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Settings)
		ok   bool
	}{
		{"off needs nothing", func(s *Settings) { s.Enabled = false; s.MomentoURL = "" }, true},
		{"momento needs url", func(s *Settings) { s.MomentoURL = "" }, false},
		{"momento needs http url", func(s *Settings) { s.MomentoURL = "ftp://x" }, false},
		{"momento needs site", func(s *Settings) { s.MomentoSiteID = "" }, false},
		{"ga4 needs id", func(s *Settings) { s.Provider = ProviderGA4 }, false},
		{"ga4 ok", func(s *Settings) { s.Provider = ProviderGA4; s.MeasurementID = "G-1" }, true},
		{"matomo needs both", func(s *Settings) { s.Provider = ProviderMatomo; s.MatomoURL = "https://m.example" }, false},
		{"custom needs snippet", func(s *Settings) { s.Provider = ProviderCustom }, false},
		{"custom ok", func(s *Settings) { s.Provider = ProviderCustom; s.CustomSnippet = "<script></script>" }, true},
		{"unknown provider", func(s *Settings) { s.Provider = "pixel" }, false},
		{"too large even when off", func(s *Settings) { s.Enabled = false; s.CustomSnippet = strings.Repeat("a", MaxSnippetBytes+1) }, false},
		{"at limit", func(s *Settings) { s.Provider = ProviderCustom; s.CustomSnippet = strings.Repeat("a", MaxSnippetBytes) }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := momento(true)
			tc.edit(&s)
			if err := s.Validate(); (err == nil) != tc.ok {
				t.Fatalf("ok=%v err=%v", tc.ok, err)
			}
		})
	}
}

func TestActiveSkipsAdminUnlessAsked(t *testing.T) {
	s := momento(true)
	if !s.Active("/orbit") || !s.Active("/") || !s.Active("/administration") {
		t.Fatal("pages must be tracked")
	}
	if s.Active("/admin") || s.Active("/admin/users") {
		t.Fatal("admin must be skipped by default")
	}
	s.IncludeAdmin = true
	if !s.Active("/admin/users") {
		t.Fatal("admin must be tracked when asked")
	}
	s.Enabled = false
	if s.Active("/orbit") {
		t.Fatal("disabled must never track")
	}
}

func TestNormalizedFixesEnums(t *testing.T) {
	s := Settings{Provider: " Momento ", Placement: "FOOTER", MomentoURL: " https://m.example/ "}.Normalized()
	if s.Provider != ProviderMomento || s.Placement != "head" || s.MomentoURL != "https://m.example" {
		t.Fatalf("%+v", s)
	}
	if (Settings{Placement: "Body"}).Normalized().Placement != "body" {
		t.Fatal("body placement must survive")
	}
}

func TestWithNonceMarksEveryScriptTag(t *testing.T) {
	s := Settings{Provider: ProviderCustom, CustomSnippet: `<SCRIPT src="https://t.example/a.js"></SCRIPT><script nonce="keep">1</script><script>2</script>`}
	got := s.Snippet("n1")
	if strings.Count(got, `nonce="n1"`) != 2 || !strings.Contains(got, `nonce="keep"`) {
		t.Fatalf("nonce placement: %s", got)
	}
	if !strings.HasPrefix(got, `<SCRIPT nonce="n1" src=`) {
		t.Fatalf("nonce must sit inside the tag: %s", got)
	}
}

// 대소문자를 무시하려고 ToLower 로 얻은 인덱스를 원본에 쓰면, 접을 때 길이가
// 바뀌는 글자 뒤의 위치가 모두 어긋난다. 표준이 잡은 결함이다.
func TestWithNonceSurvivesLengthChangingRunes(t *testing.T) {
	for _, prefix := range []string{"İİİİ", "KK", "한글 İ K"} {
		s := Settings{Provider: ProviderCustom, CustomSnippet: prefix + `<script>1</script><Script src="https://t.example/x.js"></Script>`}
		got := s.Snippet("n")
		if !strings.Contains(got, prefix+`<script nonce="n">1</script><Script nonce="n" src=`) {
			t.Fatalf("prefix %q broke the tag: %s", prefix, got)
		}
	}
}

func TestInjectPlacesSnippetBeforeClosingTag(t *testing.T) {
	page := []byte("<!doctype html><html><HEAD><title>İ</title></HEAD><body><div></div></BODY></html>")
	head := string(Inject(page, "<s/>", "head"))
	if !strings.Contains(head, "<title>İ</title><s/>\n</HEAD>") {
		t.Fatalf("head: %s", head)
	}
	body := string(Inject(page, "<s/>", "body"))
	if !strings.Contains(body, "<div></div><s/>\n</BODY>") {
		t.Fatalf("body: %s", body)
	}
	bare := string(Inject([]byte("<p>x</p>"), "<s/>", "head"))
	if bare != "<p>x</p>\n<s/>\n" {
		t.Fatalf("bare: %q", bare)
	}
}

func TestSnippetOriginsReadsEveryAddress(t *testing.T) {
	snippet := "İ<script src='HTTPS://Cdn.Example/loader.js'></script><script>fetch(\"https://collect.example/v1/events\");new Image().src=\"https://cdn.example/px.gif\"+q;</script>"
	got := SnippetOrigins(snippet)
	if len(got) != 2 || got[0] != "https://cdn.example" || got[1] != "https://collect.example" {
		t.Fatalf("origins = %v", got)
	}
	if len(SnippetOrigins("<script>var http = 1; httpx</script>")) != 0 {
		t.Fatal("bare words must not become origins")
	}
}

func TestPolicySourcesIncludeAllowedHosts(t *testing.T) {
	s := Settings{Provider: ProviderCustom, CustomSnippet: `<script src="https://a.example/x.js"></script>`, AllowedHosts: "https://b.example, https://c.example\nhttps://a.example"}
	scripts, connects, images := s.PolicySources()
	want := []string{"https://a.example", "https://b.example", "https://c.example", "https://a.example"}
	if strings.Join(scripts, " ") != strings.Join(want, " ") || len(connects) != 4 || len(images) != 4 {
		t.Fatalf("scripts=%v connects=%v images=%v", scripts, connects, images)
	}
	ga := Settings{Provider: ProviderGA4, MeasurementID: "G-1"}
	scripts, _, _ = ga.PolicySources()
	if len(scripts) != 1 || scripts[0] != "https://www.googletagmanager.com" {
		t.Fatalf("ga4 scripts=%v", scripts)
	}
}

func TestAddAllowedHost(t *testing.T) {
	if got := AddAllowedHost("", "https://a.example/"); got != "https://a.example" {
		t.Fatalf("%q", got)
	}
	if got := AddAllowedHost("https://a.example", "HTTPS://A.EXAMPLE"); got != "https://a.example" {
		t.Fatalf("duplicate must be ignored: %q", got)
	}
	if got := AddAllowedHost("https://a.example", "https://b.example"); got != "https://a.example, https://b.example" {
		t.Fatalf("%q", got)
	}
}

func TestRecorderKeepsDistinctOrigins(t *testing.T) {
	r := NewRecorder()
	clock := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	r.now = func() time.Time { clock = clock.Add(time.Second); return clock }
	for i := 0; i < 3; i++ {
		r.Record("https://momento.corp.example/collect/v1/events", "connect-src https://x", "https://orbit.example/orbit")
	}
	r.Record("https://momento.corp.example/tracker.js", "script-src-elem", "https://orbit.example/")
	r.Record("chrome-extension://abc/x.js", "script-src", "")
	r.Record("data", "img-src", "")
	items := r.List(Settings{})
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Directive != "script-src-elem" || items[1].Directive != "connect-src" || items[1].Count != 3 {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Allowed || items[1].Allowed {
		t.Fatal("nothing is allowed by empty settings")
	}
	listed := r.List(Settings{AllowedHosts: "https://momento.corp.example"})
	if !listed[0].Allowed || !listed[1].Allowed {
		t.Fatalf("allow list must mark items: %+v", listed)
	}
	r.Record("https://region1.google-analytics.com/g/collect", "connect-src", "")
	ga := r.List(Settings{Provider: ProviderGA4, MeasurementID: "G-1"})
	if !ga[0].Allowed {
		t.Fatalf("wildcard must match: %+v", ga[0])
	}
	r.Forget()
	if len(r.List(Settings{})) != 0 {
		t.Fatal("forget must clear")
	}
}

func TestRecorderIsBounded(t *testing.T) {
	r := NewRecorder()
	for i := 0; i < MaxViolations+20; i++ {
		r.Record("https://h"+strings.Repeat("x", i%50)+".example"+strings.Repeat("y", i/50)+"/p", "connect-src", "")
	}
	if got := len(r.List(Settings{})); got != MaxViolations {
		t.Fatalf("recorder holds %d, want %d", got, MaxViolations)
	}
}
