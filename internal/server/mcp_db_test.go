package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hkjang/orbit/internal/id"
	"github.com/hkjang/orbit/internal/store"
)

// seedNamedPerson 은 display_name 을 직접 정하는 사람 행을 넣는다.
// timetravel_db_test.go 의 seedPerson 은 이름을 "시험 인물" 로 고정하므로,
// 행마다 실패하는 뷰가 걸릴 이름('boom…')을 골라야 하는 이 시험에서는 쓸 수 없다.
func seedNamedPerson(t *testing.T, st *store.Store, userID, displayName string) string {
	t.Helper()
	personID := id.New()
	if _, err := st.DB.Exec(context.Background(), `INSERT INTO people (id,user_id,display_name,key_version) VALUES ($1,$2,$3,1)`, personID, userID, displayName); err != nil {
		t.Fatalf("seed person %s: %v", displayName, err)
	}
	return personID
}

// breakPeopleRowStream 은 people 테이블을 같은 이름의 뷰로 가려서, 행 스트림이
// 도중에 깨지는 상황을 결정적으로 만든다. settings_db_test.go:39
// breakUsersRowStream 과 같은 기법이다 — display_name 의 CASE 는 상수가 아니라
// 행마다 평가되므로 'boom…' 사람에 닿는 순간 22P02 로 실패하고, 계획 시점이
// 아니라 실행 중이라서 pgx 는 Next() 를 조용히 false 로 돌려준 뒤 오류를
// rows.Err() 에만 남긴다.
//
// 프로덕션 질의(mcp.go 의 orbit_search_people)는 한 글자도 바꾸지 않는다. 뷰 DDL
// 을 되돌리지 않으면 같은 컨테이너의 다른 DB 시험이 전부 깨지므로 t.Cleanup 으로
// 반드시 원복하고, people 을 공유하는 다른 DB 시험과 섞이면 안 되므로 이 시험은
// t.Parallel() 을 쓰지 않는다. 뷰의 SELECT 목록은 001_init.sql:73~87 의 13개
// 컬럼을 순서대로 지킨다(이후 마이그레이션에 ALTER TABLE people 이 없다).
func breakPeopleRowStream(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()
	if _, err := st.DB.Exec(ctx, `ALTER TABLE people RENAME TO people_probe`); err != nil {
		t.Fatalf("rename people: %v", err)
	}
	t.Cleanup(func() {
		if _, err := st.DB.Exec(ctx, `DROP VIEW IF EXISTS people`); err != nil {
			t.Fatalf("drop probe view: %v", err)
		}
		if _, err := st.DB.Exec(ctx, `ALTER TABLE people_probe RENAME TO people`); err != nil {
			t.Fatalf("restore people: %v", err)
		}
	})
	if _, err := st.DB.Exec(ctx, `CREATE VIEW people AS SELECT id, user_id,
		CASE WHEN display_name LIKE 'boom%' THEN display_name::int::text ELSE display_name END AS display_name,
		company, role_title, avatar_url, email_cipher, phone_cipher, note_cipher,
		key_version, first_met, created_at, updated_at FROM people_probe`); err != nil {
		t.Fatalf("create probe view: %v", err)
	}
}

// callSearchPeople 은 프로덕션 배선 그대로 MCP 핸들러에 실제 JSON-RPC 본문을
// 준다(손으로 만든 대역 없음). authContextKey 는 넣지 않는다 — requestHasScope
// 는 authInfo 가 없으면 API 키가 아니라고 보고 통과시킨다(mcp.go:236~239).
func callSearchPeople(t *testing.T, st *store.Store, userID, query string) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "orbit_search_people", "arguments": map[string]any{"query": query}},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, User{ID: userID}))

	rec := httptest.NewRecorder()
	(&Server{store: st}).mcp(rec, req)

	var envelope struct {
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("응답이 JSON-RPC 봉투가 아니다 (status=%d): %s", rec.Code, rec.Body.String())
	}
	return rec.Code, envelope.Result
}

