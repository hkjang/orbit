package mail

import (
	"fmt"
	"strings"
	"time"
)

// 기본값은 흔한 경우를 겨냥한다: 포트 25 로 자격 증명 없이 받는 사내 릴레이.
const (
	defaultPort     = 25
	defaultSecurity = "auto"
	defaultTimeout  = 10 * time.Second
	defaultFromName = "Orbit"
)

// Settings 는 settings 표의 mail:smtp 행에 저장되는 값이다.
//
// 필드 이름은 사내 표준의 설정 키와 같다 — 관리 API 의 네임스페이스가 mail 이므로
// smtp_host 는 곧 mail.smtp_host 다. 앱마다 이름이 다르면 운영자가 스무 번
// 다르게 배운다.
//
// 비밀번호는 이 구조체에 실려 저장되지 않는다. 쓸 때만 Password 로 받아
// encrypted_value 에 따로 두고, 읽을 때는 "설정됨" 여부만 돌려준다.
type Settings struct {
	Enabled               bool   `json:"enabled"`
	SMTPHost              string `json:"smtp_host"`
	SMTPPort              int    `json:"smtp_port"`
	Security              string `json:"security"`
	SkipTLSVerify         bool   `json:"skip_tls_verify"`
	Username              string `json:"username"`
	Password              string `json:"password,omitempty"`
	ClearPassword         bool   `json:"clear_password,omitempty"`
	FromAddress           string `json:"from_address"`
	FromName              string `json:"from_name"`
	BaseURL               string `json:"base_url"`
	TimeoutSeconds        int    `json:"timeout_seconds"`
	NotifyApprovalRequest bool   `json:"notify_approval_request"`
	NotifyApprovalDecided bool   `json:"notify_approval_decided"`
	NotifyAccountCreated  bool   `json:"notify_account_created"`
}

// DefaultSettings 는 새로 설치한 곳의 값이다. 꺼져 있으므로 아무것도 달라지지
// 않는다.
func DefaultSettings() Settings {
	return Settings{
		SMTPPort: defaultPort, Security: defaultSecurity, FromName: defaultFromName,
		TimeoutSeconds:        int(defaultTimeout / time.Second),
		NotifyApprovalRequest: true, NotifyApprovalDecided: true, NotifyAccountCreated: true,
	}
}

// Normalize 는 저장 직전에 값을 다듬고 켜진 설정이 갖춰졌는지 본다. 꺼진
// 설정은 반쯤 채워 두어도 된다 — 릴레이 주소를 먼저 적고 나중에 켜는 일이 흔하다.
func (s *Settings) Normalize() error {
	s.SMTPHost = strings.TrimSpace(s.SMTPHost)
	s.Username = strings.TrimSpace(s.Username)
	s.FromAddress = strings.TrimSpace(s.FromAddress)
	s.FromName = strings.TrimSpace(s.FromName)
	s.BaseURL = strings.TrimRight(strings.TrimSpace(s.BaseURL), "/")
	s.Security = strings.ToLower(strings.TrimSpace(s.Security))
	if s.Security == "" {
		s.Security = defaultSecurity
	}
	if s.SMTPPort == 0 {
		s.SMTPPort = defaultPort
	}
	if s.TimeoutSeconds == 0 {
		s.TimeoutSeconds = int(defaultTimeout / time.Second)
	}
	if s.FromName == "" {
		s.FromName = defaultFromName
	}
	switch s.Security {
	case "auto", "none", "starttls", "tls":
	default:
		return fmt.Errorf("%w: 보안 방식은 auto·none·starttls·tls 중 하나여야 합니다", ErrInvalid)
	}
	if s.SMTPPort < 1 || s.SMTPPort > 65535 {
		return fmt.Errorf("%w: SMTP 포트는 1~65535 범위여야 합니다", ErrInvalid)
	}
	if s.TimeoutSeconds < 1 || s.TimeoutSeconds > 120 {
		return fmt.Errorf("%w: 제한 시간은 1~120초 범위여야 합니다", ErrInvalid)
	}
	if s.FromAddress != "" && !strings.Contains(s.FromAddress, "@") {
		return fmt.Errorf("%w: 보내는 주소는 메일 주소여야 합니다", ErrInvalid)
	}
	if s.BaseURL != "" && !strings.HasPrefix(s.BaseURL, "http://") && !strings.HasPrefix(s.BaseURL, "https://") {
		return fmt.Errorf("%w: 서비스 주소는 http:// 또는 https:// 로 시작해야 합니다", ErrInvalid)
	}
	if s.Enabled && s.SMTPHost == "" {
		return fmt.Errorf("%w: 켜려면 SMTP 호스트가 필요합니다", ErrInvalid)
	}
	return nil
}

// Config 는 저장된 값과 따로 보관한 비밀번호로 발송 구성을 만든다.
func (s Settings) Config(password string) Config {
	config := Config{
		Enabled: s.Enabled, Host: strings.TrimSpace(s.SMTPHost), Port: s.SMTPPort,
		Security: strings.ToLower(strings.TrimSpace(s.Security)), SkipVerify: s.SkipTLSVerify,
		Username: strings.TrimSpace(s.Username), Password: password,
		FromAddress: strings.TrimSpace(s.FromAddress), FromName: strings.TrimSpace(s.FromName),
		BaseURL: strings.TrimRight(strings.TrimSpace(s.BaseURL), "/"),
		Timeout: time.Duration(s.TimeoutSeconds) * time.Second,
		Events: map[string]bool{
			EventApprovalRequested: s.NotifyApprovalRequest,
			EventApprovalDecided:   s.NotifyApprovalDecided,
			EventAccountCreated:    s.NotifyAccountCreated,
		},
	}
	if config.Port == 0 {
		config.Port = defaultPort
	}
	if config.Security == "" {
		config.Security = defaultSecurity
	}
	// 암묵적 TLS 포트의 릴레이는 따로 고르지 않아도 된다.
	if config.Security == defaultSecurity && config.Port == 465 {
		config.Security = "tls"
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultTimeout
	}
	if config.FromName == "" {
		config.FromName = defaultFromName
	}
	if config.FromAddress == "" && config.Host != "" {
		config.FromAddress = "orbit@" + config.Host
	}
	return config
}
