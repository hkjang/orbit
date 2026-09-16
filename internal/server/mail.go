package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/hkjang/orbit/internal/mail"
)

// mailConfig 는 저장된 설정과 따로 보관한 비밀번호로 발송 구성을 만든다.
// 알림마다 읽으므로 관리자가 바꾼 값이 재시작 없이 다음 알림부터 적용된다.
func (s *Server) mailConfig(ctx context.Context) (mail.Config, error) {
	settings := mail.DefaultSettings()
	var password string
	if err := s.readSetting(ctx, "mail", "smtp", &settings, &password); err != nil {
		return mail.Config{}, err
	}
	return settings.Config(password), nil
}

// lookupEmails 는 계정 id 를 메일 주소로 바꾼다. 사용자 표는 이미 있으므로
// 메일 기능은 이 조회 하나만 빌려 쓴다. 비활성 계정과 주소 없는 계정은 빠진다.
func (s *Server) lookupEmails(ctx context.Context, userIDs []string) (map[string]string, error) {
	rows, err := s.store.DB.Query(ctx, `SELECT id::text,email FROM users WHERE id::text = ANY($1::text[]) AND status='active' AND email<>''`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	emails := map[string]string{}
	for rows.Next() {
		var userID, email string
		if err := rows.Scan(&userID, &email); err != nil {
			return nil, err
		}
		emails[userID] = email
	}
	return emails, rows.Err()
}

// reviewerIDs 는 검토 요청을 받아 볼 사람이다: 관리자 전원과, 검토자 역할이
// 팀장이면 팀장 전원.
func (s *Server) reviewerIDs(ctx context.Context, reviewerRole string) ([]string, error) {
	rows, err := s.store.DB.Query(ctx, `SELECT id::text FROM users WHERE status='active' AND (role='admin' OR (role='team_lead' AND $1)) ORDER BY created_at`, reviewerRole == "team_lead")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		ids = append(ids, userID)
	}
	return ids, rows.Err()
}

// notifyApprovalRequested 는 검토자에게 기억이 검토를 기다린다고 알린다.
// 실패해도 요청은 이미 성공했으므로 조용히 로그만 남긴다.
func (s *Server) notifyApprovalRequested(ctx context.Context, requester User, memoryID, title, note string, workflow ApprovalSettings) {
	if s.mail == nil {
		return
	}
	reviewers, err := s.reviewerIDs(ctx, workflow.ReviewerRole)
	if err != nil || len(reviewers) == 0 {
		return
	}
	var pending int
	_ = s.store.DB.QueryRow(ctx, `SELECT count(*) FROM approval_requests WHERE status='pending'`).Scan(&pending)
	notification := mail.ApprovalRequested(requester.DisplayName, title, note, pending)
	notification.ResourceID = memoryID
	s.mail.Notify(ctx, notification, requester.ID, reviewers)
}

// notifyApprovalDecided 는 요청자에게 결과를 알린다. 검토자가 자기 요청을
// 처리했다면 행위자 제외 규칙으로 보내지 않는다.
func (s *Server) notifyApprovalDecided(ctx context.Context, reviewer User, requesterID, memoryID, decision, note string) {
	if s.mail == nil {
		return
	}
	var title string
	if err := s.store.DB.QueryRow(ctx, `SELECT title FROM memories WHERE id=$1`, memoryID).Scan(&title); err != nil {
		return
	}
	notification := mail.ApprovalDecided(reviewer.DisplayName, title, decision, note)
	notification.ResourceID = memoryID
	s.mail.Notify(ctx, notification, reviewer.ID, []string{requesterID})
}

// notifyAccountCreated 는 관리자가 만든 계정의 주인에게 준비되었다고 알린다.
func (s *Server) notifyAccountCreated(ctx context.Context, actor User, created User) {
	if s.mail == nil || strings.TrimSpace(created.Email) == "" {
		return
	}
	notification := mail.AccountCreated(s.serviceName(ctx), created.Username, created.DisplayName)
	notification.ResourceID = created.ID
	s.mail.Notify(ctx, notification, actor.ID, []string{created.ID})
}

func (s *Server) serviceName(ctx context.Context) string {
	var general struct {
		ServiceName string `json:"service_name"`
	}
	if s.readSetting(ctx, "system", "general", &general, nil) == nil && strings.TrimSpace(general.ServiceName) != "" {
		return strings.TrimSpace(general.ServiceName)
	}
	return "Orbit"
}

// mailSettingsView 는 화면에 돌려줄 모양이다. 비밀번호는 "설정됨" 여부만 나간다.
func mailSettingsView(settings mail.Settings, hasPassword bool) map[string]any {
	settings.Password = ""
	settings.ClearPassword = false
	return map[string]any{"smtp": settings, "has_password": hasPassword}
}

func (s *Server) adminMailDeliveries(w http.ResponseWriter, r *http.Request) {
	if s.mail == nil {
		writeJSON(w, 200, mail.Page{Items: []mail.Delivery{}, Summary: mail.Summary{Status: map[string]int{}}})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	page, err := s.mail.Deliveries(r.Context(), r.URL.Query().Get("status"), limit)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, 200, page)
}

// adminSendTestMail 은 저장된 설정으로 실제 한 통을 보내고 결과를 그 자리에서
// 보여 준다. 릴레이 설정은 한 번에 맞는 일이 드물다.
func (s *Server) adminSendTestMail(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	var in struct {
		Recipient string `json:"recipient"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	recipient := strings.TrimSpace(in.Recipient)
	if !strings.Contains(recipient, "@") {
		writeError(w, 400, "validation_error", "받는 사람 메일 주소를 확인해 주세요.")
		return
	}
	if s.mail == nil {
		writeError(w, 503, "mail_unavailable", "메일 서비스가 구성되지 않았습니다.")
		return
	}
	err := s.mail.SendNow(r.Context(), mail.TestMessage(), u.ID, recipient)
	s.audit(r.Context(), u.ID, "mail.test", "mail", recipient, r.RemoteAddr, map[string]any{"sent": err == nil})
	switch {
	case errors.Is(err, mail.ErrDisabled):
		writeError(w, 409, "mail_disabled", "메일 알림을 먼저 켜고 저장한 뒤 시험 발송하세요.")
		return
	case errors.Is(err, mail.ErrInvalid):
		writeError(w, 400, "validation_error", err.Error())
		return
	case err != nil:
		// 오류 문구에는 릴레이의 응답이 들어가지만 비밀번호는 들어가지 않는다.
		writeError(w, 502, "mail_send_failed", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"sent": true, "recipient": recipient})
}
