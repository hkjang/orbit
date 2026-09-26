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

// seedRelationship 은 relationships 행을 넣는다. orbitAt 이 people 을
// JOIN relationships 로 묻기 때문에 이 행이 없는 사람은 응답에 아예 나오지
// 않는다 — 노드를 기대하는 시험에서는 사람과 항상 짝지어 넣어야 한다.
func seedRelationship(t *testing.T, st *store.Store, userID, personID string) {
	t.Helper()
	if _, err := st.DB.Exec(context.Background(), `INSERT INTO relationships (id,user_id,person_id) VALUES ($1,$2,$3)`, id.New(), userID, personID); err != nil {
		t.Fatalf("seed relationship: %v", err)
	}
}

// seedPersonLink 는 사람↔사람 간선을 넣는다. person_links 에 CHECK (person_a <
// person_b) 가 걸려 있으므로 프로덕션과 같은 normalizeLink 로 정규화해 넣고,
// 정규화된 쌍을 그대로 돌려준다 — 응답의 a/b 와 비교할 값이 이것이다.
func seedPersonLink(t *testing.T, st *store.Store, userID, personA, personB string) (string, string) {
	t.Helper()
	a, b := normalizeLink(personA, personB)
	if _, err := st.DB.Exec(context.Background(), `INSERT INTO person_links (id,user_id,person_a,person_b) VALUES ($1,$2,$3,$4)`, id.New(), userID, a, b); err != nil {
		t.Fatalf("seed person link: %v", err)
	}
	return a, b
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

// decodeObjects 는 응답의 한 키를 객체 배열로 읽는다. 전용 struct 가 아니라
// map 으로 받는 이유는 키가 늘거나 줄은 것까지 보기 위해서다. null 은 실패다 —
// links·nodes 는 빈 경우에도 JSON 배열로 나가는 계약이다.
func decodeObjects(t *testing.T, body map[string]json.RawMessage, key string) []map[string]any {
	t.Helper()
	raw, ok := body[key]
	if !ok {
		t.Fatalf("응답에 %q 키가 없다", key)
	}
	if string(raw) == "null" {
		t.Fatalf("%q 가 null 이다 — 빈 경우에도 배열이어야 한다", key)
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%q 디코딩: %v", key, err)
	}
	return out
}

// linkPairs 는 links 의 끝점 쌍만 뽑아 집합으로 만든다. 질의 순서에 기대지 않고
// "어떤 간선이 남았는가"만 비교하기 위해서다.
func linkPairs(t *testing.T, links []map[string]any) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, l := range links {
		a, aok := l["a"].(string)
		b, bok := l["b"].(string)
		if !aok || !bok {
			t.Fatalf("링크에 a/b 문자열이 없다: %v", l)
		}
		out[a+"|"+b] = true
	}
	return out
}

// 과거 응답(?at=)의 links 가 그날 없던 사람을 가리키면 그 응답만으로는 그래프를
// 그릴 수 없다. nodes 는 orbitAt 의 포함 규칙으로 걸러지는데 links 는 오늘의
// 전체 목록이 그대로 실려 왔기 때문이다. 웹 캔버스는 끝점 없는 링크를 건너뛰어
// 우연히 견디지만 API 키로 ?at= 만 부르는 호출자는 끊긴 참조를 받는다.
func TestOrbitAtLinksStayWithinNodes(t *testing.T) {
	st := openTestStore(t)
	s := &Server{store: st}
	base := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	at := base.AddDate(0, 1, 0)

	me := seedUser(t, st)
	// A·B 는 그 시점 전부터 있었고 C 는 그 뒤에 생겼다. 포함 규칙은 세 갈래라
	// (created_at·first_met·그 이전 교류) 하나만 걸려도 들어오므로 C 에는
	// first_met 도 교류도 두지 않는다.
	personA := seedPerson(t, st, me, base)
	personB := seedPerson(t, st, me, base)
	personC := seedPerson(t, st, me, at.AddDate(0, 1, 0))
	for _, p := range []string{personA, personB, personC} {
		seedRelationship(t, st, me, p)
	}
	keptA, keptB := seedPersonLink(t, st, me, personA, personB)
	brokenA, brokenB := seedPersonLink(t, st, me, personB, personC)
	kept := keptA + "|" + keptB
	broken := brokenA + "|" + brokenB

	t.Run("과거 응답의 링크는 모두 그 응답의 nodes 안에서 닫힌다", func(t *testing.T) {
		past := callOrbit(t, s, me, at.Format(time.RFC3339))

		ids := map[string]bool{}
		for _, n := range decodeObjects(t, past, "nodes") {
			nodeID, ok := n["id"].(string)
			if !ok {
				t.Fatalf("노드에 id 문자열이 없다: %v", n)
			}
			ids[nodeID] = true
		}
		if !ids[personA] || !ids[personB] {
			t.Fatalf("A·B 는 그 시점에 있었으므로 nodes 에 있어야 한다: %v", ids)
		}
		if ids[personC] {
			t.Fatalf("C 는 그 시점 뒤에 생겼으므로 nodes 에 없어야 한다: %v", ids)
		}

		links := decodeObjects(t, past, "links")
		pairs := linkPairs(t, links)
		if !pairs[kept] {
			t.Fatalf("둘 다 그때 있던 A-B 링크는 남아야 한다: %v", pairs)
		}
		if pairs[broken] {
			t.Fatalf("그 시점 뒤에 생긴 C 와 이어진 링크는 빠져야 한다: %v", pairs)
		}
		for _, l := range links {
			if !ids[l["a"].(string)] || !ids[l["b"].(string)] {
				t.Fatalf("링크 %v 의 끝점이 nodes 밖을 가리킨다: %v", l, ids)
			}
			for _, key := range []string{"a", "b", "kind", "strength"} {
				if _, ok := l[key]; !ok {
					t.Fatalf("링크에 %q 키가 없다: %v", key, l)
				}
			}
			if len(l) != 4 {
				t.Fatalf("링크의 키가 a/b/kind/strength 넷이어야 한다: %v", l)
			}
		}
	})

	t.Run("현재 응답은 오늘의 링크를 전부 담는다", func(t *testing.T) {
		now := callOrbit(t, s, me, "")
		pairs := linkPairs(t, decodeObjects(t, now, "links"))
		if !pairs[kept] || !pairs[broken] {
			t.Fatalf("현재 경로는 링크 둘 다 담아야 한다: %v", pairs)
		}
	})

	t.Run("링크가 하나도 안 남아도 배열로 나간다", func(t *testing.T) {
		// A·B 보다 앞선 시점이면 두 링크 모두 끝점을 잃는다.
		past := callOrbit(t, s, me, base.AddDate(-1, 0, 0).Format(time.RFC3339))
		if links := decodeObjects(t, past, "links"); len(links) != 0 {
			t.Fatalf("그때는 아무도 없었으므로 링크가 비어야 한다: %v", links)
		}
	})
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
