package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hkjang/orbit/internal/id"
	"github.com/hkjang/orbit/internal/store"
	"github.com/jackc/pgx/v5"
)

type Person struct {
	ID                string     `json:"id"`
	DisplayName       string     `json:"display_name"`
	Company           string     `json:"company"`
	RoleTitle         string     `json:"role_title"`
	AvatarURL         string     `json:"avatar_url"`
	Email             string     `json:"email"`
	Phone             string     `json:"phone"`
	Note              string     `json:"note"`
	FirstMet          *time.Time `json:"first_met,omitempty"`
	Importance        float64    `json:"importance"`
	Closeness         float64    `json:"closeness"`
	Momentum          float64    `json:"momentum"`
	StableX           float64    `json:"stable_x"`
	StableY           float64    `json:"stable_y"`
	Categories        []string   `json:"categories"`
	RelationshipLabel string     `json:"relationship_label"`
	LastInteractionAt *time.Time `json:"last_interaction_at,omitempty"`
	Anchored          bool       `json:"anchored"`
	CreatedAt         time.Time  `json:"created_at"`
}

func (s *Server) activeDataKey(ctx context.Context, userID string) ([]byte, int, error) {
	var wrapped string
	var version int
	if err := s.store.DB.QueryRow(ctx, `SELECT k.wrapped_key,k.version FROM user_key_versions k JOIN key_permissions p ON p.key_version_id=k.id AND p.principal_type='owner' AND p.principal_id=k.user_id::text AND p.permissions ? 'encrypt' WHERE k.user_id=$1 AND k.status='active'`, userID).Scan(&wrapped, &version); err != nil {
		return nil, 0, err
	}
	key, err := s.store.Vault.UnwrapKey(wrapped)
	return key, version, err
}

func (s *Server) dataKeyVersion(ctx context.Context, userID string, version int) ([]byte, error) {
	var wrapped string
	if err := s.store.DB.QueryRow(ctx, `SELECT k.wrapped_key FROM user_key_versions k JOIN key_permissions p ON p.key_version_id=k.id AND p.principal_type='owner' AND p.principal_id=k.user_id::text AND p.permissions ? 'decrypt' WHERE k.user_id=$1 AND k.version=$2 AND k.status<>'revoked'`, userID, version).Scan(&wrapped); err != nil {
		return nil, err
	}
	return s.store.Vault.UnwrapKey(wrapped)
}

// escapeLike는 검색어 안의 ILIKE 와일드카드를 글자 그대로 다루게 한다.
// 이 처리가 없으면 "100%"를 찾을 때 "100"으로 시작하는 모든 것이 걸리고,
// "C_O"는 가운데 한 글자가 무엇이든 일치한다.
func escapeLike(query string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)
	return replacer.Replace(query)
}

// userCategories는 사용자가 쓰고 있는 소속 전체를 돌려준다.
//
// 소속 색은 목록 전체를 보고 배정하므로, 화면이 검색이나 필터로 일부만
// 들고 있어도 같은 소속은 같은 색이어야 한다. 그래서 화면이 가진 자료가
// 아니라 이 목록을 기준으로 삼는다.
func (s *Server) userCategories(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.store.DB.Query(ctx, `SELECT DISTINCT c.value FROM relationships r, jsonb_array_elements_text(r.categories) AS c(value) WHERE r.user_id=$1 AND jsonb_typeof(r.categories)='array' ORDER BY c.value`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if name != "" {
			out = append(out, name)
		}
	}
	return out, rows.Err()
}

