package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/hkjang/orbit/internal/id"
	"github.com/hkjang/orbit/internal/secure"
	"github.com/jackc/pgx/v5"
)

// 기억을 다른 서비스로 넘기기.
//
// 사람과 함께한 순간의 기록은 문서가 되고 슬라이드가 되어 보고로 들어간다.
// 형식은 이미 맞는다 — Orbit 은 마크다운을 만들고 muni·ptium·weekly 는
// 마크다운을 읽는다. 없던 것은 넘기는 손이다.
//
// Orbit 은 사내 표준(HANDOFF-STANDARD.md)의 보내는 쪽이고, 그것만이다. 마크다운
// 하나를 내주고 아무것도 받아 들이지 않는다. 아래 경로의 이름·요청 모양·응답
// 코드는 표준의 것이라 이 저장소가 바꿀 수 없다.
//
//	GET  /api/v1/handoff/targets        보낼 수 있는 곳 (로그인)
//	POST /api/v1/handoff/claims         기억 하나에 표를 발급한다 (로그인)
//	GET  /api/v1/handoff/claims/{claim} 표를 들고 온 쪽에 문서를 내준다 (표가 곧 자격)
//
// 서비스끼리 서로의 자격 증명을 들고 있지 않는다. 표는 256비트 난수이고 5분
// 뒤에 죽으며 한 번 받아 가면 끝이다. 받는 쪽 브라우저에는
// <origin>/handoff?source=<orbit>&claim=<표> 가 열린다.

// handoffFormat 은 Orbit 이 내주는 유일한 형식이다.
const handoffFormat = "markdown"

// handoffClaimTTL 은 표의 수명이다. 표준의 상한이 5분이고, 브라우저는 표를
// 받는 그 순간 받는 쪽을 여니 그 아래로 내려갈 이유가 없다.
const handoffClaimTTL = 5 * time.Minute

// handoffFormats 는 표준의 형식 표 전체다. Orbit 은 markdown 만 보내지만,
// 관리자가 받는 쪽이 무엇을 받는지 적을 때 표준에 없는 낱말을 걸러 준다.
var handoffFormats = []string{"markdown", "docx", "csv", "xlsx", "txt", "pptx"}

// maxHandoffTargets 는 허용 목록의 크기 상한이다. 서비스는 여섯이다.
const maxHandoffTargets = 20

// maxHandoffStemBytes 는 파일 이름(확장자 제외)의 바이트 상한이다.
const maxHandoffStemBytes = 120

// handoffClaimRoute 는 자격이 경로에 실리는 유일한 곳이다. 로그에 이 경로가
// 남으면 표가 남는다.
const handoffClaimRoute = "/api/v1/handoff/claims/"

type HandoffSettings struct {
	Targets []HandoffTarget `json:"targets"`
}

// HandoffTarget 은 관리자가 적은 서비스 하나다. Formats 는 그 서비스가 받는
// 형식(표준의 낱말)이라, Orbit 이 보내는 것을 받을 수 있을 때만 단추에 오른다.
type HandoffTarget struct {
	Name    string   `json:"name"`
	Origin  string   `json:"origin"`
	Formats []string `json:"formats"`
}

// parseOrigin 은 스킴과 호스트뿐인 주소만 받는다. 경로·쿼리·자격 증명이 붙은
// 값은 오리진이 아니다. 받는 쪽은 source 를 자기 목록과 글자 그대로 대조하므로
// 표기를 여기서 하나로 맞춘다.
func parseOrigin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("http(s)://호스트 형태의 오리진이어야 합니다")
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil || u.Opaque != "" {
		return "", errors.New("오리진에는 경로·쿼리·자격 증명이 올 수 없습니다")
	}
	return strings.ToLower(u.Scheme + "://" + u.Host), nil
}

