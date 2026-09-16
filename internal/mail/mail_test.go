package mail

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeRelay 는 가장 작은 SMTP 서버다. 대화를 기록해 두어 Orbit 이 실제로 무엇을
// 말했는지 — 특히 인증을 시도했는지 — 확인할 수 있다.
type fakeRelay struct {
	address   string
	offerAuth bool
	mu        sync.Mutex
	commands  []string
	body      string
	listener  net.Listener
}

func startRelay(t *testing.T, offerAuth bool) *fakeRelay {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	relay := &fakeRelay{address: listener.Addr().String(), offerAuth: offerAuth, listener: listener}
	go relay.serve()
	t.Cleanup(func() { _ = listener.Close() })
	return relay
}

func (f *fakeRelay) config() Config {
	host, port, _ := net.SplitHostPort(f.address)
	value := 0
	_, _ = fmt.Sscanf(port, "%d", &value)
	return Config{Enabled: true, Host: host, Port: value, Security: "auto", FromAddress: "orbit@example.internal", FromName: "Orbit 알림", Timeout: 2 * time.Second}
}

func (f *fakeRelay) transcript() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.commands...)
}

func (f *fakeRelay) serve() {
	for {
		connection, err := f.listener.Accept()
		if err != nil {
			return
		}
		go f.handle(connection)
	}
}

func (f *fakeRelay) handle(connection net.Conn) {
	defer connection.Close()
	reader := bufio.NewReader(connection)
	write := func(line string) { _, _ = connection.Write([]byte(line + "\r\n")) }
	write("220 relay.internal ESMTP orbit-test")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		command := strings.TrimSpace(line)
		f.mu.Lock()
		f.commands = append(f.commands, command)
		f.mu.Unlock()
		upper := strings.ToUpper(command)
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			write("250-relay.internal")
			if f.offerAuth {
				write("250-AUTH PLAIN LOGIN")
			}
			write("250 8BITMIME")
		case strings.HasPrefix(upper, "AUTH"):
			write("235 ok")
		case strings.HasPrefix(upper, "MAIL FROM"), strings.HasPrefix(upper, "RCPT TO"):
			write("250 ok")
		case upper == "DATA":
			write("354 go ahead")
			var body strings.Builder
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(line, "\r\n") == "." {
					break
				}
				body.WriteString(line)
			}
			f.mu.Lock()
			f.body = body.String()
			f.mu.Unlock()
			write("250 queued")
		case upper == "QUIT":
			write("221 bye")
			return
		default:
			write("250 ok")
		}
	}
}

func TestDeliverWithoutAuthOnPlainRelay(t *testing.T) {
	relay := startRelay(t, false)
	config := relay.config()
	config.Username, config.Password = "", ""
	err := Deliver(context.Background(), config, Message{To: "lead@example.internal", Subject: "검토 요청", Body: "첫 줄\n.점으로 시작하는 줄\n마지막"})
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	for _, command := range relay.transcript() {
		if strings.HasPrefix(strings.ToUpper(command), "AUTH") {
			t.Fatalf("relay without credentials must not see AUTH: %v", relay.transcript())
		}
	}
	relay.mu.Lock()
	body := relay.body
	relay.mu.Unlock()
	if !strings.Contains(body, "Subject: =?utf-8?q?") {
		t.Fatalf("subject must be encoded: %q", body)
	}
	// 릴레이는 점이 하나 더 붙은 줄을 받는다(DATA 기록기의 몫). 두 번 붙으면
	// 받는 사람에게 점이 남는다.
	if !strings.Contains(body, "\r\n..점으로") || strings.Contains(body, "...점으로") {
		t.Fatalf("leading dot must be stuffed exactly once on the wire: %q", body)
	}
	if !strings.Contains(body, "X-Orbit-Notification: 1") {
		t.Fatalf("notification header missing: %q", body)
	}
}