func (s *Server) listPeople(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, err := s.store.DB.Query(r.Context(), `SELECT p.id,p.display_name,p.company,p.role_title,p.avatar_url,p.email_cipher,p.phone_cipher,p.note_cipher,p.key_version,p.first_met,p.created_at,r.importance,r.closeness,r.momentum,r.stable_x,r.stable_y,r.categories,r.relationship_label,r.last_interaction_at,r.anchored FROM people p JOIN relationships r ON r.person_id=p.id WHERE p.user_id=$1 AND ($2='' OR p.display_name ILIKE '%'||$2||'%' ESCAPE '\' OR p.company ILIKE '%'||$2||'%' ESCAPE '\' OR p.role_title ILIKE '%'||$2||'%' ESCAPE '\' OR r.relationship_label ILIKE '%'||$2||'%' ESCAPE '\' OR (jsonb_typeof(r.categories)='array' AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(r.categories) AS c(value) WHERE c.value ILIKE '%'||$2||'%' ESCAPE '\'))) ORDER BY r.importance DESC,p.display_name LIMIT 1000`, u.ID, escapeLike(query))
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer rows.Close()
	people := make([]Person, 0)
	keys := map[int][]byte{}
	for rows.Next() {
		var p Person
		var email, phone, note string
		var version int
		var categories []byte
		if err := rows.Scan(&p.ID, &p.DisplayName, &p.Company, &p.RoleTitle, &p.AvatarURL, &email, &phone, &note, &version, &p.FirstMet, &p.CreatedAt, &p.Importance, &p.Closeness, &p.Momentum, &p.StableX, &p.StableY, &categories, &p.RelationshipLabel, &p.LastInteractionAt, &p.Anchored); err != nil {
			internalError(w, r, err)
			return
		}
		key := keys[version]
		if key == nil {
			key, err = s.dataKeyVersion(r.Context(), u.ID, version)
			if err != nil {
				internalError(w, r, err)
				return
			}
			keys[version] = key
		}
		if err = s.decryptPersonFields(key, &p, email, phone, note); err != nil {
			internalError(w, r, err)
			return
		}
		_ = json.Unmarshal(categories, &p.Categories)
		if p.Categories == nil {
			p.Categories = []string{}
		}
		people = append(people, p)
	}
	categories, err := s.userCategories(r.Context(), u.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"people": people, "count": len(people), "categories": categories})
}

func (s *Server) getPerson(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	personID := chi.URLParam(r, "personID")
	var p Person
	var email, phone, note string
	var version int
	var categories []byte
	err := s.store.DB.QueryRow(r.Context(), `SELECT p.id,p.display_name,p.company,p.role_title,p.avatar_url,p.email_cipher,p.phone_cipher,p.note_cipher,p.key_version,p.first_met,p.created_at,r.importance,r.closeness,r.momentum,r.stable_x,r.stable_y,r.categories,r.relationship_label,r.last_interaction_at,r.anchored FROM people p JOIN relationships r ON r.person_id=p.id WHERE p.user_id=$1 AND p.id=$2`, u.ID, personID).Scan(&p.ID, &p.DisplayName, &p.Company, &p.RoleTitle, &p.AvatarURL, &email, &phone, &note, &version, &p.FirstMet, &p.CreatedAt, &p.Importance, &p.Closeness, &p.Momentum, &p.StableX, &p.StableY, &categories, &p.RelationshipLabel, &p.LastInteractionAt, &p.Anchored)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "not_found", "사람을 찾을 수 없습니다.")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	key, err := s.dataKeyVersion(r.Context(), u.ID, version)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if err = s.decryptPersonFields(key, &p, email, phone, note); err != nil {
		internalError(w, r, err)
		return
	}
	_ = json.Unmarshal(categories, &p.Categories)
	interactions, err := s.personInteractions(r.Context(), u.ID, p.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	memories, err := s.memoriesForPerson(r.Context(), u.ID, p.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"person": p, "interactions": interactions, "memories": memories})
}

func (s *Server) decryptPersonFields(key []byte, p *Person, email, phone, note string) error {
	var err error
	if p.Email, err = s.store.Vault.Decrypt(key, email, "person:"+p.ID+":email"); err != nil {
		return err
	}
	if p.Phone, err = s.store.Vault.Decrypt(key, phone, "person:"+p.ID+":phone"); err != nil {
		return err
	}
	if p.Note, err = s.store.Vault.Decrypt(key, note, "person:"+p.ID+":note"); err != nil {
		return err
	}
	return nil
}

