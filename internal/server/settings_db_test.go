package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hkjang/orbit/internal/id"
	"github.com/hkjang/orbit/internal/store"
)

// seedNamedUser 는 username 과 created_at 을 직접 정하는 사용자 행을 넣는다.
// timetravel_db_test.go 의 seedUser 는 username 을 스스로 짓기 때문에, 행마다
// 실패하는 뷰가 걸릴 이름('boom…')을 골라야 하는 이 시험에서는 쓸 수 없다.
func seedNamedUser(t *testing.T, st *store.Store, username string, createdAt time.Time) string {
	t.Helper()
	userID := id.New()
	if _, err := st.DB.Exec(context.Background(), `INSERT INTO users (id,username,display_name,created_at) VALUES ($1,$2,$3,$4)`, userID, username, "시험 사용자 "+username, createdAt); err != nil {
		t.Fatalf("seed user %s: %v", username, err)
	}
	t.Cleanup(func() {
		_, _ = st.DB.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})
	return userID
}

// breakUsersRowStream 은 users 테이블을 같은 이름의 뷰로 가려서, 행 스트림이
// 도중에 깨지는 상황을 결정적으로 만든다. display_name 의 CASE 는 상수가 아니라
// 행마다 평가되므로 'boom…' 사용자에 닿는 순간 22P02 로 실패한다 — 계획 시점이
// 아니라 실행 중이라서, postgres 가 이미 보낸 행이 있으면 pgx 는 rows.Next() 를
// true 로 한 번 돌려준 뒤 false 를 주고 오류는 rows.Err() 에만 남긴다.
//
// 프로덕션 질의(settings.go:listUsers)는 한 글자도 바꾸지 않는다. 뷰는 시험이
// 끝나면 반드시 원복해야 하므로 t.Cleanup 으로 되돌리고, users 를 공유하는 다른
// DB 시험과 섞이면 안 되므로 이 시험은 t.Parallel() 을 쓰지 않는다.
func breakUsersRowStream(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()
	if _, err := st.DB.Exec(ctx, `ALTER TABLE users RENAME TO users_probe`); err != nil {
		t.Fatalf("rename users: %v", err)
	}
	t.Cleanup(func() {
		if _, err := st.DB.Exec(ctx, `DROP VIEW IF EXISTS users`); err != nil {
			t.Fatalf("drop probe view: %v", err)
		}
		if _, err := st.DB.Exec(ctx, `ALTER TABLE users_probe RENAME TO users`); err != nil {
			t.Fatalf("restore users: %v", err)
		}
	})
	if _, err := st.DB.Exec(ctx, `CREATE VIEW users AS SELECT id, username, email,
		CASE WHEN username LIKE 'boom%' THEN username::int::text ELSE display_name END AS display_name,
		password_hash, role, status, oidc_subject, last_login_at, created_at, updated_at FROM users_probe`); err != nil {
		t.Fatalf("create probe view: %v", err)
	}
}

func callListUsers(t *testing.T, st *store.Store) (int, map[string]any) {
	t.Helper()
	s := &Server{store: st}
	rec := httptest.NewRecorder()
	s.listUsers(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("응답이 JSON 이 아니다 (status=%d): %s", rec.Code, rec.Body.String())
	}
	return rec.Code, body
}

// TestListUsersBrokenRowStream 은 GET /api/v1/users 가 중간에 끊긴 행 스트림을
// 성공(200)으로 내보내지 않는지 고정한다. rows.Err() 를 보지 않으면 호출자는
// 절반만 담긴 목록과 "사용자가 정말 그것뿐" 인 상태를 구분할 수 없다.
func TestListUsersBrokenRowStream(t *testing.T) {
	st := openTestStore(t)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	okID := seedNamedUser(t, st, "ok-"+id.New()[:8], base)
	boomID := seedNamedUser(t, st, "boom-"+id.New()[:8], base.Add(time.Hour))

	// 먼저 정상 경로. 가드가 멀쩡한 목록을 삼키지 않는다는 회귀 기준이라,
	// 뷰를 씌우기 전에 같은 실행에서 확인한다.
	t.Run("정상 목록은 200 이고 모든 행을 담는다", func(t *testing.T) {
		status, body := callListUsers(t, st)
		if status != 200 {
			t.Fatalf("status = %d, want 200; body = %v", status, body)
		}
		seen := map[string]bool{}
		rows, _ := body["users"].([]any)
		for _, row := range rows {
			if m, ok := row.(map[string]any); ok {
				seen[m["id"].(string)] = true
			}
		}
		if !seen[okID] || !seen[boomID] {
			t.Fatalf("씨앗 사용자 두 명이 모두 나와야 한다: ok=%v boom=%v (총 %d행)", seen[okID], seen[boomID], len(rows))
		}
	})

	breakUsersRowStream(t, st)

	t.Run("행 스트림이 깨지면 200 이 아니라 500 이다", func(t *testing.T) {
		status, body := callListUsers(t, st)
		if status != 500 {
			rows, _ := body["users"].([]any)
			t.Fatalf("status = %d(%d행), want 500 — 끊긴 목록이 성공으로 나갔다", status, len(rows))
		}
		errObj, _ := body["error"].(map[string]any)
		if errObj == nil || errObj["code"] != "internal_error" {
			t.Fatalf("error.code = %v, want internal_error; body = %v", body["error"], body)
		}
	})
}

// TestGuardedListHandlersStillServeNormalRows 는 같은 rows.Err() 가드를 받은
// 나머지 네 목록 핸들러가 멀쩡한 스트림을 삼키지 않는지 실제 postgres 위에서
// 고정한다. 깨진 스트림 쪽은 listUsers 하나로 증명했고(위 시험), 여기서는 가드가
// 정상 경로에 잘못 끼어들지 않는다는 반대편 기준만 본다.
func TestGuardedListHandlersStillServeNormalRows(t *testing.T) {
	st := openTestStore(t)
	userID := seedUser(t, st)
	ctx := context.WithValue(context.Background(), userContextKey, User{ID: userID, Role: "admin"})

	if _, err := st.DB.Exec(context.Background(), `INSERT INTO api_keys(id,user_id,name,prefix,secret_hash,scopes) VALUES($1,$2,'시험 키','orb_probe123','hash','["read"]'::jsonb)`, id.New(), userID); err != nil {
		t.Fatalf("seed api key: %v", err)
	}

	cases := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
		key     string
		minRows int
	}{
		{"listPeople", (&Server{store: st}).listPeople, "people", 0},
		{"listKeys", (&Server{store: st}).listKeys, "keys", 0},
		{"listAPIKeys", (&Server{store: st}).listAPIKeys, "api_keys", 1},
		{"listKeyPermissions", (&Server{store: st}).listKeyPermissions, "permissions", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.handler(rec, httptest.NewRequest(http.MethodGet, "/probe", nil).WithContext(ctx))
			if rec.Code != 200 {
				t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("응답이 JSON 이 아니다: %s", rec.Body.String())
			}
			rows, ok := body[tc.key].([]any)
			if !ok {
				t.Fatalf("%q 가 배열이 아니다: %s", tc.key, rec.Body.String())
			}
			if len(rows) < tc.minRows {
				t.Fatalf("%q 행이 %d개, 최소 %d개여야 한다", tc.key, len(rows), tc.minRows)
			}
		})
	}
}
