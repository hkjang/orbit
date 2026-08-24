package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// jsonArray는 JSON 배열을 한 항목씩 흘려보낸다.
//
// 항목 사이 쉼표를 직접 관리해야 하므로 작은 도우미로 감싼다. 전체를 메모리에
// 모았다가 한 번에 내보내면 기록이 많은 사용자의 내보내기가 서버를 압박한다.
type jsonArray struct {
	w     io.Writer
	enc   *json.Encoder
	first bool
}

func newJSONArray(w io.Writer, name string) *jsonArray {
	fmt.Fprintf(w, `,%q:[`, name)
	return &jsonArray{w: w, enc: json.NewEncoder(w), first: true}
}

func (a *jsonArray) add(value any) error {
	if !a.first {
		if _, err := fmt.Fprint(a.w, ","); err != nil {
			return err
		}
	}
	a.first = false
	return a.enc.Encode(value)
}

func (a *jsonArray) close() {
	fmt.Fprint(a.w, "]")
}

// exportData는 사용자가 자기 기록 전부를 내려받게 한다.
//
// "관계는 당신의 것"이라는 말은 가져나갈 수 있을 때만 참이다. 암호문이 아니라
// 읽을 수 있는 형태로 내보내되, 이 응답은 오직 본인에게만 간다.
//
// 사람·교류·기억을 각각 평평한 배열로 내보내고 person_id로 잇는다. 사람마다
// 하위 항목을 물려 담으면 사람 수만큼 질의가 늘어난다.
func (s *Server) exportData(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	now := time.Now()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set(
		"Content-Disposition",
		fmt.Sprintf(`attachment; filename="orbit-export-%s.json"`, now.Format("2006-01-02")),
	)

	fmt.Fprintf(w, `{"exported_at":%q,"service_version":%q,"user":`, now.Format(time.RFC3339), s.version)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id": u.ID, "username": u.Username, "email": u.Email,
		"display_name": u.DisplayName, "role": u.Role, "created_at": u.CreatedAt,
	}); err != nil {
		return
	}

	sections := []struct {
		name  string
		write func(*http.Request, *jsonArray) error
	}{
		{"people", s.exportPeople},
		{"interactions", s.exportInteractions},
		{"memories", s.exportMemories},
		{"links", s.exportLinks},
	}
	for _, section := range sections {
		array := newJSONArray(w, section.name)
		if err := section.write(r, array); err != nil {
			// 헤더는 이미 나갔으므로 상태 코드를 바꿀 수 없다. 아래 complete
			// 표시를 남기지 않는 것으로 "이 파일은 온전하지 않다"를 알린다.
			slog.Error("내보내기가 중간에 끊겼습니다", "user", u.ID, "section", section.name, "error", err)
			return
		}
		array.close()
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}

	// 중간에 끊긴 파일을 온전한 것으로 착각하지 않도록 마지막에만 표시한다.
	fmt.Fprint(w, `,"complete":true}`)
	s.audit(r.Context(), u.ID, "data.export", "user", u.ID, r.RemoteAddr, nil)
}