type personInput struct {
	DisplayName       string   `json:"display_name"`
	Company           string   `json:"company"`
	RoleTitle         string   `json:"role_title"`
	AvatarURL         string   `json:"avatar_url"`
	Email             string   `json:"email"`
	Phone             string   `json:"phone"`
	Note              string   `json:"note"`
	FirstMet          string   `json:"first_met"`
	Importance        float64  `json:"importance"`
	Categories        []string `json:"categories"`
	RelationshipLabel string   `json:"relationship_label"`
}

// normalizeTags는 소속·주제처럼 사용자가 자유롭게 적는 꼬리표를 다듬는다.
//
// 앞뒤 공백을 떼고, 빈 값을 버리고, 같은 것을 한 번만 남긴다. 다듬지 않으면
// "가족"과 "가족 "이 서로 다른 소속이 되어 화면에서는 같아 보이는데 색과
// 묶음은 갈린다. 프론트엔드가 정리해 주더라도 MCP나 API 직접 호출은 무엇이든
// 보낼 수 있으므로 저장 직전인 여기서 막는다.
func normalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func validatePersonInput(in *personInput) error {
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.DisplayName == "" || len([]rune(in.DisplayName)) > 120 {
		return errors.New("이름은 1~120자로 입력해 주세요")
	}
	if in.Importance < 0 || in.Importance > 1 {
		return errors.New("중요도는 0~1 범위여야 합니다")
	}
	if in.Importance == 0 {
		in.Importance = .5
	}
	// 개수 제한은 다듬은 뒤에 센다. 중복이나 빈 값 때문에 한도에 걸리면
	// 사용자는 왜 막히는지 알 수 없다.
	in.Categories = normalizeTags(in.Categories)
	if len(in.Categories) > 12 {
		return errors.New("관계 맥락은 최대 12개까지 지정할 수 있습니다")
	}
	return nil
}

func (s *Server) createPerson(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	var in personInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := validatePersonInput(&in); err != nil {
		writeError(w, 400, "validation_error", err.Error())
		return
	}
	key, version, err := s.activeDataKey(r.Context(), u.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	personID := id.New()
	email, err := s.store.Vault.Encrypt(key, in.Email, "person:"+personID+":email")
	if err != nil {
		internalError(w, r, err)
		return
	}
	phone, err := s.store.Vault.Encrypt(key, in.Phone, "person:"+personID+":phone")
	if err != nil {
		internalError(w, r, err)
		return
	}
	note, err := s.store.Vault.Encrypt(key, in.Note, "person:"+personID+":note")
	if err != nil {
		internalError(w, r, err)
		return
	}
	var firstMet any = nil
	if in.FirstMet != "" {
		parsed, e := time.Parse("2006-01-02", in.FirstMet)
		if e != nil {
			writeError(w, 400, "validation_error", "처음 만난 날짜를 확인해 주세요.")
			return
		}
		firstMet = parsed
	}
	x, y := stablePosition(personID)
	categories, _ := json.Marshal(in.Categories)
	tx, err := s.store.DB.Begin(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `INSERT INTO people(id,user_id,display_name,company,role_title,avatar_url,email_cipher,phone_cipher,note_cipher,key_version,first_met) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, personID, u.ID, in.DisplayName, in.Company, in.RoleTitle, in.AvatarURL, email, phone, note, version, firstMet); err != nil {
		internalError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO relationships(id,user_id,person_id,importance,closeness,stable_x,stable_y,categories,relationship_label) VALUES($1,$2,$3,$4,.5,$5,$6,$7::jsonb,$8)`, id.New(), u.ID, personID, in.Importance, x, y, string(categories), in.RelationshipLabel); err != nil {
		internalError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internalError(w, r, err)
		return
	}
	s.audit(r.Context(), u.ID, "person.create", "person", personID, r.RemoteAddr, map[string]any{"display_name": in.DisplayName})
	writeJSON(w, 201, map[string]string{"id": personID})
}

func (s *Server) updatePerson(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	personID := chi.URLParam(r, "personID")
	var in personInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := validatePersonInput(&in); err != nil {
		writeError(w, 400, "validation_error", err.Error())
		return
	}
	key, version, err := s.activeDataKey(r.Context(), u.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	email, err := s.store.Vault.Encrypt(key, in.Email, "person:"+personID+":email")
	if err != nil {
		internalError(w, r, err)
		return
	}
	phone, err := s.store.Vault.Encrypt(key, in.Phone, "person:"+personID+":phone")
	if err != nil {
		internalError(w, r, err)
		return
	}
	note, err := s.store.Vault.Encrypt(key, in.Note, "person:"+personID+":note")
	if err != nil {
		internalError(w, r, err)
		return
	}
	categories, _ := json.Marshal(in.Categories)
	var firstMet any = nil
	if in.FirstMet != "" {
		parsed, e := time.Parse("2006-01-02", in.FirstMet)
		if e != nil {
			writeError(w, 400, "validation_error", "날짜를 확인해 주세요.")
			return
		}
		firstMet = parsed
	}
	tag, err := s.store.DB.Exec(r.Context(), `UPDATE people SET display_name=$3,company=$4,role_title=$5,avatar_url=$6,email_cipher=$7,phone_cipher=$8,note_cipher=$9,key_version=$10,first_met=$11,updated_at=now() WHERE user_id=$1 AND id=$2`, u.ID, personID, in.DisplayName, in.Company, in.RoleTitle, in.AvatarURL, email, phone, note, version, firstMet)
	if err == nil {
		_, err = s.store.DB.Exec(r.Context(), `UPDATE relationships SET importance=$3,categories=$4::jsonb,relationship_label=$5,updated_at=now() WHERE user_id=$1 AND person_id=$2`, u.ID, personID, in.Importance, string(categories), in.RelationshipLabel)
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "사람을 찾을 수 없습니다.")
		return
	}
	s.audit(r.Context(), u.ID, "person.update", "person", personID, r.RemoteAddr, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) deletePerson(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	personID := chi.URLParam(r, "personID")
	tag, err := s.store.DB.Exec(r.Context(), `DELETE FROM people WHERE user_id=$1 AND id=$2`, u.ID, personID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "사람을 찾을 수 없습니다.")
		return
	}
	s.audit(r.Context(), u.ID, "person.delete", "person", personID, r.RemoteAddr, nil)
	w.WriteHeader(204)
}

func stablePosition(value string) (float64, float64) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(value))
	n := h.Sum64()
	angle := float64(n%36000) / 100 * math.Pi / 180
	radius := .32 + float64((n>>16)%6200)/10000
	return math.Cos(angle) * radius, math.Sin(angle) * radius
}