// validateHandoffSettings 는 관리 화면에서 온 목록의 규칙이다: 이름이 있고,
// 주소는 오리진이고, 형식은 표준의 낱말이며, 같은 주소가 두 번 없다.
// 저장되는 값은 다듬어진 뒤의 것이다.
func validateHandoffSettings(v *HandoffSettings) error {
	if v.Targets == nil {
		v.Targets = []HandoffTarget{}
	}
	if len(v.Targets) > maxHandoffTargets {
		return fmt.Errorf("보낼 곳은 %d개까지 둘 수 있습니다.", maxHandoffTargets)
	}
	seen := map[string]bool{}
	for i := range v.Targets {
		target := &v.Targets[i]
		target.Name = strings.TrimSpace(target.Name)
		if target.Name == "" {
			return fmt.Errorf("%d번째 보낼 곳의 이름이 필요합니다.", i+1)
		}
		origin, err := parseOrigin(target.Origin)
		if err != nil {
			return fmt.Errorf("%s의 주소: %s", target.Name, err)
		}
		if seen[origin] {
			return fmt.Errorf("같은 주소가 두 번 있습니다: %s", origin)
		}
		seen[origin] = true
		target.Origin = origin
		if len(target.Formats) == 0 {
			return fmt.Errorf("%s이(가) 받는 형식을 하나 이상 골라 주세요.", target.Name)
		}
		for _, format := range target.Formats {
			if !contains(handoffFormats, format) {
				return fmt.Errorf("%s의 형식 %q은(는) 표준에 없는 형식입니다.", target.Name, format)
			}
		}
	}
	return nil
}

// handoffTargetsFor 는 저장된 목록 가운데 Orbit 이 보내는 것을 받을 수 있는
// 곳만 남긴다. pptx 만 받는 곳은 실제 서비스이고 실제 항목이지만, 마크다운이
// 갈 곳은 아니다.
func handoffTargetsFor(cfg HandoffSettings) []HandoffTarget {
	out := []HandoffTarget{}
	for _, target := range cfg.Targets {
		if !contains(target.Formats, handoffFormat) {
			continue
		}
		origin, err := parseOrigin(target.Origin)
		if err != nil {
			continue
		}
		out = append(out, HandoffTarget{Name: strings.TrimSpace(target.Name), Origin: origin, Formats: target.Formats})
	}
	return out
}

