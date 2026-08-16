package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hkjang/orbit/internal/id"
	"github.com/jackc/pgx/v5"
)

type Memory struct {
	ID              string     `json:"id"`
	PersonID        *string    `json:"person_id,omitempty"`
	PersonName      string     `json:"person_name,omitempty"`
	Title           string     `json:"title"`
	Content         string     `json:"content"`
	OccurredAt      *time.Time `json:"occurred_at,omitempty"`
	SourceType      string     `json:"source_type"`
	SourceReference string     `json:"source_reference"`
	Topics          []string   `json:"topics"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
}

func (s *Server) approvalSettings(ctx context.Context) (ApprovalSettings, error) {
	var settings ApprovalSettings
	err := s.readSetting(ctx, "workflow", "approval", &settings, nil)
	return settings, err
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func (s *Server) createMemory(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	var in struct {
		PersonID        string     `json:"person_id"`
		Title           string     `json:"title"`
		Content         string     `json:"content"`
		OccurredAt      *time.Time `json:"occurred_at"`
		SourceType      string     `json:"source_type"`
		SourceReference string     `json:"source_reference"`
		Topics          []string   `json:"topics"`
		RequestNote     string     `json:"request_note"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Content = strings.TrimSpace(in.Content)
	if in.Title == "" || in.Content == "" {
		writeError(w, 400, "validation_error", "기억의 제목과 내용을 입력해 주세요.")
		return
	}
	if len(in.Topics) > 20 {
		writeError(w, 400, "validation_error", "주제는 최대 20개까지 입력할 수 있습니다.")
		return
	}
	if in.SourceType == "" {
		in.SourceType = "manual"
	}
	if in.PersonID != "" {
		var exists bool
		if err := s.store.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM people WHERE id=$1 AND user_id=$2)`, in.PersonID, u.ID).Scan(&exists); err != nil || !exists {
			writeError(w, 400, "invalid_person", "관계 인물을 확인해 주세요.")
			return
		}
	}
	key, version, err := s.activeDataKey(r.Context(), u.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	memoryID := id.New()
	cipher, err := s.store.Vault.Encrypt(key, in.Content, "memory:"+memoryID+":content")
	if err != nil {
		internalError(w, r, err)
		return
	}
	topics, _ := json.Marshal(in.Topics)
	workflow, err := s.approvalSettings(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	status := "approved"
	requiresApproval := workflow.Enabled && contains(workflow.ResourceTypes, "memory")
	if requiresApproval {
		status = "pending"
	}
	var personID any = nil
	if in.PersonID != "" {
		personID = in.PersonID
	}
	tx, err := s.store.DB.Begin(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `INSERT INTO memories(id,user_id,person_id,title,content_cipher,key_version,occurred_at,source_type,source_reference,topics,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11)`, memoryID, u.ID, personID, in.Title, cipher, version, in.OccurredAt, in.SourceType, in.SourceReference, string(topics), status); err != nil {
		internalError(w, r, err)
		return
	}
	if requiresApproval {
		if _, err = tx.Exec(r.Context(), `INSERT INTO approval_requests(id,requester_id,resource_type,resource_id,action,request_note) VALUES($1,$2,'memory',$3,'create',$4)`, id.New(), u.ID, memoryID, in.RequestNote); err != nil {
			internalError(w, r, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		internalError(w, r, err)
		return
	}
	s.audit(r.Context(), u.ID, "memory.create", "memory", memoryID, r.RemoteAddr, map[string]any{"status": status})
	writeJSON(w, 201, map[string]any{"id": memoryID, "status": status, "approval_required": requiresApproval})
}

func (s *Server) listMemories(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	personID := r.URL.Query().Get("person_id")
	status := r.URL.Query().Get("status")
	memories, err := s.queryMemories(r.Context(), u.ID, personID, status)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"memories": memories, "count": len(memories)})
}

func (s *Server) queryMemories(ctx context.Context, userID, personID, status string) ([]Memory, error) {
	rows, err := s.store.DB.Query(ctx, `SELECT m.id,m.person_id,coalesce(p.display_name,''),m.title,m.content_cipher,m.key_version,m.occurred_at,m.source_type,m.source_reference,m.topics,m.status,m.created_at FROM memories m LEFT JOIN people p ON p.id=m.person_id WHERE m.user_id=$1 AND ($2='' OR m.person_id=NULLIF($2,'')::uuid) AND ($3='' OR m.status=$3) ORDER BY coalesce(m.occurred_at,m.created_at) DESC LIMIT 500`, userID, personID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Memory{}
	keys := map[int][]byte{}
	for rows.Next() {
		var m Memory
		var cipher string
		var version int
		var topics []byte
		if err := rows.Scan(&m.ID, &m.PersonID, &m.PersonName, &m.Title, &cipher, &version, &m.OccurredAt, &m.SourceType, &m.SourceReference, &topics, &m.Status, &m.CreatedAt); err != nil {
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
		m.Content, err = s.store.Vault.Decrypt(key, cipher, "memory:"+m.ID+":content")
		if err != nil {
			return nil, err
		}
		_ = json.Unmarshal(topics, &m.Topics)
		if m.Topics == nil {
			m.Topics = []string{}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Server) memoriesForPerson(ctx context.Context, userID, personID string) ([]Memory, error) {
	return s.queryMemories(ctx, userID, personID, "")
}

type Approval struct {
	ID            string     `json:"id"`
	RequesterID   string     `json:"requester_id"`
	RequesterName string     `json:"requester_name"`
	ReviewerID    *string    `json:"reviewer_id,omitempty"`
	ResourceType  string     `json:"resource_type"`
	ResourceID    string     `json:"resource_id"`
	Action        string     `json:"action"`
	Status        string     `json:"status"`
	RequestNote   string     `json:"request_note"`
	ReviewNote    string     `json:"review_note"`
	CreatedAt     time.Time  `json:"created_at"`
	ReviewedAt    *time.Time `json:"reviewed_at,omitempty"`
	ResourceTitle string     `json:"resource_title"`
}

func (s *Server) listApprovals(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	workflow, err := s.approvalSettings(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !workflow.Enabled {
		writeJSON(w, 200, map[string]any{"enabled": false, "can_review": false, "approvals": []Approval{}})
		return
	}
	status := r.URL.Query().Get("status")
	canReview := u.Role == "admin" || (workflow.ReviewerRole == "team_lead" && u.Role == "team_lead")
	rows, err := s.store.DB.Query(r.Context(), `SELECT a.id,a.requester_id,u.display_name,a.reviewer_id,a.resource_type,a.resource_id,a.action,a.status,a.request_note,a.review_note,a.created_at,a.reviewed_at,coalesce(m.title,'') FROM approval_requests a JOIN users u ON u.id=a.requester_id LEFT JOIN memories m ON a.resource_type='memory' AND m.id=a.resource_id WHERE ($1='' OR a.status=$1) AND ($2 OR a.requester_id=$3) ORDER BY a.created_at DESC LIMIT 300`, status, canReview, u.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer rows.Close()
	items := []Approval{}
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ID, &a.RequesterID, &a.RequesterName, &a.ReviewerID, &a.ResourceType, &a.ResourceID, &a.Action, &a.Status, &a.RequestNote, &a.ReviewNote, &a.CreatedAt, &a.ReviewedAt, &a.ResourceTitle); err != nil {
			internalError(w, r, err)
			return
		}
		items = append(items, a)
	}
	writeJSON(w, 200, map[string]any{"enabled": true, "can_review": canReview, "approvals": items})
}

func (s *Server) reviewApproval(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	workflow, settingsErr := s.approvalSettings(r.Context())
	if settingsErr != nil {
		internalError(w, r, settingsErr)
		return
	}
	if !workflow.Enabled || (u.Role != "admin" && !(workflow.ReviewerRole == "team_lead" && u.Role == "team_lead")) {
		writeError(w, 403, "forbidden", "검토 권한이 없습니다.")
		return
	}
	approvalID := chi.URLParam(r, "approvalID")
	var in struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Decision != "approved" && in.Decision != "rejected" {
		writeError(w, 400, "validation_error", "승인 또는 반려를 선택해 주세요.")
		return
	}
	tx, err := s.store.DB.Begin(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var resourceType, resourceID, status string
	err = tx.QueryRow(r.Context(), `SELECT resource_type,resource_id,status FROM approval_requests WHERE id=$1 FOR UPDATE`, approvalID).Scan(&resourceType, &resourceID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "not_found", "검토 요청을 찾을 수 없습니다.")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	if status != "pending" {
		writeError(w, 409, "already_reviewed", "이미 처리된 요청입니다.")
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE approval_requests SET status=$2,reviewer_id=$3,review_note=$4,reviewed_at=now() WHERE id=$1`, approvalID, in.Decision, u.ID, in.Note); err != nil {
		internalError(w, r, err)
		return
	}
	if resourceType == "memory" {
		if _, err = tx.Exec(r.Context(), `UPDATE memories SET status=$2,updated_at=now() WHERE id=$1`, resourceID, in.Decision); err != nil {
			internalError(w, r, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		internalError(w, r, err)
		return
	}
	s.audit(r.Context(), u.ID, "approval."+in.Decision, "approval", approvalID, r.RemoteAddr, map[string]string{"resource_id": resourceID})
	writeJSON(w, 200, map[string]bool{"ok": true})
}
