package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// captureErrorLogs는 기본 로거를 버퍼로 갈아 끼운다. slog 기본 로거는
// 전역이므로 t.Cleanup으로 반드시 원복하고 병렬로 돌리지 않는다.
func captureErrorLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// userRequest는 심은 사용자를 컨텍스트에 실어 준다. personRequest는 사용자를
// "u1"로 고정하므로 실제 DB 행이 필요한 이 시험에서는 쓸 수 없다.
func userRequest(method, target, body, userID string, params map[string]string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, userContextKey, User{ID: userID})
	return req.WithContext(ctx)
}

// 사람 존재 확인의 DB 장애가 "사람을 찾을 수 없습니다."(404) / "관계 인물을
// 확인해 주세요."(400)로 덮이면 호출자는 자기 잘못이라고 읽고, internalError를
// 지나지 않으므로 서버 로그에는 원인이 한 줄도 남지 않는다. 조회 오류를
// 갈라 올리면 500이 나가고 slog.Error에 원인이 남는지를 실제 postgres로 본다.
//
// 모양 가드를 지난 정상 uuid여야 이 분기에 닿으므로 심은 사람의 진짜 id를
// 쓴다. display_name이 아니라 질의가 반드시 읽는 user_id를 깨뜨려야 한다 —
// display_name을 깨뜨리면 플래너가 그 식을 가지치기해 SELECT EXISTS가
// 멀쩡히 성공한다(2026-10-05 회차의 실측 교훈).
func TestPersonExistsLookupFailureBecomesInternalError(t *testing.T) {
	cases := []struct {
		name     string
		request  func(userID, personID string) *http.Request
		handler  func(s *Server) func(http.ResponseWriter, *http.Request)
		wantPath string
	}{
		{
			name: "createInteraction",
			request: func(userID, personID string) *http.Request {
				return userRequest(http.MethodPost, "/api/v1/people/"+personID+"/interactions",
					`{"kind":"meeting","summary":"x"}`, userID, map[string]string{"personID": personID})
			},
			handler:  func(s *Server) func(http.ResponseWriter, *http.Request) { return s.createInteraction },
			wantPath: "/interactions",
		},
		{
			name: "createMemory",
			request: func(userID, personID string) *http.Request {
				body, _ := json.Marshal(map[string]string{"person_id": personID, "title": "제목", "content": "내용"})
				return userRequest(http.MethodPost, "/api/v1/memories", string(body), userID, nil)
			},
			handler:  func(s *Server) func(http.ResponseWriter, *http.Request) { return s.createMemory },
			wantPath: "/api/v1/memories",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			userID := seedUser(t, st)
			// 'boom'으로 시작하는 이름이라야 뷰가 이 행의 user_id에서 터진다.
			personID := seedNamedPerson(t, st, userID, "boom-"+userID[:8])
			logs := captureErrorLogs(t)
			// 씨앗을 심은 뒤에 깨뜨린다 — t.Cleanup이 LIFO라 테이블 원복이
			// seedUser의 DELETE보다 먼저 돌아야 행이 실제로 지워진다.
			breakPeopleUserIDLookup(t, st)

			rec := httptest.NewRecorder()
			tc.handler(&Server{store: st})(rec, tc.request(userID, personID))

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("상태가 %d, 500이어야 한다 — DB 장애가 호출자 실수로 보인다 (본문: %s)", rec.Code, rec.Body.String())
			}
			var out apiError
			if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
			if out.Error.Code != "internal_error" {
				t.Fatalf("오류 코드가 %q, internal_error이어야 한다", out.Error.Code)
			}
			line := logs.String()
			if line == "" {
				t.Fatalf("로그 레코드가 0개 — DB 장애 원인이 서버 로그에 남지 않는다")
			}
			if !strings.Contains(line, "22P02") {
				t.Fatalf("로그에 원인(22P02)이 없다: %s", line)
			}
			if !strings.Contains(line, tc.wantPath) {
				t.Fatalf("로그에 경로 %q가 없다: %s", tc.wantPath, line)
			}
		})
	}
}