func (s *Server) exportPeople(r *http.Request, out *jsonArray) error {
	u := userFromContext(r.Context())
	rows, err := s.store.DB.Query(r.Context(), `SELECT p.id,p.display_name,p.company,p.role_title,p.avatar_url,p.email_cipher,p.phone_cipher,p.note_cipher,p.key_version,p.first_met,p.created_at,r.importance,r.closeness,r.momentum,r.categories,r.relationship_label,r.last_interaction_at,r.anchored FROM people p JOIN relationships r ON r.person_id=p.id WHERE p.user_id=$1 ORDER BY p.created_at`, u.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	keys := map[int][]byte{}
	for rows.Next() {
		var p Person
		var email, phone, note string
		var version int
		var categories []byte
		if err := rows.Scan(&p.ID, &p.DisplayName, &p.Company, &p.RoleTitle, &p.AvatarURL, &email, &phone, &note, &version, &p.FirstMet, &p.CreatedAt, &p.Importance, &p.Closeness, &p.Momentum, &categories, &p.RelationshipLabel, &p.LastInteractionAt, &p.Anchored); err != nil {
			return err
		}
		key, err := s.exportKey(r, keys, version)
		if err != nil {
			return err
		}
		if err := s.decryptPersonFields(key, &p, email, phone, note); err != nil {
			return err
		}
		_ = json.Unmarshal(categories, &p.Categories)
		if p.Categories == nil {
			p.Categories = []string{}
		}
		if err := out.add(p); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Server) exportInteractions(r *http.Request, out *jsonArray) error {
	u := userFromContext(r.Context())
	rows, err := s.store.DB.Query(r.Context(), `SELECT id,person_id,kind,occurred_at,weight,summary_cipher,key_version,source,created_at FROM interactions WHERE user_id=$1 ORDER BY occurred_at`, u.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	keys := map[int][]byte{}
	for rows.Next() {
		var item Interaction
		var personID, cipher string
		var version int
		if err := rows.Scan(&item.ID, &personID, &item.Kind, &item.OccurredAt, &item.Weight, &cipher, &version, &item.Source, &item.CreatedAt); err != nil {
			return err
		}
		key, err := s.exportKey(r, keys, version)
		if err != nil {
			return err
		}
		if item.Summary, err = s.store.Vault.Decrypt(key, cipher, "interaction:"+item.ID+":summary"); err != nil {
			return err
		}
		if err := out.add(map[string]any{
			"id": item.ID, "person_id": personID, "kind": item.Kind,
			"occurred_at": item.OccurredAt, "weight": item.Weight,
			"summary": item.Summary, "source": item.Source, "created_at": item.CreatedAt,
		}); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Server) exportMemories(r *http.Request, out *jsonArray) error {
	u := userFromContext(r.Context())
	rows, err := s.store.DB.Query(r.Context(), `SELECT id,person_id,title,content_cipher,key_version,occurred_at,source_type,source_reference,topics,status,created_at FROM memories WHERE user_id=$1 ORDER BY coalesce(occurred_at,created_at)`, u.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	keys := map[int][]byte{}
	for rows.Next() {
		var m Memory
		var cipher string
		var version int
		var topics []byte
		if err := rows.Scan(&m.ID, &m.PersonID, &m.Title, &cipher, &version, &m.OccurredAt, &m.SourceType, &m.SourceReference, &topics, &m.Status, &m.CreatedAt); err != nil {
			return err
		}
		key, err := s.exportKey(r, keys, version)
		if err != nil {
			return err
		}
		if m.Content, err = s.store.Vault.Decrypt(key, cipher, "memory:"+m.ID+":content"); err != nil {
			return err
		}
		_ = json.Unmarshal(topics, &m.Topics)
		if m.Topics == nil {
			m.Topics = []string{}
		}
		if err := out.add(m); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Server) exportLinks(r *http.Request, out *jsonArray) error {
	u := userFromContext(r.Context())
	rows, err := s.store.DB.Query(r.Context(), `SELECT id,person_a,person_b,kind,strength,created_at FROM person_links WHERE user_id=$1 ORDER BY created_at`, u.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, a, b, kind string
		var strength float64
		var created time.Time
		if err := rows.Scan(&id, &a, &b, &kind, &strength, &created); err != nil {
			return err
		}
		if err := out.add(map[string]any{
			"id": id, "person_a": a, "person_b": b, "kind": kind,
			"strength": strength, "created_at": created,
		}); err != nil {
			return err
		}
	}
	return rows.Err()
}

// exportKey는 키 버전별 데이터 키를 한 번만 풀어 쓴다.
func (s *Server) exportKey(r *http.Request, cache map[int][]byte, version int) ([]byte, error) {
	if key, ok := cache[version]; ok {
		return key, nil
	}
	u := userFromContext(r.Context())
	key, err := s.dataKeyVersion(r.Context(), u.ID, version)
	if err != nil {
		return nil, err
	}
	cache[version] = key
	return key, nil
}
