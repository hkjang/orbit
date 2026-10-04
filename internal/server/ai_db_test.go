package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hkjang/orbit/internal/id"
	"github.com/hkjang/orbit/internal/store"
	"github.com/jackc/pgx/v5"
)

// AI 설정은 전역 행이다. 병렬 실행하지 않고 모든 컬럼을 보존해 복원한다.
// openTestStore와 마찬가지로 격리된 시험 DB에서만 실행해야 한다.
func preserveAISetting(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()
	var value []byte
	var encrypted string
	var updatedBy *string
	var updatedAt time.Time
	err := st.DB.QueryRow(ctx, `SELECT value,encrypted_value,updated_by,updated_at FROM settings WHERE namespace='ai' AND key='provider'`).Scan(&value, &encrypted, &updatedBy, &updatedAt)
	existed := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		var err error
		if existed {
			_, err = st.DB.Exec(ctx, `INSERT INTO settings(namespace,key,value,encrypted_value,updated_by,updated_at) VALUES('ai','provider',$1,$2,$3,$4) ON CONFLICT(namespace,key) DO UPDATE SET value=EXCLUDED.value,encrypted_value=EXCLUDED.encrypted_value,updated_by=EXCLUDED.updated_by,updated_at=EXCLUDED.updated_at`, value, encrypted, updatedBy, updatedAt)
		} else {
			_, err = st.DB.Exec(ctx, `DELETE FROM settings WHERE namespace='ai' AND key='provider'`)
		}
		if err != nil {
			t.Errorf("restore AI setting: %v", err)
		}
	})
}

func TestStreamAIPersonLookup(t *testing.T) {
	st := openTestStore(t)
	preserveAISetting(t, st)
	ctx := context.Background()
	me, other := seedUser(t, st), seedUser(t, st)
	own := seedPerson(t, st, me, time.Now())
	seedRelationship(t, st, me, own)
	foreign := seedPerson(t, st, other, time.Now())
	seedRelationship(t, st, other, foreign)
	unlinked := seedPerson(t, st, me, time.Now())
	broken := seedPerson(t, st, me, time.Now())
	seedRelationship(t, st, me, broken)
	if _, err := st.DB.Exec(ctx, `INSERT INTO memories(id,user_id,person_id,title,content_cipher,key_version,status) VALUES($1,$2,$3,'키 없는 기억','',999,'approved')`, id.New(), me, broken); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			t.Errorf("provider request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"delta\":\"ok\"}\n\n")
	}))
	defer provider.Close()

	cases := []struct {
		name, personID, setting string
		status                  int
		code, message           string
	}{
		{"missing_person", id.New(), "enabled", 404, "not_found", "사람을 찾을 수 없습니다."},
		{"other_users_person", foreign, "enabled", 404, "not_found", "사람을 찾을 수 없습니다."},
		{"missing_relationship", unlinked, "enabled", 404, "not_found", "사람을 찾을 수 없습니다."},
		{"own_person_without_memories", own, "enabled", 200, "", ""},
		{"empty_person_id", "", "enabled", 200, "", ""},
		{"malformed_uuid", "not-a-uuid", "enabled", 400, "validation_error", "사람을 확인해 주세요."},
		{"disabled_ai", id.New(), "disabled", 503, "ai_disabled", "관리자 설정에서 AI를 먼저 활성화해 주세요."},
		{"missing_memory_key", broken, "enabled", 500, "internal_error", "요청을 처리하지 못했습니다."},
		{"missing_setting", id.New(), "missing", 500, "internal_error", "요청을 처리하지 못했습니다."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := AISettings{Enabled: tc.setting == "enabled", BaseURL: provider.URL, Model: "test", MaxOutputTokens: 32, RequestTimeoutSeconds: 5}
			raw, err := json.Marshal(settings)
			if err != nil {
				t.Fatal(err)
			}
			if tc.setting == "missing" {
				_, err = st.DB.Exec(ctx, `DELETE FROM settings WHERE namespace='ai' AND key='provider'`)
			} else {
				_, err = st.DB.Exec(ctx, `INSERT INTO settings(namespace,key,value,encrypted_value) VALUES('ai','provider',$1,'') ON CONFLICT(namespace,key) DO UPDATE SET value=EXCLUDED.value,encrypted_value=''`, raw)
			}
			if err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(map[string]string{"prompt": "관계를 알려 주세요.", "person_id": tc.personID})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream", bytes.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), userContextKey, User{ID: me}))
			rec := httptest.NewRecorder()
			before := calls.Load()
			(&Server{store: st}).streamAI(rec, req)
			gotCalls := calls.Load() - before
			if tc.status != 200 && gotCalls != 0 {
				t.Errorf("rejected request called provider %d times", gotCalls)
			}
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tc.status, rec.Body.String())
			}
			contentType, _, err := mime.ParseMediaType(rec.Header().Get("Content-Type"))
			if err != nil {
				t.Fatal(err)
			}
			if tc.status == 200 {
				if contentType != "text/event-stream" || gotCalls != 1 {
					t.Fatalf("content-type=%s, provider calls=%d", contentType, gotCalls)
				}
				want := "event: meta\ndata: {\"max_output_tokens\":32,\"model\":\"test\"}\n\nevent: delta\ndata: {\"text\":\"ok\"}\n\nevent: done\ndata: {\"ok\":true}\n\n"
				if rec.Body.String() != want {
					t.Fatalf("SSE = %q, want %q", rec.Body.String(), want)
				}
			} else {
				if contentType != "application/json" {
					t.Fatalf("content-type=%s", contentType)
				}
				if strings.Contains(rec.Body.String(), "event:") {
					t.Fatalf("SSE started: %s", rec.Body.String())
				}
				var out apiError
				if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				if out.Error.Code != tc.code || out.Error.Message != tc.message {
					t.Fatalf("error = %+v", out.Error)
				}
			}
		})
	}
}