// TestMCPSearchPeopleBrokenRowStream 은 tools/call orbit_search_people 이 중간에
// 끊긴 행 스트림을 성공으로 내보내지 않는지 고정한다. rows.Err() 를 보지 않으면
// 외부 MCP 에이전트는 잘린 목록과 "검색 결과가 정말 그것뿐" 인 상태를 구분할 수
// 없다. v0.7.6(e16e59d)이 REST 다섯 곳에 같은 가드를 채웠고 저장소에서 이 한 곳만
// 비어 있었다.
func TestMCPSearchPeopleBrokenRowStream(t *testing.T) {
	st := openTestStore(t)
	userID := seedUser(t, st)

	// MCP 질의는 people p JOIN relationships r 이므로 사람마다 relationships
	// 행이 반드시 있어야 한다 — 없으면 애초에 검색에 걸리지 않는다.
	// 뷰의 CASE 는 접두사 LIKE 'boom%' 로 터뜨리므로, 멀쩡한 쪽 이름은 'boom' 을
	// 가운데 넣어 검색어(ILIKE '%boom%')에는 걸리되 뷰에서는 터지지 않게 한다.
	okID := seedNamedPerson(t, st, userID, "ok-boom-"+id.New()[:8])
	seedRelationship(t, st, userID, okID)
	boomID := seedNamedPerson(t, st, userID, "boom-"+id.New()[:8])
	seedRelationship(t, st, userID, boomID)

	// 먼저 정상 경로. 가드가 멀쩡한 목록을 삼키지 않는다는 회귀 기준이라,
	// 뷰를 씌우기 전에 같은 실행에서 확인한다.
	t.Run("정상 검색은 isError 없이 질의에 걸리는 사람 전원을 담는다", func(t *testing.T) {
		status, result := callSearchPeople(t, st, userID, "boom")
		if status != 200 {
			t.Fatalf("status = %d, want 200; result = %v", status, result)
		}
		if _, isErr := result["isError"]; isErr {
			t.Fatalf("정상 검색인데 isError 가 붙었다: %v", result)
		}
		seen := map[string]bool{}
		rows, _ := result["structuredContent"].([]any)
		for _, row := range rows {
			if m, ok := row.(map[string]any); ok {
				seen[m["id"].(string)] = true
			}
		}
		if !seen[okID] || !seen[boomID] {
			t.Fatalf("씨앗 사람 두 명이 모두 나와야 한다: ok=%v boom=%v (총 %d행)", seen[okID], seen[boomID], len(rows))
		}
	})

	breakPeopleRowStream(t, st)

	// 몇 행이 돌아왔는지로 판정하지 않는다 — ORDER BY r.importance DESC 의 Sort
	// 는 블로킹이라 postgres 가 DataRow 를 하나도 보내기 전에 ErrorResponse 를
	// 보낼 수 있고, 잘린 정도는 질의 계획에 따라 달라진다. 결함의 성질은
	// "끊긴 결과가 성공으로 나간다" 는 것이므로 isError 만 본다.
	t.Run("행 스트림이 깨지면 isError 가 붙는다", func(t *testing.T) {
		status, result := callSearchPeople(t, st, userID, "boom")
		if status != 200 {
			t.Fatalf("status = %d, want 200 — MCP 오류는 JSON-RPC 봉투로 나간다; result = %v", status, result)
		}
		if result["isError"] != true {
			rows, _ := result["structuredContent"].([]any)
			t.Fatalf("isError = %v(%d행), want true — 끊긴 검색 결과가 성공으로 나갔다", result["isError"], len(rows))
		}
		content, _ := result["content"].([]any)
		if len(content) == 0 {
			t.Fatalf("content 가 비어 있다: %v", result)
		}
		first, _ := content[0].(map[string]any)
		if first["text"] != "요청을 처리하지 못했습니다." {
			t.Fatalf("content[0].text = %v, want 기존 일반 메시지", first["text"])
		}
	})
}

// breakPeopleUserIDLookup 은 mcpCreateMemory 의 사람 존재 확인 질의
// (SELECT EXISTS … WHERE id=$1 AND user_id=$2)가 실패하는 상황을 결정적으로
// 만든다. breakPeopleRowStream 처럼 display_name 을 깨뜨리면 플래너가 그 식을
// 가지치기해 이 질의는 멀쩡히 성공한다(실제 postgres 로 확인: exists=true,
// err=nil). 그래서 질의가 반드시 읽는 user_id 를 깨뜨린다 — WHERE 절에 쓰이므로
// 'boom…' 사람에 닿는 순간 22P02 가 나고 Scan 이 오류를 돌려준다.
//
// 프로덕션 질의는 한 글자도 바꾸지 않는다. 뷰의 SELECT 목록은 001_init.sql:73~87
// 의 13개 컬럼을 순서대로 지키고, t.Cleanup 으로 반드시 원복한다.
func breakPeopleUserIDLookup(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()
	if _, err := st.DB.Exec(ctx, `ALTER TABLE people RENAME TO people_probe`); err != nil {
		t.Fatalf("rename people: %v", err)
	}
	t.Cleanup(func() {
		if _, err := st.DB.Exec(ctx, `DROP VIEW IF EXISTS people`); err != nil {
			t.Fatalf("drop probe view: %v", err)
		}
		if _, err := st.DB.Exec(ctx, `ALTER TABLE people_probe RENAME TO people`); err != nil {
			t.Fatalf("restore people: %v", err)
		}
	})
	if _, err := st.DB.Exec(ctx, `CREATE VIEW people AS SELECT id,
		CASE WHEN display_name LIKE 'boom%' THEN (display_name::int)::text::uuid ELSE user_id END AS user_id,
		display_name, company, role_title, avatar_url, email_cipher, phone_cipher, note_cipher,
		key_version, first_met, created_at, updated_at FROM people_probe`); err != nil {
		t.Fatalf("create probe view: %v", err)
	}
}

