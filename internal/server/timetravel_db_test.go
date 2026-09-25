package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/hkjang/orbit/internal/id"
	"github.com/hkjang/orbit/internal/store"
)

// 실제 postgres 를 끼는 테스트. ORBIT_TEST_DATABASE_URL 이 비어 있으면 건너뛴다.
//
//	docker run -d --name orbit-pg -e POSTGRES_PASSWORD=orbit -e POSTGRES_DB=orbit -p 127.0.0.1:55470:5432 postgres:16-alpine
//	ORBIT_TEST_DATABASE_URL='postgres://postgres:orbit@127.0.0.1:55470/orbit?sslmode=disable' go test -run TestOrbitRange -v ./internal/server/
//
// 마이그레이션은 store.Open 이 그대로 적용한다. 데이터는 시험마다 새 사용자
// 아래에만 넣고 끝나면 사용자 행을 지워(FK 가 ON DELETE CASCADE) 흔적을 남기지 않는다.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("ORBIT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORBIT_TEST_DATABASE_URL 이 비어 있어 DB 테스트를 건너뛴다")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := store.Open(ctx, dsn, nil)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

// seedUser 는 사용자 행을 직접 넣는다. Bootstrap 은 관리자 계정과 설정 행을
// 함께 만들어 부작용이 있으므로 쓰지 않는다.
func seedUser(t *testing.T, st *store.Store) string {
	t.Helper()
	userID := id.New()
	if _, err := st.DB.Exec(context.Background(), `INSERT INTO users (id,username,display_name) VALUES ($1,$2,$3)`, userID, "t-"+userID[:8], "시험 사용자"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = st.DB.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})
	return userID
}

func seedPerson(t *testing.T, st *store.Store, userID string, createdAt time.Time) string {
	t.Helper()
	personID := id.New()
	if _, err := st.DB.Exec(context.Background(), `INSERT INTO people (id,user_id,display_name,key_version,created_at) VALUES ($1,$2,$3,1,$4)`, personID, userID, "시험 인물", createdAt); err != nil {
		t.Fatalf("seed person: %v", err)
	}
	return personID
}

func seedInteraction(t *testing.T, st *store.Store, userID, personID string, occurredAt time.Time) {
	t.Helper()
	if _, err := st.DB.Exec(context.Background(), `INSERT INTO interactions (id,user_id,person_id,kind,occurred_at,key_version) VALUES ($1,$2,$3,'note',$4,1)`, id.New(), userID, personID, occurredAt); err != nil {
		t.Fatalf("seed interaction: %v", err)
	}
}

// legacyOrbitRange 는 예전 질의(사람 LEFT JOIN 교류 — 사람×교류 곱을 만든다)를
// 회귀 기준으로 그대로 돌린다. 새 질의는 값이 같아야 한다.
func legacyOrbitRange(t *testing.T, st *store.Store, userID string) *time.Time {
	t.Helper()
	var first *time.Time
	if err := st.DB.QueryRow(context.Background(), `SELECT least(min(p.created_at),min(i.occurred_at)) FROM people p LEFT JOIN interactions i ON i.user_id=p.user_id WHERE p.user_id=$1`, userID).Scan(&first); err != nil {
		t.Fatalf("legacy orbitRange: %v", err)
	}
	return first
}

func sameInstant(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func TestOrbitRangeMatchesLegacyJoin(t *testing.T) {
	st := openTestStore(t)
	s := &Server{store: st}
	ctx := context.Background()
	base := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)

	// 다른 사용자의 더 오래된 사람·교류를 먼저 깔아 둔다. 아래 모든 경우에서
	// 내 값에 섞여 들면 안 된다.
	other := seedUser(t, st)
	otherPerson := seedPerson(t, st, other, base.AddDate(-5, 0, 0))
	seedInteraction(t, st, other, otherPerson, base.AddDate(-6, 0, 0))

	check := func(t *testing.T, userID string, want *time.Time) {
		t.Helper()
		got, err := s.orbitRange(ctx, userID)
		if err != nil {
			t.Fatalf("orbitRange: %v", err)
		}
		if !sameInstant(got, want) {
			t.Fatalf("orbitRange = %v, want %v", got, want)
		}
		if legacy := legacyOrbitRange(t, st, userID); !sameInstant(got, legacy) {
			t.Fatalf("orbitRange = %v, 옛 질의는 %v", got, legacy)
		}
	}

	t.Run("사람이 없으면 nil", func(t *testing.T) {
		me := seedUser(t, st)
		check(t, me, nil)
	})

	t.Run("사람만 있으면 created_at", func(t *testing.T) {
		me := seedUser(t, st)
		seedPerson(t, st, me, base)
		seedPerson(t, st, me, base.AddDate(0, 1, 0))
		check(t, me, &base)
	})

	t.Run("더 앞선 교류가 있으면 occurred_at", func(t *testing.T) {
		me := seedUser(t, st)
		p := seedPerson(t, st, me, base)
		earlier := base.AddDate(-1, 0, 0)
		seedInteraction(t, st, me, p, earlier)
		seedInteraction(t, st, me, p, base.AddDate(0, 0, 1))
		check(t, me, &earlier)
	})

	t.Run("다른 사용자 것은 보지 않는다", func(t *testing.T) {
		me := seedUser(t, st)
		p := seedPerson(t, st, me, base)
		seedInteraction(t, st, me, p, base.AddDate(0, 0, 3))
		// 다른 사용자의 5~6년 전 기록이 있어도 내 첫 기록은 base 다.
		check(t, me, &base)
	})
}