// handoffSource 는 받는 쪽이 표를 받아 갈 주소다: 관리자가 적은 공개 URL 의
// 오리진이고, 아직 없으면 이 요청이 들어온 주소다. 받는 쪽은 어느 쪽이든
// 자기 목록과 대조하므로 잘못된 값은 닫히는 쪽으로만 실패한다.
func (s *Server) handoffSource(r *http.Request) string {
	var general struct {
		PublicURL string `json:"public_url"`
	}
	_ = s.readSetting(r.Context(), "system", "general", &general, nil)
	if origin, err := parseOrigin(general.PublicURL); err == nil {
		return origin
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// handoffTargets 는 화면의 물음에 답한다: 이 기억을 어디로, 누구로서 보낼 수
// 있는가. 목록이 비어 있으면 화면은 단추를 그리지 않는다.
func (s *Server) handoffTargets(w http.ResponseWriter, r *http.Request) {
	var cfg HandoffSettings
	if err := s.readSetting(r.Context(), "system", "handoff", &cfg, nil); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		internalError(w, r, err)
		return
	}
	targets := handoffTargetsFor(cfg)
	out := make([]map[string]string, 0, len(targets))
	for _, target := range targets {
		out = append(out, map[string]string{"name": target.Name, "origin": target.Origin})
	}
	writeJSON(w, 200, map[string]any{"targets": out, "source": s.handoffSource(r), "format": handoffFormat})
}

// issueHandoffClaim 은 기억 하나를 마크다운으로 만들고 표를 발급한다.
//
// 표는 이 사람이 읽을 수 있는 그 기억 하나에만 묶인다. 남의 기억은 없는 것과
// 같이 404 이고, 검토가 끝나지 않은 기억은 승인 절차를 건너뛰어 밖으로 나가지
// 않는다. 표를 만든다고 권한이 넓어지지 않는다.
func (s *Server) issueHandoffClaim(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	var in struct {
		Resource string `json:"resource"`
		Format   string `json:"format"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Format != handoffFormat {
		writeError(w, 400, "unsupported_format", "Orbit은 markdown 형식으로만 보낼 수 있습니다.")
		return
	}
	memoryID := strings.TrimSpace(in.Resource)
	if !looksLikeUUID(memoryID) {
		writeError(w, 400, "validation_error", "resource는 기억 id여야 합니다.")
		return
	}
	m, err := s.memoryByID(r, memoryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 404, "not_found", "기억을 찾을 수 없습니다.")
			return
		}
		internalError(w, r, err)
		return
	}
	if m.Status != "approved" {
		writeError(w, 409, "not_approved", "검토가 끝난 기억만 다른 서비스로 보낼 수 있습니다.")
		return
	}
	body := []byte(renderMemoryMarkdown(m))
	filename := handoffFilename(m.Title)
	contentType := "text/markdown; charset=utf-8"

	// 만료된 행은 더 이상 누구의 것도 아니다. 표를 발급할 때만 표가 늘어나므로
	// 따로 스케줄러를 두지 않고 여기서 치운다.
	_, _ = s.store.DB.Exec(r.Context(), `DELETE FROM handoff_claims WHERE expires_at < now() - interval '1 hour'`)
	claim := id.Token(32)
	expires := time.Now().Add(handoffClaimTTL)
	if _, err := s.store.DB.Exec(r.Context(), `INSERT INTO handoff_claims(claim_digest,memory_id,issued_by,filename,content_type,body,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, secure.SHA256(claim), m.ID, u.ID, filename, contentType, body, expires); err != nil {
		internalError(w, r, err)
		return
	}
	// 감사 기록에는 기억이 나갔다는 사실만 남는다. 표는 절대 넣지 않는다 —
	// 로그에 적힌 토큰은 토큰이다.
	s.audit(r.Context(), u.ID, "memory.handoff", "memory", m.ID, r.RemoteAddr, map[string]any{"format": handoffFormat, "bytes": len(body)})
	writeJSON(w, 201, map[string]any{
		"claim":        claim,
		"source":       s.handoffSource(r),
		"filename":     filename,
		"content_type": contentType,
		"bytes":        len(body),
		"expires_at":   expires.Format(time.RFC3339),
	})
}

// redeemHandoffClaim 은 표를 들고 온 쪽에 문서를 내준다.
//
// 로그인은 없다 — 표가 곧 자격이다. 그래서 짧고, 한 번이고, 기억 하나에 묶여
// 있다. 한 번 쓰이는 것은 DELETE 로 보장된다: 같은 표로 달려든 둘이 같은
// 문장을 실행해도 행을 돌려받는 것은 하나뿐이다. 쓴 표·만료된 표·발급된 적
// 없는 표는 모두 같은 404 다.
func (s *Server) redeemHandoffClaim(w http.ResponseWriter, r *http.Request) {
	claim := chi.URLParam(r, "claim")
	var filename, contentType string
	var body []byte
	err := s.store.DB.QueryRow(r.Context(), `DELETE FROM handoff_claims WHERE claim_digest=$1 AND expires_at > now() RETURNING filename,content_type,body`, secure.SHA256(claim)).Scan(&filename, &contentType, &body)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 404, "not_found", "표를 찾을 수 없습니다.")
			return
		}
		internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", attachmentDisposition(filename))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write(body)
}

