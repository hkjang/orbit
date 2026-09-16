package mail

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/hkjang/orbit/internal/id"
)

// Delivery 는 발송 기록 한 줄이다. 언제·어떤 이벤트로·누구에게·무슨 제목으로
// 보냈고 되었는지 안 되었는지를 남긴다. 본문은 담지 않는다.
type Delivery struct {
	ID           string    `json:"id"`
	Event        string    `json:"event"`
	Recipient    string    `json:"recipient"`
	Subject      string    `json:"subject"`
	ActorID      string    `json:"actor_id,omitempty"`
	ResourceType string    `json:"resource_type,omitempty"`
	ResourceID   string    `json:"resource_id,omitempty"`
	Status       string    `json:"status"`
	Attempts     int       `json:"attempts"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Summary struct {
	Total  int            `json:"total"`
	Status map[string]int `json:"status"`
}

type Page struct {
	Items   []Delivery `json:"items"`
	Summary Summary    `json:"summary"`
}

// Log 는 발송 기록을 보관한다. nil 이면 아무것도 기록하지 않는다(테스트).
type Log interface {
	Record(ctx context.Context, delivery Delivery) error
	Complete(ctx context.Context, delivery Delivery) error
	// RecentlyNotified 는 같은 사람에게 같은 이벤트가 그 시간 안에 큐에 들어갔거나
	// 나갔는지 말한다. 묶어 보내기의 근거다.
	RecentlyNotified(ctx context.Context, event, recipient string, within time.Duration) (bool, error)
	List(ctx context.Context, status string, limit int) (Page, error)
}

// Directory 는 계정 id 를 메일 주소로 바꾼다. 앱은 이미 사용자를 알고 있으므로
// 이 조회 하나만 빌려 쓴다 — 메일이 자기 사용자 표를 갖기 시작하면 두 명부가
// 어긋난다. 주소가 없는 계정은 결과에서 빠진다.
type Directory func(ctx context.Context, userIDs []string) (map[string]string, error)

// ConfigSource 는 저장된 설정에서 발송 구성을 읽는다. 요청마다 읽으므로
// 관리자가 바꾼 값이 재시작 없이 바로 적용된다.
type ConfigSource func(ctx context.Context) (Config, error)

type Service struct {
	log       Log
	config    ConfigSource
	directory Directory
	send      func(context.Context, Config, Message) error
	now       func() time.Time
	pending   sync.WaitGroup
}

func NewService(log Log, config ConfigSource, directory Directory) *Service {
	return &Service{log: log, config: config, directory: directory, send: Deliver, now: func() time.Time { return time.Now().UTC() }}
}

// SetSender 는 전송부를 바꾼다. 테스트가 실제 릴레이 없이 서비스를 돌리는 길이다.
func (s *Service) SetSender(sender func(context.Context, Config, Message) error) { s.send = sender }

// Wait 은 배경 발송이 모두 끝날 때까지 기다린다. 테스트와 종료 절차용이다.
func (s *Service) Wait() { s.pending.Wait() }

func (s *Service) Config(ctx context.Context) (Config, error) { return s.config(ctx) }

// Notify 는 받는 사람을 정하고 배경에서 보낸다. 어떤 요청도 메일 서버를
// 기다리지 않는다. 꺼져 있거나, 이벤트 스위치가 꺼져 있거나, 주소가 없는 사람은
// 조용히 건너뛴다. 설정이 모자라면(호스트 없음 등) 보내지 않되 그 이유를
// 기록에 남긴다 — 그래야 "안 왔다" 는 문의에 답할 수 있다.
func (s *Service) Notify(ctx context.Context, notification Notification, actorID string, recipients []string) {
	config, err := s.config(ctx)
	if err != nil {
		slog.Warn("mail settings were not read", "error", err)
		return
	}
	if !config.Enabled || !config.Allows(notification.Event) {
		return
	}
	addresses := s.resolve(ctx, recipients, actorID)
	if len(addresses) == 0 {
		return
	}
	body := notification.Render(config)
	for _, address := range addresses {
		if s.collapsed(ctx, notification, address) {
			continue
		}
		delivery := Delivery{
			ID: id.New(), Event: notification.Event, Recipient: address, Subject: notification.Subject,
			ActorID: actorID, ResourceType: notification.ResourceType, ResourceID: notification.ResourceID,
			Status: "queued", CreatedAt: s.now(), UpdatedAt: s.now(),
		}
		s.record(ctx, delivery)
		s.pending.Add(1)
		go func(delivery Delivery, message Message) {
			defer s.pending.Done()
			s.deliver(delivery, config, message)
		}(delivery, Message{To: address, Subject: notification.Subject, Body: body})
	}
}

// SendNow 는 즉시 보내고 결과를 돌려준다. 관리자의 시험 발송 단추가 쓴다.
func (s *Service) SendNow(ctx context.Context, notification Notification, actorID, recipient string) error {
	config, err := s.config(ctx)
	if err != nil {
		return err
	}
	if !config.Enabled {
		return ErrDisabled
	}
	delivery := Delivery{
		ID: id.New(), Event: notification.Event, Recipient: recipient, Subject: notification.Subject,
		ActorID: actorID, Status: "queued", CreatedAt: s.now(), UpdatedAt: s.now(),
	}
	s.record(ctx, delivery)
	sendContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), config.Timeout+5*time.Second)
	defer cancel()
	delivery.Attempts = 1
	err = s.send(sendContext, config, Message{To: recipient, Subject: notification.Subject, Body: notification.Render(config)})
	s.complete(sendContext, delivery, err)
	return err
}

// Deliveries 는 나간 것을 최신순으로 보여 준다.
func (s *Service) Deliveries(ctx context.Context, status string, limit int) (Page, error) {
	if s.log == nil {
		return Page{Items: []Delivery{}, Summary: Summary{Status: map[string]int{}}}, nil
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	return s.log.List(ctx, strings.TrimSpace(status), limit)
}

// deliver 는 한 번 더 시도한다. 잠깐 연결을 거절하는 릴레이는 흔하고, 알림을
// 잃는 것이 몇 초 기다리는 것보다 나쁘다.
func (s *Service) deliver(delivery Delivery, config Config, message Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*config.Timeout+15*time.Second)
	defer cancel()
	var err error
	for attempt := 1; attempt <= 2; attempt++ {
		delivery.Attempts = attempt
		if err = s.send(ctx, config, message); err == nil {
			break
		}
		// 설정 자체가 틀렸으면 다시 해도 같다.
		if isInvalid(err) || attempt == 2 {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}
	s.complete(ctx, delivery, err)
}

func (s *Service) complete(ctx context.Context, delivery Delivery, cause error) {
	delivery.Status, delivery.ErrorMessage, delivery.UpdatedAt = "sent", "", s.now()
	if cause != nil {
		delivery.Status, delivery.ErrorMessage = "failed", trim(cause.Error(), 1000)
		slog.Warn("notification mail failed", "event", delivery.Event, "recipient", delivery.Recipient, "error", cause)
	}
	if s.log == nil {
		return
	}
	if err := s.log.Complete(ctx, delivery); err != nil {
		slog.Warn("mail delivery status was not recorded", "error", err)
	}
}

func (s *Service) record(ctx context.Context, delivery Delivery) {
	if s.log == nil {
		return
	}
	delivery.Subject = trim(delivery.Subject, 300)
	if err := s.log.Record(context.WithoutCancel(ctx), delivery); err != nil {
		slog.Warn("mail delivery was not recorded", "error", err)
	}
}

func (s *Service) collapsed(ctx context.Context, notification Notification, address string) bool {
	if s.log == nil || notification.Collapse <= 0 {
		return false
	}
	recent, err := s.log.RecentlyNotified(ctx, notification.Event, address, notification.Collapse)
	if err != nil {
		slog.Warn("mail delivery history was not read", "error", err)
		return false
	}
	return recent
}

// resolve 는 계정 id 를 겹치지 않는 주소로 바꾼다. 행위자는 뺀다 — 자기가 한
// 일을 자기에게 알리지 않는다.
func (s *Service) resolve(ctx context.Context, recipients []string, actorID string) []string {
	wanted := make([]string, 0, len(recipients))
	seenID := map[string]struct{}{}
	for _, recipient := range recipients {
		trimmed := strings.TrimSpace(recipient)
		if trimmed == "" || strings.EqualFold(trimmed, strings.TrimSpace(actorID)) {
			continue
		}
		if _, duplicate := seenID[strings.ToLower(trimmed)]; duplicate {
			continue
		}
		seenID[strings.ToLower(trimmed)] = struct{}{}
		wanted = append(wanted, trimmed)
	}
	if len(wanted) == 0 || s.directory == nil {
		return nil
	}
	emails, err := s.directory(ctx, wanted)
	if err != nil {
		slog.Warn("mail recipients were not resolved", "error", err)
		return nil
	}
	seen, addresses := map[string]struct{}{}, make([]string, 0, len(wanted))
	for _, recipient := range wanted {
		address := strings.TrimSpace(emails[recipient])
		if address == "" {
			continue
		}
		key := strings.ToLower(address)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		addresses = append(addresses, address)
	}
	return addresses
}

func isInvalid(err error) bool { return errors.Is(err, ErrInvalid) }

func trim(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