func TestDeliverAuthenticatesWhenCredentialsGiven(t *testing.T) {
	relay := startRelay(t, true)
	config := relay.config()
	config.Username, config.Password = "orbit", "secret"
	if err := Deliver(context.Background(), config, Message{To: "lead@example.internal", Subject: "t", Body: "b"}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	authenticated := false
	for _, command := range relay.transcript() {
		if strings.HasPrefix(strings.ToUpper(command), "AUTH") {
			authenticated = true
		}
		if strings.Contains(command, "secret") {
			t.Fatalf("password must not travel in clear on the command line: %q", command)
		}
	}
	if !authenticated {
		t.Fatalf("credentials were given but no AUTH was sent: %v", relay.transcript())
	}
}

func TestDeliverRejectsIncompleteConfig(t *testing.T) {
	err := Deliver(context.Background(), Config{Enabled: true, Port: 25, Security: "auto", FromAddress: "a@b"}, Message{To: "x@y", Subject: "t"})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "mail.smtp_host") {
		t.Fatalf("missing host must be reported as invalid with the key name, got %v", err)
	}
}

func TestSettingsDefaultsMatchInternalRelay(t *testing.T) {
	config := DefaultSettings().Config("")
	if config.Enabled {
		t.Fatal("mail must be off by default")
	}
	if config.Port != 25 || config.Security != "auto" || config.Timeout != 10*time.Second {
		t.Fatalf("defaults must be port 25, auto, 10s: %+v", config)
	}
	if config.Username != "" || config.Password != "" {
		t.Fatal("credentials are optional and empty by default")
	}
	for _, event := range []string{EventApprovalRequested, EventApprovalDecided, EventAccountCreated, "unknown.event"} {
		if !config.Allows(event) {
			t.Fatalf("event %s must be allowed by default", event)
		}
	}
	// 암묵적 TLS 포트는 따로 고르지 않아도 된다.
	settings := DefaultSettings()
	settings.SMTPPort, settings.SMTPHost = 465, "relay.internal"
	if got := settings.Config("").Security; got != "tls" {
		t.Fatalf("port 465 with auto must become tls, got %s", got)
	}
	if got := settings.Config("").FromAddress; got != "orbit@relay.internal" {
		t.Fatalf("from address must fall back to the relay host, got %s", got)
	}
}

func TestSettingsNormalize(t *testing.T) {
	settings := DefaultSettings()
	settings.Enabled = true
	if err := settings.Normalize(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("enabled without host must be invalid, got %v", err)
	}
	settings = DefaultSettings()
	settings.SMTPHost, settings.Security, settings.BaseURL = " relay.internal ", " STARTTLS ", "https://orbit.internal/"
	if err := settings.Normalize(); err != nil {
		t.Fatalf("disabled but half-filled settings must save: %v", err)
	}
	if settings.SMTPHost != "relay.internal" || settings.Security != "starttls" || settings.BaseURL != "https://orbit.internal" {
		t.Fatalf("values must be trimmed and lower-cased: %+v", settings)
	}
	settings.Security = "ssl"
	if err := settings.Normalize(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown security must be rejected, got %v", err)
	}
}

// memoryLog 는 테스트용 발송 기록이다.
type memoryLog struct {
	mu    sync.Mutex
	items []Delivery
}

func (m *memoryLog) Record(_ context.Context, d Delivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append(m.items, d)
	return nil
}

func (m *memoryLog) Complete(_ context.Context, d Delivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.items {
		if m.items[i].ID == d.ID {
			m.items[i].Status, m.items[i].Attempts, m.items[i].ErrorMessage = d.Status, d.Attempts, d.ErrorMessage
		}
	}
	return nil
}

func (m *memoryLog) RecentlyNotified(_ context.Context, event, recipient string, within time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.items {
		if item.Event == event && strings.EqualFold(item.Recipient, recipient) && item.Status != "failed" && time.Since(item.CreatedAt) < within {
			return true, nil
		}
	}
	return false, nil
}

func (m *memoryLog) List(_ context.Context, status string, limit int) (Page, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	page := Page{Items: []Delivery{}, Summary: Summary{Status: map[string]int{}}}
	for _, item := range m.items {
		page.Summary.Status[item.Status]++
		page.Summary.Total++
		if status == "" || item.Status == status {
			page.Items = append(page.Items, item)
		}
	}
	return page, nil
}

func (m *memoryLog) snapshot() []Delivery {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Delivery(nil), m.items...)
}

type sentMail struct {
	mu    sync.Mutex
	items []Message
}

func (s *sentMail) send(_ context.Context, _ Config, message Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, message)
	return nil
}

