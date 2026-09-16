package mail

import (
	"fmt"
	"strings"
	"time"
)

// Notification 은 받는 사람을 정하기 전의 이벤트 메일 한 통이다.
type Notification struct {
	Event   string
	Subject string
	Lines   []string
	// Link 는 앱 안의 경로다. 설정의 base_url 을 앞에 붙여 절대 주소로 만든다.
	Link         string
	ResourceType string
	ResourceID   string
	// Collapse 가 0 보다 크면, 같은 사람에게 같은 이벤트를 그 시간 안에 이미
	// 보냈을 때 다시 보내지 않는다. 한 사람이 기억을 열 개 잇달아 올리면
	// 검토자는 열 통이 아니라 한 통을 받고 화면에서 나머지를 본다.
	Collapse time.Duration
}

// Render 는 본문을 만든다. 링크와, 이 메일이 왜 왔는지 말하는 꼬리를 붙인다.
func (n Notification) Render(config Config) string {
	lines := append([]string{}, n.Lines...)
	if link := n.absoluteLink(config); link != "" {
		lines = append(lines, "", "바로 열기: "+link)
	}
	lines = append(lines, "", "—", "이 메일은 Orbit 메일 알림 설정에 따라 자동으로 발송되었습니다.")
	return strings.Join(lines, "\n")
}

func (n Notification) absoluteLink(config Config) string {
	base := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if n.Link == "" || base == "" {
		return ""
	}
	return base + "/" + strings.TrimLeft(n.Link, "/")
}

// ApprovalRequested 는 검토자에게 기억이 검토를 기다린다고 알린다.
//
// 기억 본문은 싣지 않는다. 본문은 사용자 키로 암호화된 것이고, 검토자는 화면에서
// 자기 권한으로 본다. 메일에 실으면 알림이 유출 경로가 된다.
func ApprovalRequested(requester, title, note string, pending int) Notification {
	lines := []string{fmt.Sprintf("%s 님이 기억 '%s' 의 검토를 요청했습니다.", requester, title)}
	if strings.TrimSpace(note) != "" {
		lines = append(lines, "", quote(note))
	}
	if pending > 1 {
		lines = append(lines, "", fmt.Sprintf("검토를 기다리는 요청이 모두 %d건 있습니다.", pending))
	}
	return Notification{
		Event: EventApprovalRequested, Subject: fmt.Sprintf("[Orbit] 검토 요청: %s", title),
		Lines: lines, Link: "/approvals", ResourceType: "memory", Collapse: 15 * time.Minute,
	}
}

// ApprovalDecided 는 요청자에게 검토 결과를 알린다.
func ApprovalDecided(reviewer, title, decision, note string) Notification {
	result := "반려되었습니다"
	if decision == "approved" {
		result = "승인되었습니다"
	}
	lines := []string{fmt.Sprintf("기억 '%s' 이(가) %s. (검토: %s)", title, result, reviewer)}
	if strings.TrimSpace(note) != "" {
		lines = append(lines, "", quote(note))
	}
	return Notification{
		Event: EventApprovalDecided, Subject: fmt.Sprintf("[Orbit] 기억 '%s' 검토 결과: %s", title, result),
		Lines: lines, Link: "/memories", ResourceType: "memory",
	}
}

// AccountCreated 는 관리자가 만든 계정이 준비되었다고 그 사람에게 알린다.
// 비밀번호는 싣지 않는다 — 관리자가 따로 전한다.
func AccountCreated(serviceName, username, displayName string) Notification {
	return Notification{
		Event: EventAccountCreated, Subject: fmt.Sprintf("[%s] 계정이 준비되었습니다", serviceName),
		Lines: []string{
			fmt.Sprintf("%s 님, %s 계정이 만들어졌습니다.", displayName, serviceName),
			fmt.Sprintf("사용자 ID: %s", username),
			"비밀번호는 관리자에게 따로 받으세요. SSO 를 쓰는 곳에서는 사내 계정으로 바로 들어갈 수 있습니다.",
		},
		Link: "/login", ResourceType: "user",
	}
}

// TestMessage 는 관리 화면에서 릴레이가 동작하는지 증명한다.
func TestMessage() Notification {
	return Notification{
		Event: EventTest, Subject: "[Orbit] SMTP 발송 테스트",
		Lines: []string{"Orbit 관리 화면에서 보낸 테스트 메일입니다.", "이 메일을 받았다면 SMTP 설정이 정상입니다."},
	}
}

func quote(body string) string {
	trimmed := strings.TrimSpace(body)
	if len([]rune(trimmed)) > 500 {
		trimmed = string([]rune(trimmed)[:500]) + "…"
	}
	lines := strings.Split(trimmed, "\n")
	for index, line := range lines {
		lines[index] = "> " + line
	}
	return strings.Join(lines, "\n")
}
