package server

import (
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hkjang/orbit/internal/store"
)

func TestParseOriginKeepsOnlySchemeAndHost(t *testing.T) {
	good := map[string]string{
		"https://ptium.intra":         "https://ptium.intra",
		"  HTTPS://Muni.Intra:8443/ ": "https://muni.intra:8443",
		"http://weekly.intra/":        "http://weekly.intra",
	}
	for raw, want := range good {
		got, err := parseOrigin(raw)
		if err != nil || got != want {
			t.Errorf("parseOrigin(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	bad := []string{
		"", "ptium.intra", "ftp://ptium.intra", "https://ptium.intra/handoff",
		"https://ptium.intra?x=1", "https://ptium.intra#frag", "https://user:pw@ptium.intra",
		"javascript:alert(1)", "https://",
	}
	for _, raw := range bad {
		if got, err := parseOrigin(raw); err == nil {
			t.Errorf("parseOrigin(%q) accepted as %q", raw, got)
		}
	}
}

func TestValidateHandoffSettings(t *testing.T) {
	ok := HandoffSettings{Targets: []HandoffTarget{
		{Name: " Ptium ", Origin: "https://Ptium.intra/", Formats: []string{"markdown", "docx"}},
		{Name: "Weekly", Origin: "https://weekly.intra", Formats: []string{"pptx"}},
	}}
	if err := validateHandoffSettings(&ok); err != nil {
		t.Fatalf("valid settings rejected: %v", err)
	}
	// 저장되는 값은 다듬어진 것이어야 받는 쪽의 표기와 글자 그대로 맞는다.
	if ok.Targets[0].Name != "Ptium" || ok.Targets[0].Origin != "https://ptium.intra" {
		t.Fatalf("settings were not normalised: %+v", ok.Targets[0])
	}

	var empty HandoffSettings
	if err := validateHandoffSettings(&empty); err != nil || empty.Targets == nil || len(empty.Targets) != 0 {
		t.Fatalf("empty list must be valid and become []: %v %+v", err, empty)
	}

	bad := []HandoffSettings{
		{Targets: []HandoffTarget{{Name: "", Origin: "https://a.intra", Formats: []string{"markdown"}}}},
		{Targets: []HandoffTarget{{Name: "A", Origin: "https://a.intra/path", Formats: []string{"markdown"}}}},
		{Targets: []HandoffTarget{{Name: "A", Origin: "https://a.intra", Formats: nil}}},
		{Targets: []HandoffTarget{{Name: "A", Origin: "https://a.intra", Formats: []string{"pdf"}}}},
		{Targets: []HandoffTarget{
			{Name: "A", Origin: "https://a.intra", Formats: []string{"markdown"}},
			{Name: "B", Origin: "HTTPS://A.INTRA/", Formats: []string{"markdown"}},
		}},
	}
	for i, v := range bad {
		if err := validateHandoffSettings(&v); err == nil {
			t.Errorf("case %d accepted: %+v", i, v)
		}
	}
	many := HandoffSettings{}
	for i := 0; i <= maxHandoffTargets; i++ {
		many.Targets = append(many.Targets, HandoffTarget{Name: "S", Origin: "https://s" + strings.Repeat("x", i) + ".intra", Formats: []string{"markdown"}})
	}
	if err := validateHandoffSettings(&many); err == nil {
		t.Error("list above the cap accepted")
	}
}

func TestHandoffTargetsForShowsOnlyMarkdownReceivers(t *testing.T) {
	cfg := HandoffSettings{Targets: []HandoffTarget{
		{Name: "Ptium", Origin: "https://ptium.intra", Formats: []string{"markdown", "docx"}},
		{Name: "Kanpic", Origin: "https://kanpic.intra", Formats: []string{"csv", "xlsx"}},
		{Name: "Broken", Origin: "not-an-origin", Formats: []string{"markdown"}},
	}}
	got := handoffTargetsFor(cfg)
	if len(got) != 1 || got[0].Name != "Ptium" || got[0].Origin != "https://ptium.intra" {
		t.Fatalf("got %+v", got)
	}
	if got := handoffTargetsFor(HandoffSettings{}); len(got) != 0 {
		t.Fatalf("empty list must yield no targets, got %+v", got)
	}
}

func TestRenderMemoryMarkdown(t *testing.T) {
	when := time.Date(2026, 9, 12, 7, 0, 0, 0, time.UTC)
	m := Memory{
		ID: "x", Title: "첫 만남\n그리고", PersonName: "김하늘", Content: "  카페에서 두 시간.\n\n창업 이야기.  ",
		OccurredAt: &when, Topics: []string{"창업", "커피"}, Status: "approved",
		CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	got := renderMemoryMarkdown(m)
	want := "# 첫 만남 그리고\n\n- 함께한 사람: 김하늘\n- 언제: 2026-09-12\n- 주제: 창업, 커피\n- 출처: Orbit\n\n카페에서 두 시간.\n\n창업 이야기.\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	// 사람·시각·주제가 없으면 그 줄이 빠지고 만든 날이 '언제'가 된다.
	bare := renderMemoryMarkdown(Memory{Title: "혼자", Content: "메모", CreatedAt: when})
	if strings.Contains(bare, "함께한 사람") || strings.Contains(bare, "주제") || !strings.Contains(bare, "- 언제: 2026-09-12\n") {
		t.Fatalf("got\n%s", bare)
	}
	// id 나 상태 같은 살림살이는 문서에 없다.
	if strings.Contains(got, "approved") || strings.Contains(got, m.ID+"\n") {
		t.Fatalf("housekeeping leaked into the document:\n%s", got)
	}
}

func TestHandoffFilename(t *testing.T) {
	cases := map[string]string{
		"2026년 3분기 개편안":         "2026년 3분기 개편안.md",
		"a/b\\c:d*e?f\"g<h>i|j": "abcdefghij.md",
		"  ...  ":               "orbit-memory.md",
		"":                      "orbit-memory.md",
		"줄\n바꿈":                 "줄 바꿈.md",
	}
	for title, want := range cases {
		if got := handoffFilename(title); got != want {
			t.Errorf("handoffFilename(%q) = %q, want %q", title, got, want)
		}
	}
	long := handoffFilename(strings.Repeat("가", 100))
	stem := strings.TrimSuffix(long, ".md")
	if len(stem) > maxHandoffStemBytes || !strings.HasSuffix(long, ".md") || strings.ContainsRune(stem, '�') {
		t.Fatalf("long title not cut on a rune boundary: %q", long)
	}
	for _, r := range stem {
		if r != '가' {
			t.Fatalf("cut broke a rune: %q", long)
		}
	}
}

func TestAttachmentDisposition(t *testing.T) {
	got := attachmentDisposition("첫 만남.md")
	if !strings.HasPrefix(got, `attachment; filename="`) || !strings.Contains(got, `filename*=UTF-8''%EC%B2%AB%20%EB%A7%8C%EB%82%A8.md`) {
		t.Fatalf("got %q", got)
	}
	// 한글만 있는 이름은 ASCII 대체 이름이 비지 않는다.
	if !strings.Contains(got, `filename="orbit-memory.md"`) {
		t.Fatalf("no ascii fallback: %q", got)
	}
	if got := attachmentDisposition(`we"ird.md`); strings.Contains(got, `"we"ird`) {
		t.Fatalf("quote leaked into ascii name: %q", got)
	}
	// 보내는 쪽에서 만든 헤더를 받는 쪽 파서로 되돌려 본다. '=', '@', ';' 같은
	// 글자는 RFC 5987 attr-char 가 아니라서 그대로 두면 파서가 헤더 전체를 거부한다.
	for _, name := range []string{
		"a=b @c.md",
		"회의; 정리=요약 @팀:공유.md",
		"first meeting.md",
		"a+b&c$d!e#f.md",
	} {
		got := attachmentDisposition(name)
		disp, params, err := mime.ParseMediaType(got)
		if err != nil {
			t.Fatalf("attachmentDisposition(%q) = %q: not parseable: %v", name, got, err)
		}
		if disp != "attachment" {
			t.Fatalf("attachmentDisposition(%q): disposition %q", name, disp)
		}
		if params["filename"] != name {
			t.Fatalf("attachmentDisposition(%q) round-tripped to %q (header %q)", name, params["filename"], got)
		}
	}
}

func TestLoggedPathHidesClaims(t *testing.T) {
	if got := loggedPath("/api/v1/handoff/claims/abcdef0123"); got != "/api/v1/handoff/claims/{claim}" {
		t.Fatalf("got %q", got)
	}
	for _, p := range []string{"/api/v1/handoff/claims", "/api/v1/handoff/claims/", "/api/v1/memories/", "/healthz"} {
		if got := loggedPath(p); got != p {
			t.Errorf("loggedPath(%q) = %q", p, got)
		}
	}
}

func TestHandoffScopeIsMemoriesRead(t *testing.T) {
	if got := requiredScope(http.MethodPost, "/api/v1/handoff/claims"); got != "memories:read" {
		t.Fatalf("POST claims: got %q", got)
	}
	if got := requiredScope(http.MethodGet, "/api/v1/handoff/targets"); got != "memories:read" {
		t.Fatalf("GET targets: got %q", got)
	}
}

func TestLooksLikeUUID(t *testing.T) {
	if !looksLikeUUID("f7e62238-bad1-4c73-ad45-a46777cc20ef") {
		t.Fatal("uuid rejected")
	}
	for _, v := range []string{"", "f7e62238bad14c73ad45a46777cc20ef", "f7e62238-bad1-4c73-ad45-a46777cc20e", "f7e62238-bad1-4c73-ad45-a46777cc20eg", "'; DROP TABLE memories; --"} {
		if looksLikeUUID(v) {
			t.Errorf("%q accepted", v)
		}
	}
}

func TestHandoffClaimTTLWithinStandard(t *testing.T) {
	if handoffClaimTTL > 5*time.Minute {
		t.Fatalf("claim TTL %v exceeds the standard's five minutes", handoffClaimTTL)
	}
}

// 표를 내주는 경로는 로그인 뒤가 아니라 그 앞에 있어야 한다: 표가 곧 자격이다.
// 표를 발급하는 경로는 로그인 뒤에 있어야 한다.
func TestHandoffRoutes(t *testing.T) {
	router := New(&store.Store{}, "test", "", "").(chi.Router)
	cases := []struct{ method, path, want string }{
		{http.MethodGet, "/api/v1/handoff/claims/abc123", "/api/v1/handoff/claims/{claim}"},
		{http.MethodPost, "/api/v1/handoff/claims", "/api/v1/handoff/claims"},
		{http.MethodGet, "/api/v1/handoff/targets", "/api/v1/handoff/targets"},
	}
	for _, tc := range cases {
		rctx := chi.NewRouteContext()
		if got := router.Find(rctx, tc.method, tc.path); got != tc.want {
			t.Errorf("%s %s routed to %q, want %q", tc.method, tc.path, got, tc.want)
		}
	}
	// 로그인 없이 표를 발급하거나 목록을 읽을 수는 없다.
	for _, tc := range cases[1:] {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`)))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without credentials: got %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}
