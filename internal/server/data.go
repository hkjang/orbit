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

func (s *Server) listPeople(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, err := s.store.DB.Query(r.Context(), `SELECT p.id,p.display_name,p.company,p.role_title,p.avatar_url,p.email_cipher,p.phone_cipher,p.note_cipher,p.key_version,p.first_met,p.created_at,r.importance,r.closeness,r.momentum,r.stable_x,r.stable_y,r.categories,r.relationship_label,r.last_interaction_at FROM people p JOIN relationships r ON r.person_id=p.id WHERE p.user_id=$1 AND ($2='' OR p.display_name ILIKE '%'||$2||'%' OR p.company ILIKE '%'||$2||'%') ORDER BY r.importance DESC,p.display_name LIMIT 1000`, u.ID, query)
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
		if err := rows.Scan(&p.ID, &p.DisplayName, &p.Company, &p.RoleTitle, &p.AvatarURL, &email, &phone, &note, &version, &p.FirstMet, &p.CreatedAt, &p.Importance, &p.Closeness, &p.Momentum, &p.StableX, &p.StableY, &categories, &p.RelationshipLabel, &p.LastInteractionAt); err != nil {
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
	writeJSON(w, 200, map[string]any{"people": people, "count": len(people)})
}

func (s *Server) getPerson(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	personID := chi.URLParam(r, "personID")
	var p Person
	var email, phone, note string
	var version int
	var categories []byte
	err := s.store.DB.QueryRow(r.Context(), `SELECT p.id,p.display_name,p.company,p.role_title,p.avatar_url,p.email_cipher,p.phone_cipher,p.note_cipher,p.key_version,p.first_met,p.created_at,r.importance,r.closeness,r.momentum,r.stable_x,r.stable_y,r.categories,r.relationship_label,r.last_interaction_at FROM people p JOIN relationships r ON r.person_id=p.id WHERE p.user_id=$1 AND p.id=$2`, u.ID, personID).Scan(&p.ID, &p.DisplayName, &p.Company, &p.RoleTitle, &p.AvatarURL, &email, &phone, &note, &version, &p.FirstMet, &p.CreatedAt, &p.Importance, &p.Closeness, &p.Momentum, &p.StableX, &p.StableY, &categories, &p.RelationshipLabel, &p.LastInteractionAt)
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

func (s *Server) recalculateRelationship(ctx context.Context, userID, personID string) error {
	rows, err := s.store.DB.Query(ctx, `SELECT occurred_at,weight FROM interactions WHERE user_id=$1 AND person_id=$2 AND occurred_at>now()-interval '365 days'`, userID, personID)
	if err != nil {
		return err
	}
	defer rows.Close()
	now := time.Now()
	var score, recent, previous float64
	var last *time.Time
	for rows.Next() {
		var at time.Time
		var weight float64
		if err := rows.Scan(&at, &weight); err != nil {
			return err
		}
		days := now.Sub(at).Hours() / 24
		value := weight * math.Exp(-.018*math.Max(0, days))
		score += value
		if days <= 45 {
			recent += weight
		} else if days <= 90 {
			previous += weight
		}
		if last == nil || at.After(*last) {
			copy := at
			last = &copy
		}
	}
	closeness := 1 - math.Exp(-score/12)
	momentum := (recent - previous) / math.Max(1, recent+previous)
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

func (s *Server) getOrbit(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	rows, err := s.store.DB.Query(r.Context(), `SELECT p.id,p.display_name,p.avatar_url,r.importance,r.closeness,r.momentum,r.stable_x,r.stable_y,r.categories,r.relationship_label,r.last_interaction_at,(SELECT count(*) FROM memories m WHERE m.user_id=p.user_id AND m.person_id=p.id AND m.status='approved') FROM people p JOIN relationships r ON r.person_id=p.id WHERE p.user_id=$1 ORDER BY r.importance DESC LIMIT 1000`, u.ID)
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
		var memories int
		if err := rows.Scan(&personID, &name, &avatar, &importance, &closeness, &momentum, &x, &y, &raw, &label, &last, &memories); err != nil {
			internalError(w, r, err)
			return
		}
		var cats []string
		_ = json.Unmarshal(raw, &cats)
		for _, c := range cats {
			contexts[c]++
		}
		nodes = append(nodes, map[string]any{"id": personID, "name": name, "avatar_url": avatar, "importance": importance, "closeness": closeness, "momentum": momentum, "x": x, "y": y, "categories": cats, "label": label, "last_interaction_at": last, "memory_count": memories})
	}
	writeJSON(w, 200, map[string]any{"center": map[string]string{"id": u.ID, "name": u.DisplayName}, "nodes": nodes, "contexts": contexts, "generated_at": time.Now()})
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