// memoryByID 는 이 사용자의 기억 하나를 복호화해 돌려준다.
func (s *Server) memoryByID(r *http.Request, memoryID string) (Memory, error) {
	u := userFromContext(r.Context())
	var m Memory
	var cipher string
	var version int
	var topics []byte
	err := s.store.DB.QueryRow(r.Context(), `SELECT m.id,m.person_id,coalesce(p.display_name,''),m.title,m.content_cipher,m.key_version,m.occurred_at,m.source_type,m.source_reference,m.topics,m.status,m.created_at FROM memories m LEFT JOIN people p ON p.id=m.person_id WHERE m.id=$1 AND m.user_id=$2`, memoryID, u.ID).
		Scan(&m.ID, &m.PersonID, &m.PersonName, &m.Title, &cipher, &version, &m.OccurredAt, &m.SourceType, &m.SourceReference, &topics, &m.Status, &m.CreatedAt)
	if err != nil {
		return m, err
	}
	key, err := s.dataKeyVersion(r.Context(), u.ID, version)
	if err != nil {
		return m, err
	}
	if m.Content, err = s.store.Vault.Decrypt(key, cipher, "memory:"+m.ID+":content"); err != nil {
		return m, err
	}
	_ = json.Unmarshal(topics, &m.Topics)
	if m.Topics == nil {
		m.Topics = []string{}
	}
	return m, nil
}

// renderMemoryMarkdown 은 기억을 문서로 적는다: 제목, 누구와·언제·주제, 본문.
// 받는 쪽이 문서의 첫 줄부터 읽어도 되도록 id 나 상태 같은 살림살이는 넣지
// 않는다. 본문은 사람이 쓴 그대로 둔다 — 문단이 곧 문단이다.
func renderMemoryMarkdown(m Memory) string {
	var b strings.Builder
	b.WriteString("# " + oneLine(m.Title) + "\n\n")
	if m.PersonName != "" {
		b.WriteString("- 함께한 사람: " + oneLine(m.PersonName) + "\n")
	}
	when := m.CreatedAt
	if m.OccurredAt != nil {
		when = *m.OccurredAt
	}
	b.WriteString("- 언제: " + when.Format("2006-01-02") + "\n")
	if len(m.Topics) > 0 {
		b.WriteString("- 주제: " + strings.Join(m.Topics, ", ") + "\n")
	}
	b.WriteString("- 출처: Orbit\n\n")
	b.WriteString(strings.TrimSpace(m.Content))
	b.WriteString("\n")
	return b.String()
}

// handoffFilename 은 문서가 여행하는 이름이다. 화면에서 제목이던 글자가 다른
// 서비스가 디스크에 쓰는 순간 경로가 되므로, 경로가 될 수 있는 글자는 뺀다.
func handoffFilename(title string) string {
	stem := strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			return -1
		case r < 0x20 || r == 0x7f:
			return -1
		}
		return r
	}, oneLine(title))
	stem = strings.Trim(strings.TrimSpace(stem), ". ")
	// 파일 이름은 바이트 수로 제한되는 곳이 많다. 룬 중간에서 끊지 않는다.
	if len(stem) > maxHandoffStemBytes {
		cut := maxHandoffStemBytes
		for cut > 0 && !utf8.RuneStart(stem[cut]) {
			cut--
		}
		stem = strings.TrimSpace(stem[:cut])
	}
	if stem == "" {
		stem = "orbit-memory"
	}
	return stem + ".md"
}

// attachmentDisposition 은 RFC 6266 대로 두 이름을 함께 준다: 옛 클라이언트를
// 위한 ASCII 이름과, 한글 제목을 잃지 않는 UTF-8 이름.
func attachmentDisposition(filename string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return -1
		}
		return r
	}, filename)
	if strings.TrimSuffix(strings.TrimSpace(ascii), ".md") == "" {
		ascii = "orbit-memory.md"
	}
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, url.PathEscape(filename))
}

// oneLine 은 줄바꿈을 공백 하나로 접는다. 제목 한 줄이 두 줄이 되면 마크다운
// 머리말이 깨진다.
func oneLine(v string) string {
	return strings.Join(strings.Fields(v), " ")
}

func looksLikeUUID(v string) bool {
	if len(v) != 36 {
		return false
	}
	for i, r := range v {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
	}
	return true
}

// loggedPath 는 로그에 찍어도 되는 경로다. 표는 5분짜리 자격이고 로그 한 줄은
// 그보다 오래 산다.
func loggedPath(path string) string {
	if strings.HasPrefix(path, handoffClaimRoute) && len(path) > len(handoffClaimRoute) {
		return handoffClaimRoute + "{claim}"
	}
	return path
}
