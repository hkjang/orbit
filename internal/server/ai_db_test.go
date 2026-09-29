package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