func (s *sentMail) recipients() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []string{}
	for _, item := range s.items {
		out = append(out, item.To)
	}
	return out
}

var directory = Directory(func(_ context.Context, ids []string) (map[string]string, error) {
	known := map[string]string{"lead": "lead@example.internal", "admin": "admin@example.internal", "member": "member@example.internal"}
	out := map[string]string{}
	for _, id := range ids {
		if email, ok := known[id]; ok {
			out[id] = email
		}
	}
	return out, nil
})

func newTestService(config Config) (*Service, *memoryLog, *sentMail) {
	log := &memoryLog{}
	sent := &sentMail{}
	service := NewService(log, func(context.Context) (Config, error) { return config, nil }, directory)
	service.SetSender(sent.send)
	return service, log, sent
}

func enabledConfig() Config {
	return Config{Enabled: true, Host: "relay.internal", Port: 25, Security: "auto", FromAddress: "orbit@example.internal", Timeout: time.Second, Events: map[string]bool{}}
}

func TestNotifyDoesNothingWhenDisabled(t *testing.T) {
	config := enabledConfig()
	config.Enabled = false
	service, log, sent := newTestService(config)
	service.Notify(context.Background(), ApprovalRequested("member", "제목", "", 1), "member", []string{"lead"})
	service.Wait()
	if len(sent.recipients()) != 0 || len(log.snapshot()) != 0 {
		t.Fatal("disabled mail must neither send nor record")
	}
}

func TestNotifySkipsActorAndUnknownAddresses(t *testing.T) {
	service, _, sent := newTestService(enabledConfig())
	service.Notify(context.Background(), ApprovalRequested("lead", "제목", "", 1), "lead", []string{"lead", "admin", "ghost", "admin"})
	service.Wait()
	if got := sent.recipients(); len(got) != 1 || got[0] != "admin@example.internal" {
		t.Fatalf("actor, unknown and duplicate recipients must be dropped, got %v", got)
	}
}

func TestNotifyHonoursEventSwitch(t *testing.T) {
	config := enabledConfig()
	config.Events[EventApprovalRequested] = false
	service, _, sent := newTestService(config)
	service.Notify(context.Background(), ApprovalRequested("member", "제목", "", 1), "member", []string{"lead"})
	service.Notify(context.Background(), ApprovalDecided("lead", "제목", "approved", ""), "lead", []string{"member"})
	service.Wait()
	if got := sent.recipients(); len(got) != 1 || got[0] != "member@example.internal" {
		t.Fatalf("only the switched-off event must stop, got %v", got)
	}
}

func TestNotifyRecordsSuccessAndFailure(t *testing.T) {
	service, log, _ := newTestService(enabledConfig())
	var calls atomic.Int32
	service.SetSender(func(_ context.Context, _ Config, message Message) error {
		calls.Add(1)
		if message.To == "lead@example.internal" {
			return errors.New("connection refused")
		}
		return nil
	})
	service.Notify(context.Background(), ApprovalRequested("member", "제목", "", 1), "member", []string{"lead", "admin"})
	service.Wait()
	byRecipient := map[string]Delivery{}
	for _, item := range log.snapshot() {
		byRecipient[item.Recipient] = item
	}
	if got := byRecipient["admin@example.internal"]; got.Status != "sent" || got.Attempts != 1 {
		t.Fatalf("success must be recorded as sent: %+v", got)
	}
	if got := byRecipient["lead@example.internal"]; got.Status != "failed" || got.Attempts != 2 || !strings.Contains(got.ErrorMessage, "refused") {
		t.Fatalf("failure must be recorded with the reason after a retry: %+v", got)
	}
	if calls.Load() != 3 {
		t.Fatalf("a refused connection is retried once, got %d calls", calls.Load())
	}
}

func TestNotifyLeavesReasonWhenConfigIncomplete(t *testing.T) {
	config := enabledConfig()
	config.Host = ""
	service, log, _ := newTestService(config)
	service.SetSender(Deliver)
	service.Notify(context.Background(), ApprovalRequested("member", "제목", "", 1), "member", []string{"lead"})
	service.Wait()
	items := log.snapshot()
	if len(items) != 1 || items[0].Status != "failed" || !strings.Contains(items[0].ErrorMessage, "mail.smtp_host") || items[0].Attempts != 1 {
		t.Fatalf("enabled but incomplete settings must record why nothing went out, without retrying: %+v", items)
	}
}