type Interaction struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	OccurredAt time.Time `json:"occurred_at"`
	Weight     float64   `json:"weight"`
	Summary    string    `json:"summary"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Server) createInteraction(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	personID := chi.URLParam(r, "personID")
	var in struct {
		Kind       string    `json:"kind"`
		OccurredAt time.Time `json:"occurred_at"`
		Weight     float64   `json:"weight"`
		Summary    string    `json:"summary"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	validKinds := map[string]bool{"meeting": true, "call": true, "message": true, "note": true, "other": true}
	if !validKinds[in.Kind] {
		writeError(w, 400, "validation_error", "교류 유형을 확인해 주세요.")
		return
	}
	if in.OccurredAt.IsZero() {
		in.OccurredAt = time.Now()
	}
	if in.Weight <= 0 || in.Weight > 10 {
		in.Weight = 1
	}
	var exists bool
	if err := s.store.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM people WHERE id=$1 AND user_id=$2)`, personID, u.ID).Scan(&exists); err != nil || !exists {
		writeError(w, 404, "not_found", "사람을 찾을 수 없습니다.")
		return
	}
	key, version, err := s.activeDataKey(r.Context(), u.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	interactionID := id.New()
	summary, err := s.store.Vault.Encrypt(key, in.Summary, "interaction:"+interactionID+":summary")
	if err != nil {
		internalError(w, r, err)
		return
	}
	if _, err = s.store.DB.Exec(r.Context(), `INSERT INTO interactions(id,user_id,person_id,kind,occurred_at,weight,summary_cipher,key_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, interactionID, u.ID, personID, in.Kind, in.OccurredAt, in.Weight, summary, version); err != nil {
		internalError(w, r, err)
		return
	}
	if err = s.recalculateRelationship(r.Context(), u.ID, personID); err != nil {
		internalError(w, r, err)
		return
	}
	s.audit(r.Context(), u.ID, "interaction.create", "interaction", interactionID, r.RemoteAddr, map[string]string{"person_id": personID})
	writeJSON(w, 201, map[string]string{"id": interactionID})
}

