package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Time Travel — 과거 어느 날의 우주를 다시 세운다.
//
// 이력 테이블이 필요할 것 같지만 그렇지 않다. 친밀도와 흐름은 저장된 사실이
// 아니라 교류에서 계산해 낸 값이고, 교류에는 일어난 시각이 남아 있다. 그래서
// 계산의 기준 시각만 과거로 옮기면 그날의 상태가 그대로 나온다.
//
// 다만 정직하게 말해 둘 것이 있다. 중요도·소속·고정 여부는 사용자가 직접
// 정하는 값이라 변경 이력이 없다. 그 세 가지는 오늘의 값을 쓴다. 되살아나는
// 것은 "교류가 만든 거리와 흐름"이다.

// parseOrbitAt은 ?at= 값을 읽는다. 없으면 현재 시점을 뜻한다.
func parseOrbitAt(raw string) (time.Time, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if at, err := time.Parse(layout, raw); err == nil {
			return at, true, nil
		}
	}
	return time.Time{}, false, errInvalidOrbitAt
}

type orbitAtError struct{}

func (orbitAtError) Error() string { return "날짜 형식을 확인해 주세요 (예: 2025-06-01)" }

var errInvalidOrbitAt = orbitAtError{}

// historicalNode는 과거 시점 계산에 필요한 사람 정보다.
type historicalNode struct {
	ID          string
	Name        string
	Avatar      string
	Importance  float64
	StableX     float64
	StableY     float64
	Categories  []string
	Label       string
	Anchored    bool
	MemoryCount int
}

