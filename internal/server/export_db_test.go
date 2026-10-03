package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hkjang/orbit/internal/id"
	"github.com/hkjang/orbit/internal/store"
)

// callExport 는 프로덕션 배선 그대로 내보내기 핸들러를 부른다(손으로 만든 대역
// 없음). exportData 는 userFromContext 만 쓰고 chi URL 파라미터를 보지 않으므로
// 라우터를 세울 필요가 없다(server.go:71 이 p.Get("/export", s.exportData) 로
// 맨다). httptest.NewRecorder 는 http.Flusher 를 구현하므로 섹션마다 도는
// Flush 분기도 실제로 지난다.
func callExport(t *testing.T, st *store.Store, userID string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/personal/export", nil)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, User{
		ID: userID, Username: "t-" + userID[:8], Role: "user",
	}))

	rec := httptest.NewRecorder()
	(&Server{store: st, version: "test"}).exportData(rec, req)
	return rec.Code, rec.Body.String()
}

// TestExportBrokenSectionStaysParseable 는 섹션 조회가 중간에 끊겼을 때도 응답
// 본문이 JSON 으로 남는지 고정한다.
//
// 끊긴 내보내기의 본문이 파싱조차 안 되면 "complete 표시가 없다" 는 신호를 읽을
// 방법이 없다 — jq·json.load·브라우저 모두 unexpected end of input 만 낸다.
// 배열과 최상위 객체를 닫고 complete:false 로 끝내면 사용자는 어느 섹션까지
// 받았는지를 파싱해서 알 수 있다.
func TestExportBrokenSectionStaysParseable(t *testing.T) {
	st := openTestStore(t)

	// 사람이 한 명이라도 스캔되면 exportPeople 이 exportKey → dataKeyVersion →
	// store.Vault.UnwrapKey 로 내려간다. openTestStore 는 store.Open(ctx,dsn,nil)
	// 이라 Vault 가 nil 이므로, 정상 경로 하위 시험은 사람이 0명인 별도 사용자로
	// 부른다(exportPeople 은 WHERE p.user_id=$1 이므로 다른 사용자 행은 섞이지
	// 않는다).
	emptyUser := seedUser(t, st)

	// 실패 경로용 사용자. exportPeople 질의는 people p JOIN relationships r 이므로
	// relationships 행을 반드시 같이 심어야 그 사람이 조인 결과에 들어오고 뷰의
	// CASE 가 평가되어 스트림이 깨진다.
	boomUser := seedUser(t, st)
	boomPerson := seedNamedPerson(t, st, boomUser, "boom-"+id.New()[:8])
	seedRelationship(t, st, boomUser, boomPerson)

	// 정상 경로를 먼저 본다. 가드가 멀쩡한 내보내기를 바꾸지 않는다는 회귀
	// 기준이라 고치기 전에도 PASS 여야 하고, 뷰를 씌우기 전에 확인해야 한다.
	t.Run("정상 내보내기는 파싱되고 complete=true 다", func(t *testing.T) {
		status, body := callExport(t, st, emptyUser)
		if status != 200 {
			t.Fatalf("status = %d, want 200; body = %s", status, body)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(body), &got); err != nil {
			t.Fatalf("정상 내보내기가 파싱되지 않는다: %v; body = %s", err, body)
		}
		if got["complete"] != true {
			t.Fatalf("complete = %v, want true; body = %s", got["complete"], body)
		}
		for _, key := range []string{"people", "interactions", "memories", "links"} {
			if _, ok := got[key].([]any); !ok {
				t.Fatalf("%s 가 배열로 들어 있어야 한다: %T; body = %s", key, got[key], body)
			}
		}
	})

	// 뷰는 select 목록에 CASE 가 있어 postgres 가 자동 갱신 가능 뷰로 보지 않는다
	// — 뷰를 씌운 뒤에는 INSERT INTO people 이 실패하므로 씨앗이 먼저여야 한다.
	breakPeopleRowStream(t, st)

	t.Run("끊긴 섹션도 파싱되고 complete=false 다", func(t *testing.T) {
		// 상태 코드는 200 그대로다 — 첫 Fprintf 에서 헤더가 이미 나갔으므로
		// 바꿀 수 없고, 바꾸려 들면 superfluous WriteHeader 만 남는다.
		status, body := callExport(t, st, boomUser)
		if status != 200 {
			t.Fatalf("status = %d, want 200 — 헤더는 이미 나갔다; body = %s", status, body)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(body), &got); err != nil {
			t.Fatalf("끊긴 내보내기가 파싱되지 않는다: %v; body = %s", err, body)
		}
		if got["complete"] != false {
			t.Fatalf("complete = %v, want false; body = %s", got["complete"], body)
		}
		// 이미 성공한 부분은 그대로 읽힌다.
		if got["exported_at"] == "" || got["exported_at"] == nil {
			t.Fatalf("exported_at 이 비어 있다; body = %s", body)
		}
		if got["service_version"] != "test" {
			t.Fatalf("service_version = %v, want \"test\"; body = %s", got["service_version"], body)
		}
		user, _ := got["user"].(map[string]any)
		if user["id"] != boomUser {
			t.Fatalf("user.id = %v, want %s; body = %s", user["id"], boomUser, body)
		}
		// 어디서 끊겼는지를 파싱으로 알 수 있어야 한다 — 실패한 섹션 뒤의 키는
		// 아예 없다.
		for _, key := range []string{"interactions", "memories", "links"} {
			if _, ok := got[key]; ok {
				t.Fatalf("%s 는 끊긴 뒤의 섹션이라 없어야 한다; body = %s", key, body)
			}
		}
		if _, ok := got["people"]; !ok {
			t.Fatalf("끊긴 섹션 키 people 자체는 열려 있었으므로 남아야 한다; body = %s", body)
		}
		if got["failed_section"] != "people" {
			t.Fatalf("failed_section = %v, want \"people\"; body = %s", got["failed_section"], body)
		}
		// 끊긴 내보내기를 성공으로 감사 기록하지 않는다.
		var audits int
		if err := st.DB.QueryRow(context.Background(),
			`SELECT count(*) FROM audit_logs WHERE actor_id=$1::uuid AND action='data.export'`,
			boomUser).Scan(&audits); err != nil {
			t.Fatalf("audit_logs 조회: %v", err)
		}
		if audits != 0 {
			t.Fatalf("data.export 감사 기록 = %d건, want 0 — 끊긴 내보내기는 성공이 아니다", audits)
		}
	})
}

// TestExportProbeRestoresSchema 는 위 시험의 뷰 DDL 이 원복됐는지 확인한다 —
// 원복되지 않으면 같은 컨테이너의 다른 DB 시험이 전부 깨진다. go test 의 파일 내
// 선언 순서에 의존한다(이 파일의 시험은 t.Parallel() 을 쓰지 않는다).
func TestExportProbeRestoresSchema(t *testing.T) {
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