// callOrbit 은 실제 핸들러 getOrbit 을 그대로 부르고 응답 JSON 을 키별 원본으로
// 돌려준다. 키가 아예 빠진 것과 null 로 들어온 것을 구별해야 하므로
// map[string]any 가 아니라 map[string]json.RawMessage 로 받는다.
func callOrbit(t *testing.T, s *Server, userID, at string) map[string]json.RawMessage {
	t.Helper()
	url := "/api/v1/orbit"
	if at != "" {
		url += "?at=" + at
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, User{ID: userID}))
	rec := httptest.NewRecorder()

	s.getOrbit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s 상태가 %d, 200이어야 한다: %s", url, rec.Code, rec.Body.String())
	}
	out := map[string]json.RawMessage{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("GET %s 응답 디코딩: %v", url, err)
	}
	return out
}

// decodeTime 은 응답의 한 키를 *time.Time 으로 읽는다. 키가 없으면 실패한다 —
// "없음"과 "null"은 호출자에게 다른 뜻이기 때문이다.
func decodeTime(t *testing.T, body map[string]json.RawMessage, key string) *time.Time {
	t.Helper()
	raw, ok := body[key]
	if !ok {
		t.Fatalf("응답에 %q 키가 없다", key)
	}
	var at *time.Time
	if err := json.Unmarshal(raw, &at); err != nil {
		t.Fatalf("%q 디코딩: %v", key, err)
	}
	return at
}

// 같은 GET /api/v1/orbit 인데 ?at= 이 붙으면 earliest_at 이 통째로 빠져 있었다.
// 화면은 if 가드로 가려 왔지만 API 키·MCP 호출자는 시간 여행 가능 구간을 알
// 방법이 없었다. 두 경로가 같은 자원을 같은 모양으로 돌려주는지 실제 핸들러를
// 두 번 불러 확인한다.
func TestOrbitAtResponseCarriesEarliestAt(t *testing.T) {
	st := openTestStore(t)
	s := &Server{store: st}
	base := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)

	t.Run("기록이 있으면 두 경로의 earliest_at 이 같다", func(t *testing.T) {
		me := seedUser(t, st)
		p := seedPerson(t, st, me, base)
		earliest := base.AddDate(-1, 0, 0)
		seedInteraction(t, st, me, p, earliest)

		now := callOrbit(t, s, me, "")
		past := callOrbit(t, s, me, base.AddDate(0, 1, 0).Format(time.RFC3339))

		gotPast := decodeTime(t, past, "earliest_at")
		if gotPast == nil || !gotPast.Equal(earliest) {
			t.Fatalf("과거 응답의 earliest_at = %v, %v 여야 한다", gotPast, earliest)
		}
		if gotNow := decodeTime(t, now, "earliest_at"); !sameInstant(gotNow, gotPast) {
			t.Fatalf("두 경로의 earliest_at 이 다르다: 현재 %v, 과거 %v", gotNow, gotPast)
		}

		var historical bool
		if err := json.Unmarshal(past["historical"], &historical); err != nil || !historical {
			t.Fatalf("과거 응답의 historical = %s (err=%v), true 여야 한다", past["historical"], err)
		}
		if _, ok := now["historical"]; ok {
			t.Fatalf("현재 응답에는 historical 이 없어야 한다: %s", now["historical"])
		}
	})

	t.Run("기록이 없으면 두 경로 모두 null", func(t *testing.T) {
		me := seedUser(t, st)

		for _, tc := range []struct {
			name string
			at   string
		}{{"현재", ""}, {"과거", base.Format(time.RFC3339)}} {
			body := callOrbit(t, s, me, tc.at)
			if got := decodeTime(t, body, "earliest_at"); got != nil {
				t.Fatalf("%s 경로의 earliest_at = %v, null 이어야 한다", tc.name, got)
			}
		}
	})

	t.Run("기존 키는 하나도 사라지지 않는다", func(t *testing.T) {
		me := seedUser(t, st)
		p := seedPerson(t, st, me, base)
		seedInteraction(t, st, me, p, base)

		past := callOrbit(t, s, me, base.AddDate(0, 1, 0).Format(time.RFC3339))
		for _, key := range []string{"center", "nodes", "contexts", "links", "categories", "generated_at", "at", "historical", "earliest_at"} {
			if _, ok := past[key]; !ok {
				t.Fatalf("과거 응답에 %q 키가 없다", key)
			}
		}
	})
}