// orbitAt은 주어진 시각 기준으로 노드를 다시 계산한다.
func (s *Server) orbitAt(ctx context.Context, userID string, at time.Time) ([]map[string]any, map[string]int, error) {
	// 그날 내 우주에 있었던 사람만 세운다.
	//
	// 판정 기준을 people.created_at으로만 두면 안 된다. 그건 관계가 시작된
	// 시점이 아니라 Orbit에 적어 넣은 시점이라, 오늘 등록하며 과거 교류를
	// 함께 기록한 사람이 통째로 사라진다. 그날 이전의 교류나 첫 만남 기록이
	// 있으면 그때 이미 곁에 있던 사람으로 본다.
	rows, err := s.store.DB.Query(ctx, `SELECT p.id,p.display_name,p.avatar_url,r.importance,r.stable_x,r.stable_y,r.categories,r.relationship_label,r.anchored,(SELECT count(*) FROM memories m WHERE m.user_id=p.user_id AND m.person_id=p.id AND m.status='approved' AND coalesce(m.occurred_at,m.created_at)<=$2) FROM people p JOIN relationships r ON r.person_id=p.id WHERE p.user_id=$1 AND (p.created_at<=$2 OR (p.first_met IS NOT NULL AND p.first_met<=$2::date) OR EXISTS (SELECT 1 FROM interactions i WHERE i.user_id=p.user_id AND i.person_id=p.id AND i.occurred_at<=$2)) ORDER BY r.importance DESC LIMIT 1000`, userID, at)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	people := []historicalNode{}
	contexts := map[string]int{}
	for rows.Next() {
		var n historicalNode
		var raw []byte
		if err := rows.Scan(&n.ID, &n.Name, &n.Avatar, &n.Importance, &n.StableX, &n.StableY, &raw, &n.Label, &n.Anchored, &n.MemoryCount); err != nil {
			return nil, nil, err
		}
		_ = json.Unmarshal(raw, &n.Categories)
		if n.Categories == nil {
			n.Categories = []string{}
		}
		for _, c := range n.Categories {
			contexts[c]++
		}
		people = append(people, n)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	// 그 시점의 지표를 내려면 그때까지의 교류만 본다. 사람마다 따로 묻지 않고
	// 한 번에 받아 묶는다.
	points := map[string][]interactionPoint{}
	last := map[string]time.Time{}
	interactionRows, err := s.store.DB.Query(ctx, `SELECT person_id,occurred_at,weight FROM interactions WHERE user_id=$1 AND occurred_at<=$2`, userID, at)
	if err != nil {
		return nil, nil, err
	}
	defer interactionRows.Close()
	window := at.AddDate(-1, 0, 0)
	for interactionRows.Next() {
		var personID string
		var p interactionPoint
		if err := interactionRows.Scan(&personID, &p.At, &p.Weight); err != nil {
			return nil, nil, err
		}
		// 마지막 접촉은 창을 두지 않는다. 점수만 1년치를 본다.
		if known, ok := last[personID]; !ok || p.At.After(known) {
			last[personID] = p.At
		}
		if p.At.After(window) {
			points[personID] = append(points[personID], p)
		}
	}
	if err := interactionRows.Err(); err != nil {
		return nil, nil, err
	}

	nodes := make([]map[string]any, 0, len(people))
	for _, n := range people {
		_, closeness, momentum := relationshipMetrics(points[n.ID], at)
		node := map[string]any{
			"id": n.ID, "name": n.Name, "avatar_url": n.Avatar,
			"importance": n.Importance, "closeness": closeness, "momentum": momentum,
			"x": n.StableX, "y": n.StableY, "categories": n.Categories,
			"label": n.Label, "anchored": n.Anchored, "memory_count": n.MemoryCount,
		}
		if when, ok := last[n.ID]; ok {
			node["last_interaction_at"] = when
		} else {
			node["last_interaction_at"] = nil
		}
		nodes = append(nodes, node)
	}
	return nodes, contexts, nil
}

// orbitRange는 시간 여행이 가능한 구간을 알려준다.
// 시작점은 첫 기록이고, 그 이전에는 보여 줄 우주가 없다.
//
// 두 테이블을 따로 훑는다 — 조인하면 사람×교류 곱이 된다. 사람이 한 명도
// 없으면 교류도 없으므로(interactions.person_id 가 people 을 참조) NULL 이다.
func (s *Server) orbitRange(ctx context.Context, userID string) (*time.Time, error) {
	var first *time.Time
	err := s.store.DB.QueryRow(ctx, `SELECT least((SELECT min(created_at) FROM people WHERE user_id=$1),(SELECT min(occurred_at) FROM interactions WHERE user_id=$1))`, userID).Scan(&first)
	return first, err
}

func (s *Server) writeOrbitAt(w http.ResponseWriter, r *http.Request, at time.Time) {
	u := userFromContext(r.Context())
	nodes, contexts, err := s.orbitAt(r.Context(), u.ID, at)
	if err != nil {
		internalError(w, r, err)
		return
	}
	all, err := s.orbitLinks(r.Context(), u.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	// 사람↔사람 연결은 사용자가 직접 정하는 값이라 변경 이력이 없다 — 위에 적은
	// 대로 그런 값은 오늘의 값을 쓴다. 그래서 링크에 시점 필터를 걸지는 않지만,
	// 그날 곁에 없던 사람을 가리키는 끝점은 이 응답 안에서 가리킬 상대가 없어
	// 끊긴 참조가 된다. 화면은 끝점 없는 링크를 건너뛰어 견디지만 ?at= 만 부르는
	// 호출자에게는 응답 하나로 그래프가 닫혀 있어야 한다. 그래서 두 끝점이 모두
	// nodes 에 있는 링크만 남긴다. 거르는 자리가 orbitLinks 가 아니라 여기인
	// 이유는 그 함수를 현재 시점 경로(getOrbit)와 함께 쓰기 때문이다.
	present := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		if nodeID, ok := n["id"].(string); ok {
			present[nodeID] = true
		}
	}
	links := make([]map[string]any, 0, len(all))
	for _, l := range all {
		a, aok := l["a"].(string)
		b, bok := l["b"].(string)
		if aok && bok && present[a] && present[b] {
			links = append(links, l)
		}
	}
	categories, err := s.userCategories(r.Context(), u.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	// 현재 시점 응답(getOrbit)과 같은 키를 같은 뜻으로 담는다. 화면은 처음
	// 열 때 받은 값을 들고 있어 없어도 버티지만, ?at= 만 부르는 API 키·MCP
	// 호출자는 이 값이 없으면 시간 여행이 어디까지 가능한지 알 길이 없다.
	first, err := s.orbitRange(r.Context(), u.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"center":      map[string]string{"id": u.ID, "name": u.DisplayName},
		"nodes":       nodes,
		"contexts":    contexts,
		"links":       links,
		"categories":  categories,
		"earliest_at": first,
		"at":          at,
		// 과거 화면임을 화면이 분명히 알 수 있게 표시한다.
		"historical":   true,
		"generated_at": time.Now(),
	})
}
