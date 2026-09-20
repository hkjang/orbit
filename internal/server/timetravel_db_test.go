package server

import (
	"context"
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
