package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractDelta(t *testing.T) {
	cases := []struct {
		name, event string
		payload     map[string]any
		want        string
	}{
		{"responses", "response.output_text.delta", map[string]any{"delta": "안녕"}, "안녕"},
		{"chat-compatible", "", map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": "hello"}}}}, "hello"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractDelta(tc.event, tc.payload); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestRequiredScope(t *testing.T) {
	cases := []struct{ method, path, want string }{
		{http.MethodGet, "/api/v1/people/", "people:read"},
		{http.MethodPost, "/api/v1/people/", "people:write"},
		{http.MethodPost, "/api/v1/ai/stream", "ai:invoke"},
		{http.MethodPost, "/mcp", "mcp:use"},
		{http.MethodGet, "/api/v1/admin/users", "session-only"},
	}
	for _, tc := range cases {
		if got := requiredScope(tc.method, tc.path); got != tc.want {
			t.Errorf("%s %s: got %q want %q", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestStablePositionIsDeterministicAndBounded(t *testing.T) {
	x1, y1 := stablePosition("f7e62238-bad1-4c73-ad45-a46777cc20ef")
	x2, y2 := stablePosition("f7e62238-bad1-4c73-ad45-a46777cc20ef")
	if x1 != x2 || y1 != y2 {
		t.Fatal("position changed")
	}
	if x1*x1+y1*y1 > 1 {
		t.Fatalf("position outside unit orbit: %f,%f", x1, y1)
	}
}

func TestMCPVersionDiscovery(t *testing.T) {
	req := rpcRequest{Method: "server/discover", Params: []byte(`{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}`)}
	httpReq := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	if got := mcpRequestedVersion(httpReq, req); got != "2026-07-28" {
		t.Fatalf("got %q", got)
	}
	if !mcpVersionSupported(gotVersion(t, httpReq, req)) {
		t.Fatal("latest MCP version must be supported")
	}
}

func gotVersion(t *testing.T, r *http.Request, req rpcRequest) string {
	t.Helper()
	return mcpRequestedVersion(r, req)
}

func TestResponsesStreamNormalization(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"stream":true`) || !strings.Contains(string(body), `"store":false`) {
			t.Fatalf("unexpected request: %s", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"안녕\"}\n\n")
	}))
	defer upstream.Close()
	recorder := httptest.NewRecorder()
	s := &Server{}
	err := s.proxyAIStream(context.Background(), recorder, recorder, AISettings{BaseURL: upstream.URL, Model: "test", RequestTimeoutSeconds: 10}, "질문", "기록", 262144)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(recorder.Body.String(), `event: delta`) || !strings.Contains(recorder.Body.String(), `안녕`) {
		t.Fatalf("unexpected normalized stream: %s", recorder.Body.String())
	}
}

func TestNormalizeLinkIsOrderIndependent(t *testing.T) {
	a, b := normalizeLink("f0000000-0000-0000-0000-000000000001", "10000000-0000-0000-0000-000000000002")
	c, d := normalizeLink("10000000-0000-0000-0000-000000000002", "f0000000-0000-0000-0000-000000000001")
	if a != c || b != d {
		t.Fatalf("link normalization must not depend on argument order: got (%s,%s) and (%s,%s)", a, b, c, d)
	}
	if a >= b {
		t.Fatalf("normalized link must satisfy person_a < person_b, got (%s,%s)", a, b)
	}
}

func TestLinkKindsCoverStoredValues(t *testing.T) {
	// person_links의 CHECK 제약과 동일한 집합이어야 저장 실패가 나지 않습니다.
	for _, kind := range []string{"colleague", "family", "friend", "community", "knows"} {
		if _, ok := linkKinds[kind]; !ok {
			t.Fatalf("kind %q is allowed by the schema but has no label", kind)
		}
	}
	if len(linkKinds) != 5 {
		t.Fatalf("linkKinds has %d entries, schema allows 5", len(linkKinds))
	}
}