// captureLogs 는 전역 기본 로거를 시험용 JSON 핸들러로 바꾸고 t.Cleanup 으로
// 원복한다. slog 기본 로거는 AI 설정 행과 마찬가지로 전역 상태라 이 시험은
// 병렬로 돌릴 수 없다. 돌려주는 함수는 그동안 쌓인 레코드를 비우면서 돌려준다.
func captureLogs(t *testing.T) func() []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return func() []map[string]any {
		raw := strings.TrimSpace(buf.String())
		buf.Reset()
		out := make([]map[string]any, 0)
		if raw == "" {
			return out
		}
		for _, line := range strings.Split(raw, "\n") {
			var rec map[string]any
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				t.Fatalf("로그 줄을 읽지 못했다: %v (%q)", err, line)
			}
			out = append(out, rec)
		}
		return out
	}
}

// streamAI 는 proxyAIStream 이 돌려준 오류를 버린 채 사용자에게 고정 문구 SSE 만
// 보냈다 — 제공자 401/429/500·잘못된 엔드포인트·타임아웃이 전부 같은 한 문장이고
// 서버 로그에도 아무것도 남지 않아 운영자가 원인을 알 방법이 없었다.
// respond.go:47·export.go:85 의 slog.Error 관례가 이 경로만 비어 있었다.
//
// 사용자에게 가는 바이트열은 그대로 두고 로그 한 줄만 늘어나야 하므로, 정상
// 경로의 SSE 바이트와 "로그 0개" 도 같은 실행에서 함께 본다.
func TestStreamAIProviderFailureIsLogged(t *testing.T) {
	st := openTestStore(t)
	preserveAISetting(t, st)
	ctx := context.Background()
	me := seedUser(t, st)
	person := seedPerson(t, st, me, time.Now())
	seedRelationship(t, st, me, person)
	drain := captureLogs(t)

	const (
		modeOK = iota
		modeFail
		modeCancel
	)
	var mode atomic.Int32
	// 취소 모드에서 제공자가 부를 함수. 맥락 조회가 끝난 뒤, 제공자 호출이
	// 진행 중일 때 요청 컨텍스트를 끊어야 proxyAIStream 이 context canceled 를
	// 돌려준다 — 핸들러 진입 전에 끊으면 relationshipContext 에서 먼저 터져
	// 이번에 고치는 분기를 아예 지나지 않는다.
	var cancelCurrent atomic.Pointer[context.CancelFunc]
	// 취소 모드의 제공자 핸들러를 풀어 주는 신호. 시험이 끝날 때 닫는다 —
	// 닫지 않으면 provider.Close() 가 붙잡힌 핸들러를 기다린다.
	release := make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch mode.Load() {
		case modeFail:
			// 제공자 오류 본문은 한국어일 수 있다 — safeAIError 가 룬을 쪼개지
			// 않는지도 이 경로로 함께 지난다.
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "제공자 내부 오류: 모델을 호출하지 못했습니다")
			return
		case modeCancel:
			if cancel := cancelCurrent.Load(); cancel != nil {
				(*cancel)()
			}
			// 응답을 쓰지 않고 붙잡아 둔다. 먼저 써 버리면 client.Do 가 취소
			// 대신 200 을 볼 수 있어 결정적이지 않다. 시험이 끝나면 release 가
			// 닫히고, 끝내 닫히지 않더라도 멈추지 않도록 상한을 둔다.
			select {
			case <-r.Context().Done():
			case <-release:
			case <-time.After(30 * time.Second):
			}
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"delta\":\"ok\"}\n\n")
	}))
	defer provider.Close()
	// provider.Close() 보다 먼저 돌아야 한다(defer 는 LIFO).
	defer close(release)

	settings := AISettings{Enabled: true, BaseURL: provider.URL, Model: "test", MaxOutputTokens: 32, RequestTimeoutSeconds: 5}
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(ctx, `INSERT INTO settings(namespace,key,value,encrypted_value) VALUES('ai','provider',$1,'') ON CONFLICT(namespace,key) DO UPDATE SET value=EXCLUDED.value,encrypted_value=''`, raw); err != nil {
		t.Fatal(err)
	}

	call := func(t *testing.T) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(map[string]string{"prompt": "관계를 알려 주세요.", "person_id": person})
		if err != nil {
			t.Fatal(err)
		}
		reqCtx, cancel := context.WithCancel(context.WithValue(context.Background(), userContextKey, User{ID: me}))
		defer cancel()
		cancelCurrent.Store(&cancel)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream", bytes.NewReader(body)).WithContext(reqCtx)
		rec := httptest.NewRecorder()
		(&Server{store: st}).streamAI(rec, req)
		return rec
	}

	t.Run("제공자 실패는 ERROR 한 줄로 남는다", func(t *testing.T) {
		mode.Store(modeFail)
		rec := call(t)
		records := drain()
		if len(records) == 0 {
			t.Fatalf("로그 레코드가 0개 — 제공자 실패 원인이 서버 로그에 남지 않는다 (SSE=%q)", rec.Body.String())
		}
		var found bool
		for _, rec := range records {
			line, err := json.Marshal(rec)
			if err != nil {
				t.Fatal(err)
			}
			if rec["level"] != "ERROR" {
				continue
			}
			if strings.Contains(string(line), "500") && strings.Contains(string(line), me) {
				found = true
			}
		}
		if !found {
			t.Fatalf("제공자 상태코드와 사용자 id 를 담은 ERROR 레코드가 없다: %v", records)
		}
		// 사용자에게 가는 응답은 바이트 단위로 예전과 같아야 한다.
		if rec.Code != 200 {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		want := "event: meta\ndata: {\"max_output_tokens\":32,\"model\":\"test\"}\n\nevent: error\ndata: {\"message\":\"AI 응답을 완료하지 못했습니다.\"}\n\n"
		if rec.Body.String() != want {
			t.Fatalf("SSE = %q, want %q", rec.Body.String(), want)
		}
	})

	t.Run("정상 경로는 로그를 남기지 않는다", func(t *testing.T) {
		mode.Store(modeOK)
		rec := call(t)
		if records := drain(); len(records) != 0 {
			t.Fatalf("정상 경로가 로그 %d개를 남겼다: %v", len(records), records)
		}
		if rec.Code != 200 {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		want := "event: meta\ndata: {\"max_output_tokens\":32,\"model\":\"test\"}\n\nevent: delta\ndata: {\"text\":\"ok\"}\n\nevent: done\ndata: {\"ok\":true}\n\n"
		if rec.Body.String() != want {
			t.Fatalf("SSE = %q, want %q", rec.Body.String(), want)
		}
	})

	// 클라이언트가 탭을 닫으면(context canceled) 정상적인 이탈이므로 ERROR 로
	// 남기지 않는다. 이것이 없으면 사용자 이탈마다 오탐 ERROR 가 쌓인다.
	t.Run("클라이언트 취소는 ERROR 로 남기지 않는다", func(t *testing.T) {
		mode.Store(modeCancel)
		rec := call(t)
		// 취소에 걸려 오류 분기를 지났다는 것부터 확인한다 — done 이 나왔다면
		// 이 하위 시험은 아무것도 증명하지 못한다.
		if strings.Contains(rec.Body.String(), "event: done") {
			t.Fatalf("취소가 오류 분기에 닿지 않았다: %q", rec.Body.String())
		}
		for _, record := range drain() {
			if record["level"] == "ERROR" {
				t.Fatalf("취소된 요청이 ERROR 를 남겼다: %v", record)
			}
		}
	})
}