// interactionPoint는 지표 계산에 필요한 교류 한 건이다.
type interactionPoint struct {
	At     time.Time
	Weight float64
}

// relationshipMetrics는 최근 교류에서 친밀도와 흐름을 뽑는다.
//
// 오래된 교류일수록 가치가 지수적으로 줄고, 최근 45일과 그 이전 45일의
// 무게를 견주어 관계가 다가오는지 멀어지는지를 낸다.
func relationshipMetrics(points []interactionPoint, now time.Time) (score, closeness, momentum float64) {
	var recent, previous float64
	for _, p := range points {
		days := now.Sub(p.At).Hours() / 24
		score += p.Weight * math.Exp(-.018*math.Max(0, days))
		if days <= 45 {
			recent += p.Weight
		} else if days <= 90 {
			previous += p.Weight
		}
	}
	closeness = 1 - math.Exp(-score/12)
	momentum = (recent - previous) / math.Max(1, recent+previous)
	return score, closeness, momentum
}

func (s *Server) recalculateRelationship(ctx context.Context, userID, personID string) error {
	rows, err := s.store.DB.Query(ctx, `SELECT occurred_at,weight FROM interactions WHERE user_id=$1 AND person_id=$2 AND occurred_at>now()-interval '365 days'`, userID, personID)
	if err != nil {
		return err
	}
	defer rows.Close()
	points := []interactionPoint{}
	for rows.Next() {
		var p interactionPoint
		if err := rows.Scan(&p.At, &p.Weight); err != nil {
			return err
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	score, closeness, momentum := relationshipMetrics(points, time.Now())

	// 마지막 교류 시각은 창을 두지 않고 전체에서 찾는다. 점수는 1년치만 보는
	// 것이 맞지만, 같은 창으로 마지막 시각까지 구하면 1년보다 오래된 교류만
	// 있는 사람의 시각이 비어 버린다. 그러면 궤도 문법이 그 사람을 "교류 기록
	// 없음"으로 읽어, 가장 오래 끊긴 관계를 안정 궤도로 표시하게 된다.
	var last *time.Time
	if err := s.store.DB.QueryRow(ctx, `SELECT max(occurred_at) FROM interactions WHERE user_id=$1 AND person_id=$2`, userID, personID).Scan(&last); err != nil {
		return err
	}

	_, err = s.store.DB.Exec(ctx, `UPDATE relationships SET closeness=$3,momentum=$4,interaction_score=$5,last_interaction_at=$6,updated_at=now() WHERE user_id=$1 AND person_id=$2`, userID, personID, closeness, momentum, score, last)
	return err
}

func (s *Server) personInteractions(ctx context.Context, userID, personID string) ([]Interaction, error) {
	rows, err := s.store.DB.Query(ctx, `SELECT id,kind,occurred_at,weight,summary_cipher,key_version,source,created_at FROM interactions WHERE user_id=$1 AND person_id=$2 ORDER BY occurred_at DESC LIMIT 100`, userID, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Interaction{}
	keys := map[int][]byte{}
	for rows.Next() {
		var item Interaction
		var cipher string
		var version int
		if err := rows.Scan(&item.ID, &item.Kind, &item.OccurredAt, &item.Weight, &cipher, &version, &item.Source, &item.CreatedAt); err != nil {
			return nil, err
		}
		key := keys[version]
		if key == nil {
			key, err = s.dataKeyVersion(ctx, userID, version)
			if err != nil {
				return nil, err
			}
			keys[version] = key
		}
		item.Summary, err = s.store.Vault.Decrypt(key, cipher, "interaction:"+item.ID+":summary")
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// setAnchor는 관계를 고정하거나 해제합니다.
// 전체 인물 수정(PUT)과 분리해, 한 번의 토글이 다른 필드를 덮어쓰지 않게 합니다.
func (s *Server) setAnchor(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	personID := chi.URLParam(r, "personID")
	var in struct {
		Anchored bool `json:"anchored"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	tag, err := s.store.DB.Exec(r.Context(), `UPDATE relationships SET anchored=$3,updated_at=now() WHERE user_id=$1 AND person_id=$2`, u.ID, personID, in.Anchored)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "인물을 찾을 수 없습니다.")
		return
	}
	writeJSON(w, 200, map[string]any{"anchored": in.Anchored})
}

var linkKinds = map[string]string{
	"colleague": "함께 일한 사이",
	"family":    "가족",
	"friend":    "친구",
	"community": "같은 모임",
	"knows":     "아는 사이",
}

// personLinks는 두 사람을 잇는 간선을 방향 없이 다룹니다.
// 저장은 항상 person_a < person_b로 정규화해 같은 쌍이 두 번 들어가지 않게 합니다.
func normalizeLink(a, b string) (string, string) {
	if a > b {
		return b, a
	}
	return a, b
}

func (s *Server) listPersonLinks(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	personID := chi.URLParam(r, "personID")
	rows, err := s.store.DB.Query(r.Context(), `SELECT l.id,l.kind,l.strength,p.id,p.display_name,p.company,p.role_title FROM person_links l JOIN people p ON p.id = CASE WHEN l.person_a=$2 THEN l.person_b ELSE l.person_a END WHERE l.user_id=$1 AND $2 IN (l.person_a,l.person_b) ORDER BY l.strength DESC,p.display_name`, u.ID, personID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer rows.Close()
	links := make([]map[string]any, 0)
	for rows.Next() {
		var linkID, kind, otherID, otherName, company, roleTitle string
		var strength float64
		if err := rows.Scan(&linkID, &kind, &strength, &otherID, &otherName, &company, &roleTitle); err != nil {
			internalError(w, r, err)
			return
		}
		links = append(links, map[string]any{"id": linkID, "kind": kind, "kind_label": linkKinds[kind], "strength": strength, "person_id": otherID, "person_name": otherName, "company": company, "role_title": roleTitle})
	}
	if err := rows.Err(); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"links": links, "count": len(links)})
}

func (s *Server) createPersonLink(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	personID := chi.URLParam(r, "personID")
	var in struct {
		PersonID string  `json:"person_id"`
		Kind     string  `json:"kind"`
		Strength float64 `json:"strength"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Kind == "" {
		in.Kind = "knows"
	}
	if _, ok := linkKinds[in.Kind]; !ok {
		writeError(w, 400, "validation_error", "관계 유형을 확인해 주세요.")
		return
	}
	if in.PersonID == personID {
		writeError(w, 400, "validation_error", "같은 사람끼리는 이을 수 없습니다.")
		return
	}
	if in.Strength <= 0 || in.Strength > 1 {
		in.Strength = .5
	}
	a, b := normalizeLink(personID, in.PersonID)
	var owned int
	if err := s.store.DB.QueryRow(r.Context(), `SELECT count(*) FROM people WHERE user_id=$1 AND id IN ($2,$3)`, u.ID, a, b).Scan(&owned); err != nil {
		internalError(w, r, err)
		return
	}
	if owned != 2 {
		writeError(w, 404, "not_found", "사람을 찾을 수 없습니다.")
		return
	}
	linkID := id.New()
	if err := s.store.DB.QueryRow(r.Context(), `INSERT INTO person_links(id,user_id,person_a,person_b,kind,strength) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (user_id,person_a,person_b) DO UPDATE SET kind=EXCLUDED.kind,strength=EXCLUDED.strength,updated_at=now() RETURNING id`, linkID, u.ID, a, b, in.Kind, in.Strength).Scan(&linkID); err != nil {
		internalError(w, r, err)
		return
	}
	s.audit(r.Context(), u.ID, "person_link.upsert", "person_link", linkID, r.RemoteAddr, map[string]string{"kind": in.Kind})
	writeJSON(w, 201, map[string]any{"id": linkID, "kind": in.Kind, "strength": in.Strength})
}

func (s *Server) deletePersonLink(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	linkID := chi.URLParam(r, "linkID")
	tag, err := s.store.DB.Exec(r.Context(), `DELETE FROM person_links WHERE user_id=$1 AND id=$2`, u.ID, linkID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "연결을 찾을 수 없습니다.")
		return
	}
	s.audit(r.Context(), u.ID, "person_link.delete", "person_link", linkID, r.RemoteAddr, nil)
	w.WriteHeader(204)
}

// orbitLinks는 사람 사이 연결을 읽는다. 현재 화면과 과거 화면이 함께 쓴다.
func (s *Server) orbitLinks(ctx context.Context, userID string) ([]map[string]any, error) {
	rows, err := s.store.DB.Query(ctx, `SELECT person_a,person_b,kind,strength FROM person_links WHERE user_id=$1 LIMIT 5000`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := make([]map[string]any, 0)
	for rows.Next() {
		var a, b, kind string
		var strength float64
		if err := rows.Scan(&a, &b, &kind, &strength); err != nil {
			return nil, err
		}
		links = append(links, map[string]any{"a": a, "b": b, "kind": kind, "strength": strength})
	}
	return links, rows.Err()
}

func (s *Server) getOrbit(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	// 과거 어느 날을 물으면 그 시점 기준으로 다시 계산해 돌려준다.
	at, historical, err := parseOrbitAt(r.URL.Query().Get("at"))
	if err != nil {
		writeError(w, 400, "validation_error", err.Error())
		return
	}
	if historical {
		s.writeOrbitAt(w, r, at)
		return
	}
	rows, err := s.store.DB.Query(r.Context(), `SELECT p.id,p.display_name,p.avatar_url,r.importance,r.closeness,r.momentum,r.stable_x,r.stable_y,r.categories,r.relationship_label,r.last_interaction_at,r.anchored,(SELECT count(*) FROM memories m WHERE m.user_id=p.user_id AND m.person_id=p.id AND m.status='approved') FROM people p JOIN relationships r ON r.person_id=p.id WHERE p.user_id=$1 ORDER BY r.importance DESC LIMIT 1000`, u.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer rows.Close()
	nodes := make([]map[string]any, 0)
	contexts := map[string]int{}
	for rows.Next() {
		var personID, name, avatar, label string
		var importance, closeness, momentum, x, y float64
		var raw []byte
		var last *time.Time
		var anchored bool
		var memories int
		if err := rows.Scan(&personID, &name, &avatar, &importance, &closeness, &momentum, &x, &y, &raw, &label, &last, &anchored, &memories); err != nil {
			internalError(w, r, err)
			return
		}
		var cats []string
		_ = json.Unmarshal(raw, &cats)
		for _, c := range cats {
			contexts[c]++
		}
		nodes = append(nodes, map[string]any{"id": personID, "name": name, "avatar_url": avatar, "importance": importance, "closeness": closeness, "momentum": momentum, "x": x, "y": y, "categories": cats, "label": label, "last_interaction_at": last, "anchored": anchored, "memory_count": memories})
	}
	if err := rows.Err(); err != nil {
		internalError(w, r, err)
		return
	}
	links, err := s.orbitLinks(r.Context(), u.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	categories, err := s.userCategories(r.Context(), u.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	first, err := s.orbitRange(r.Context(), u.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"center": map[string]string{"id": u.ID, "name": u.DisplayName}, "nodes": nodes, "contexts": contexts, "links": links, "categories": categories, "earliest_at": first, "generated_at": time.Now()})
}

func (s *Server) rediscover(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	var personID, name, title string
	var occurred *time.Time
	err := s.store.DB.QueryRow(r.Context(), `SELECT p.id,p.display_name,m.title,m.occurred_at FROM memories m JOIN people p ON p.id=m.person_id WHERE m.user_id=$1 AND m.status='approved' ORDER BY (extract(doy from coalesce(m.occurred_at,m.created_at))::int-extract(doy from now())::int+366)%366,random() LIMIT 1`, u.ID).Scan(&personID, &name, &title, &occurred)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, 200, map[string]any{"item": nil})
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"item": map[string]any{"person_id": personID, "person_name": name, "title": title, "occurred_at": occurred}})
}

var _ = fmt.Sprintf
var _ = store.ErrNotFound