// TestMCPCreateMemorySeparatesLookupFailureFromMissingPerson 은 mcpCreateMemory 가
// "조회가 실패했다" 와 "그런 사람이 없다" 를 섞지 않는지 고정한다. 두 경우를
// 한 묶음으로 pgx.ErrNoRows 로 돌려주면 DB 장애까지 "대상을 찾을 수 없습니다."
// 로 보여 원인이 사라지고, 외부 에이전트는 사람을 다시 찾으면 될 일이라고
// 오해한다.
func TestMCPCreateMemorySeparatesLookupFailureFromMissingPerson(t *testing.T) {
	st := openTestStore(t)
	userID := seedUser(t, st)
	boomID := seedNamedPerson(t, st, userID, "boom-"+id.New()[:8])

	// 먼저 "정말 없는 사람". 모양은 맞지만 심지 않은 id 이므로 가드를 지나
	// 존재 확인 질의까지 가고, 없으므로 기존 "대상을 찾을 수 없습니다." 가 된다.
	t.Run("없는 사람은 대상을 찾을 수 없습니다", func(t *testing.T) {
		status, result := callCreateMemory(t, st, userID, id.New())
		if status != 200 {
			t.Fatalf("status = %d, want 200; result = %v", status, result)
		}
		if result["isError"] != true {
			t.Fatalf("isError = %v, want true: %v", result["isError"], result)
		}
		if text := mcpContentText(result); text != "대상을 찾을 수 없습니다." {
			t.Fatalf("content[0].text = %q, want 기존 부재 메시지", text)
		}
	})

	breakPeopleUserIDLookup(t, st)

	t.Run("조회가 실패하면 일반 메시지가 된다", func(t *testing.T) {
		status, result := callCreateMemory(t, st, userID, boomID)
		if status != 200 {
			t.Fatalf("status = %d, want 200; result = %v", status, result)
		}
		if result["isError"] != true {
			t.Fatalf("isError = %v, want true: %v", result["isError"], result)
		}
		if text := mcpContentText(result); text != "요청을 처리하지 못했습니다." {
			t.Fatalf("content[0].text = %q — DB 장애가 사람 부재로 보인다", text)
		}
	})
}

// callCreateMemory 는 callSearchPeople 과 같은 프로덕션 배선으로 orbit_create_memory
// 를 부른다. title·content 는 person_id 검사 앞에서 걸리지 않게 채워 둔다.
func callCreateMemory(t *testing.T, st *store.Store, userID, personID string) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "orbit_create_memory", "arguments": map[string]any{
			"person_id": personID, "title": "시험 제목", "content": "시험 본문",
		}},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, User{ID: userID}))

	rec := httptest.NewRecorder()
	(&Server{store: st}).mcp(rec, req)

	var envelope struct {
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("응답이 JSON-RPC 봉투가 아니다 (status=%d): %s", rec.Code, rec.Body.String())
	}
	return rec.Code, envelope.Result
}

func mcpContentText(result map[string]any) string {
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		return ""
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	return text
}

// TestMCPSearchPeopleProbeRestoresSchema 는 위 시험의 뷰 DDL 이 원복됐는지
// 확인한다 — 원복되지 않으면 같은 컨테이너의 다른 DB 시험이 전부 깨진다.
// 같은 파일의 시험이 t.Cleanup 으로 되돌리므로 go test 의 파일 내 순차 실행에
// 의존한다(이 파일의 시험은 t.Parallel() 을 쓰지 않는다).
func TestMCPSearchPeopleProbeRestoresSchema(t *testing.T) {
	st := openTestStore(t)
	var relkind string
	if err := st.DB.QueryRow(context.Background(), `SELECT relkind FROM pg_class WHERE relname='people' AND relnamespace='public'::regnamespace`).Scan(&relkind); err != nil {
		t.Fatalf("people 을 pg_class 에서 찾지 못했다: %v", err)
	}
	if relkind != "r" {
		t.Fatalf("people relkind = %q, want \"r\" — 탐침 뷰가 남아 있다", relkind)
	}
	var leftover bool
	if err := st.DB.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM pg_class WHERE relname='people_probe' AND relnamespace='public'::regnamespace)`).Scan(&leftover); err != nil {
		t.Fatalf("people_probe 조회: %v", err)
	}
	if leftover {
		t.Fatal("people_probe 가 남아 있다 — 탐침이 원복되지 않았다")
	}
}
