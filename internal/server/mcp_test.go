package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mcpToolCall 은 프로덕션 배선 그대로 MCP 핸들러에 실제 JSON-RPC 본문을 준다
// (손으로 만든 대역 없음). mcp_db_test.go:64 callSearchPeople 과 같은 모양이고,
// authContextKey 는 넣지 않는다 — requestHasScope 는 authInfo 가 없으면 API 키가
// 아니라고 보고 통과시킨다(mcp.go:244).
func mcpToolCall(t *testing.T, name string, arguments map[string]any) *http.Request {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": arguments},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(context.WithValue(req.Context(), userContextKey, User{ID: "u1"}))
}

// mcpResultText 는 JSON-RPC 봉투에서 result 와 content[0].text 를 꺼낸다. MCP 는
// 호출자 실수도 HTTP 200 + result.isError:true 로 돌려주는 것이 이 저장소의
// 계약이므로(mcp.go:133·237), 봉투가 그 모양인지까지 함께 확인한다.
func mcpResultText(t *testing.T, rec *httptest.ResponseRecorder) (map[string]any, string) {
	t.Helper()
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 — MCP 오류는 JSON-RPC 봉투로 나간다: %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		JSONRPC string         `json:"jsonrpc"`
		Result  map[string]any `json:"result"`
		Error   *rpcError      `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("응답이 JSON-RPC 봉투가 아니다: %s", rec.Body.String())
	}
	if envelope.JSONRPC != "2.0" {
		t.Fatalf("jsonrpc = %q, want \"2.0\": %s", envelope.JSONRPC, rec.Body.String())
	}
	if envelope.Error != nil {
		t.Fatalf("JSON-RPC error 로 나갔다 — 모델이 보는 자리는 result.isError 다: %+v", envelope.Error)
	}
	content, _ := envelope.Result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("content 가 비어 있다: %v", envelope.Result)
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	return envelope.Result, text
}

// personIDTools 는 person_id 를 uuid 컬럼에 넘기는 세 도구다. arguments 에
// person_id 를 끼워 호출할 수 있도록 나머지 필수 인자를 함께 들고 있다
// (orbit_create_memory 는 title·content 가 없으면 person_id 에 닿기 전에 끝난다).
var personIDTools = []struct {
	tool  string
	extra map[string]any
}{
	{"orbit_get_relationship", nil},
	{"orbit_list_memories", nil},
	{"orbit_create_memory", map[string]any{"title": "제목", "content": "본문"}},
}

func mcpArgs(personID string, extra map[string]any) map[string]any {
	args := map[string]any{"person_id": personID}
	for k, v := range extra {
		args[k] = v
	}
	return args
}

// 세 도구는 person_id 를 uuid 컬럼에 그대로 넘긴다(mcp.go 의 p.id=$2,
// queryMemories 의 NULLIF($2, 빈 문자열)::uuid, mcpCreateMemory 의 WHERE id=$1). 모양이
// 어긋난 값이 DB 까지 가면 postgres 가 22P02 를 내고, 그것은 pgx.ErrNoRows 가
// 아니므로 mcp.go:232~238 이 "요청을 처리하지 못했습니다." 한 문장으로 덮는다 —
// 호출한 모델은 무엇이 잘못됐는지 알 수 없어 스스로 고칠 수 없다.
func TestMCPRejectsMalformedPersonIDBeforeDB(t *testing.T) {
	cases := []struct{ name, personID string }{
		{"uuid가 아닌 문자열", "not-a-uuid"},
		{"하이픈 없는 16진수", "0e2f1c9a4b8d4e6fa1b2c3d4e5f60718"},
		{"중괄호로 감싼 형태", "{0e2f1c9a-4b8d-4e6f-a1b2-c3d4e5f60718}"},
		{"따옴표가 섞인 값", "1' OR '1'='1"},
	}
	for _, tool := range personIDTools {
		for _, tc := range cases {
			t.Run(tool.tool+"/"+tc.name, func(t *testing.T) {
				req := mcpToolCall(t, tool.tool, mcpArgs(tc.personID, tool.extra))
				rec, panicked := callWithoutStore((&Server{}).mcp, req)
				if panicked {
					t.Fatalf("질의까지 내려갔다 — person_id 검증이 DB 앞에서 끝나지 않는다")
				}
				result, text := mcpResultText(t, rec)
				if result["isError"] != true {
					t.Fatalf("isError = %v, want true: %v", result["isError"], result)
				}
				if !strings.Contains(text, "person_id") || !strings.Contains(text, "uuid") {
					t.Fatalf("메시지가 무엇이 잘못됐는지 말하지 않는다: %q", text)
				}
			})
		}
	}
}

// orbit_get_relationship 은 required: ["person_id"] 다. 빈 값이 DB 까지 가면
// p.id 에 빈 문자열이 들어가 역시 22P02 가 되고 같은 일반 메시지로 덮인다 —
// "빠뜨렸다" 는 것을
// 말해 줘야 호출한 모델이 orbit_search_people 로 id 를 먼저 찾을 수 있다.
func TestMCPGetRelationshipRequiresPersonID(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
	}{
		{"person_id 가 빈 문자열", map[string]any{"person_id": ""}},
		{"person_id 키 자체가 없음", map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, panicked := callWithoutStore((&Server{}).mcp, mcpToolCall(t, "orbit_get_relationship", tc.args))
			if panicked {
				t.Fatalf("질의까지 내려갔다 — 빈 person_id 가 DB 앞에서 걸러지지 않는다")
			}
			result, text := mcpResultText(t, rec)
			if result["isError"] != true {
				t.Fatalf("isError = %v, want true: %v", result["isError"], result)
			}
			if !strings.Contains(text, "person_id") {
				t.Fatalf("메시지가 person_id 를 가리키지 않는다: %q", text)
			}
		})
	}
}

// 가드가 정상 입력을 함께 삼키면 지금 되던 조회가 조용히 사라진다. 대문자
// 16진수도 postgres 가 같은 id 로 읽으므로 통과해야 한다(looksLikeUUID 의 판단,
// data.go:69~72). store 가 nil 이라 질의에서 패닉이 나는 것이 곧 "가드를 지나
// DB 까지 갔다" 는 증거다 — 이 하위 시험들은 고치기 전에도 PASS 여야 한다.
func TestMCPLetsWellFormedPersonIDReachDB(t *testing.T) {
	for _, tool := range personIDTools {
		for _, tc := range []struct{ name, personID string }{
			{"소문자 uuid", "0e2f1c9a-4b8d-4e6f-a1b2-c3d4e5f60718"},
			{"대문자 uuid", "0E2F1C9A-4B8D-4E6F-A1B2-C3D4E5F60718"},
		} {
			t.Run(tool.tool+"/"+tc.name, func(t *testing.T) {
				req := mcpToolCall(t, tool.tool, mcpArgs(tc.personID, tool.extra))
				rec, panicked := callWithoutStore((&Server{}).mcp, req)
				if !panicked {
					t.Fatalf("DB 에 닿지 않고 끝났다 — 가드가 정상 id 를 거부한다: %s", rec.Body.String())
				}
			})
		}
	}
}

// orbit_list_memories 의 빈 person_id 는 "사람을 가리지 않는 전체 목록",
// orbit_create_memory 의 빈 person_id 는 "사람 없는 기억" 이라는 지금 돌고 있는
// 기능이다(두 도구는 person_id 를 required 에 넣지 않는다). 가드가 이것까지
// 막으면 회귀다.
func TestMCPAllowsEmptyPersonIDWhereOptional(t *testing.T) {
	cases := []struct {
		tool string
		args map[string]any
	}{
		{"orbit_list_memories", map[string]any{"person_id": ""}},
		{"orbit_list_memories", map[string]any{}},
		{"orbit_create_memory", map[string]any{"person_id": "", "title": "제목", "content": "본문"}},
		{"orbit_create_memory", map[string]any{"title": "제목", "content": "본문"}},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			rec, panicked := callWithoutStore((&Server{}).mcp, mcpToolCall(t, tc.tool, tc.args))
			if !panicked {
				t.Fatalf("DB 에 닿지 않고 끝났다 — 가드가 빈 person_id 를 거부한다: %s", rec.Body.String())
			}
		})
	}
}

// tools/list 가 person_id 의 모양을 알려주지 않으면 외부 에이전트는 사람 이름을
// 넣어 보고 거절당하는 길을 먼저 밟는다. orbit_get_relationship 은 이미
// format: uuid 이므로 나머지 두 도구를 같은 모양으로 맞춘다(설명은 이번에
// 손대는 두 도구에만 요구한다). 도구 이름과 required 배열은 계약이므로 함께
// 고정한다.
func TestMCPToolsDeclarePersonIDFormat(t *testing.T) {
	schemas := map[string]map[string]any{}
	required := map[string][]string{}
	for _, tool := range mcpTools() {
		name, _ := tool["name"].(string)
		schema, _ := tool["inputSchema"].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		schemas[name] = properties
		if list, ok := schema["required"].([]string); ok {
			required[name] = list
		}
	}
	for _, name := range []string{"orbit_get_relationship", "orbit_list_memories", "orbit_create_memory"} {
		t.Run(name, func(t *testing.T) {
			properties := schemas[name]
			if properties == nil {
				t.Fatalf("%s 도구가 없다", name)
			}
			personID, _ := properties["person_id"].(map[string]string)
			if personID == nil {
				t.Fatalf("person_id 속성이 없다: %v", properties["person_id"])
			}
			if personID["type"] != "string" {
				t.Fatalf("type = %q, want \"string\"", personID["type"])
			}
			if personID["format"] != "uuid" {
				t.Fatalf("format = %q, want \"uuid\"", personID["format"])
			}
			if name != "orbit_get_relationship" && personID["description"] == "" {
				t.Fatal("description 이 비어 있다 — 에이전트가 모양을 알 수 없다")
			}
		})
	}
	// required 배열은 바꾸지 않는다는 회귀 기준.
	if got := strings.Join(required["orbit_get_relationship"], ","); got != "person_id" {
		t.Fatalf("orbit_get_relationship required = %q", got)
	}
	if got := strings.Join(required["orbit_create_memory"], ","); got != "title,content" {
		t.Fatalf("orbit_create_memory required = %q", got)
	}
	if _, ok := required["orbit_list_memories"]; ok {
		t.Fatalf("orbit_list_memories 에 required 가 생겼다: %v", required["orbit_list_memories"])
	}
}