func TestNotifyDoesNotBlockOnDeadRelay(t *testing.T) {
	// 아무도 듣지 않는 포트. 연결 거절이 곧바로 돌아온다.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	config := enabledConfig()
	config.Host, _, _ = net.SplitHostPort(address)
	_, port, _ := net.SplitHostPort(address)
	_, _ = fmt.Sscanf(port, "%d", &config.Port)
	service, log, _ := newTestService(config)
	service.SetSender(Deliver)
	started := time.Now()
	service.Notify(context.Background(), ApprovalRequested("member", "제목", "", 1), "member", []string{"lead"})
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Notify must return before the relay is contacted, took %s", elapsed)
	}
	items := log.snapshot()
	if len(items) != 1 || items[0].Status != "queued" {
		t.Fatalf("the attempt must be recorded as queued before sending: %+v", items)
	}
	service.Wait()
	if got := log.snapshot()[0]; got.Status != "failed" || got.ErrorMessage == "" {
		t.Fatalf("dead relay must end as failed with a reason: %+v", got)
	}
}

func TestNotifyCollapsesRepeatedRequests(t *testing.T) {
	service, log, sent := newTestService(enabledConfig())
	for i := 0; i < 3; i++ {
		service.Notify(context.Background(), ApprovalRequested("member", fmt.Sprintf("기억 %d", i), "", i+1), "member", []string{"lead"})
	}
	service.Wait()
	if got := sent.recipients(); len(got) != 1 {
		t.Fatalf("three requests in a row must reach the reviewer once, got %d", len(got))
	}
	// 결과 알림은 묶지 않는다. 각 요청자가 자기 결과를 기다린다.
	service.Notify(context.Background(), ApprovalDecided("lead", "기억 0", "approved", ""), "lead", []string{"member"})
	service.Notify(context.Background(), ApprovalDecided("lead", "기억 1", "rejected", "다시"), "lead", []string{"member"})
	service.Wait()
	if got := sent.recipients(); len(got) != 3 {
		t.Fatalf("decisions must not collapse, got %v", got)
	}
	if page, _ := service.Deliveries(context.Background(), "", 0); page.Summary.Status["sent"] != 3 || len(log.snapshot()) != 3 {
		t.Fatalf("every attempt must be listed: %+v", page.Summary)
	}
}

func TestSendNowReportsOutcome(t *testing.T) {
	config := enabledConfig()
	config.Enabled = false
	service, _, _ := newTestService(config)
	if err := service.SendNow(context.Background(), TestMessage(), "admin", "admin@example.internal"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("test mail while disabled must say so, got %v", err)
	}
	service, log, _ := newTestService(enabledConfig())
	service.SetSender(func(context.Context, Config, Message) error { return errors.New("550 relay denied") })
	err := service.SendNow(context.Background(), TestMessage(), "admin", "admin@example.internal")
	if err == nil || !strings.Contains(err.Error(), "550") {
		t.Fatalf("relay error must surface to the admin, got %v", err)
	}
	if items := log.snapshot(); len(items) != 1 || items[0].Status != "failed" || items[0].Event != EventTest {
		t.Fatalf("the test send must be in the log too: %+v", items)
	}
}

func TestRenderAppendsLinkAndOmitsBody(t *testing.T) {
	config := enabledConfig()
	config.BaseURL = "https://orbit.internal/"
	body := ApprovalRequested("홍길동", "첫 만남", "급합니다", 4).Render(config)
	for _, want := range []string{"홍길동 님이 기억 '첫 만남'", "> 급합니다", "모두 4건", "바로 열기: https://orbit.internal/approvals", "자동으로 발송"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body must contain %q:\n%s", want, body)
		}
	}
	config.BaseURL = ""
	if strings.Contains(ApprovalDecided("팀장", "첫 만남", "rejected", "").Render(config), "바로 열기") {
		t.Fatal("without base_url no link can be built")
	}
	account := AccountCreated("Orbit", "hong", "홍길동").Render(config)
	if strings.Contains(strings.ToLower(account), "password:") {
		t.Fatal("account mail must not carry a password")
	}
}
