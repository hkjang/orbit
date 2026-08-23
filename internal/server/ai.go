package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) streamAI(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	var in struct {
		Prompt          string `json:"prompt"`
		PersonID        string `json:"person_id"`
		MaxOutputTokens int    `json:"max_output_tokens"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Prompt = strings.TrimSpace(in.Prompt)
	if in.Prompt == "" || len([]rune(in.Prompt)) > 32000 {
		writeError(w, 400, "validation_error", "질문은 1~32,000자로 입력해 주세요.")
		return
	}
	var settings AISettings
	var secret string
	if err := s.readSetting(r.Context(), "ai", "provider", &settings, &secret); err != nil {
		internalError(w, r, err)
		return
	}
	settings.APIKey = secret
	if !settings.Enabled {
		writeError(w, 503, "ai_disabled", "관리자 설정에서 AI를 먼저 활성화해 주세요.")
		return
	}
	limit := settings.MaxOutputTokens
	if in.MaxOutputTokens > 0 && in.MaxOutputTokens < limit {
		limit = in.MaxOutputTokens
	}
	if limit > 262144 {
		limit = 262144
	}
	contextText, err := s.relationshipContext(r.Context(), u.ID, in.PersonID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "stream_unsupported", "스트리밍을 지원하지 않는 연결입니다.")
		return
	}
	sendSSE(w, "meta", map[string]any{"model": settings.Model, "max_output_tokens": limit})
	flusher.Flush()
	if err := s.proxyAIStream(r.Context(), w, flusher, settings, in.Prompt, contextText, limit); err != nil {
		sendSSE(w, "error", map[string]string{"message": "AI 응답을 완료하지 못했습니다."})
		flusher.Flush()
		return
	}
	sendSSE(w, "done", map[string]bool{"ok": true})
	flusher.Flush()
	s.audit(r.Context(), u.ID, "ai.invoke", "ai", "", r.RemoteAddr, map[string]any{"model": settings.Model, "person_context": in.PersonID != "", "stream": true})
}

func (s *Server) relationshipContext(ctx context.Context, userID, personID string) (string, error) {
	var b strings.Builder
	b.WriteString("Orbit 관계 기록 (이 기록에 없는 사실은 모른다고 답하세요):\n")
	if personID != "" {
		var name, label string
		var momentum float64
		var last *time.Time
		var anchored bool
		err := s.store.DB.QueryRow(ctx, `SELECT p.display_name,r.relationship_label,r.momentum,r.last_interaction_at,r.anchored FROM people p JOIN relationships r ON r.person_id=p.id WHERE p.id=$1 AND p.user_id=$2`, personID, userID).Scan(&name, &label, &momentum, &last, &anchored)
		if err != nil {
			return "", err
		}
		// 내부 수치는 넘기지 않는다. 넘기면 AI가 그대로 되읊어 "활성도 0.31" 같은
		// 점수가 사용자에게 노출되고, 관계를 점수로 평가하지 않는다는 전제가 깨진다.
		state := ReadGrammar(momentum, last, anchored, time.Now())
		fmt.Fprintf(&b, "인물: %s, 관계: %s, 궤도 상태: %s(%s), 마지막 교류: %s\n",
			name, label, StateLabel[state], StateHint[state], describeLastInteraction(last, time.Now()))
		if anchored {
			b.WriteString("이 관계는 고정되어 있습니다. 교류가 뜸해져도 곁에 두기로 한 사이입니다.\n")
		}
		connections, err := s.personConnections(ctx, userID, personID)
		if err != nil {
			return "", err
		}
		if len(connections) > 0 {
			fmt.Fprintf(&b, "이 사람과 이어진 사람들: %s\n", strings.Join(connections, ", "))
		}
		memories, err := s.queryMemories(ctx, userID, personID, "approved")
		if err != nil {
			return "", err
		}
		for i, m := range memories {
			if i >= 30 || b.Len() > 180000 {
				break
			}
			fmt.Fprintf(&b, "- 기억 %s (%v): %s\n", m.Title, m.OccurredAt, m.Content)
		}
	} else {
		rows, err := s.store.DB.Query(ctx, `SELECT p.display_name,r.relationship_label,r.last_interaction_at,r.momentum,r.anchored FROM people p JOIN relationships r ON r.person_id=p.id WHERE p.user_id=$1 ORDER BY r.importance DESC LIMIT 80`, userID)
		if err != nil {
			return "", err
		}
		defer rows.Close()
		now := time.Now()
		for rows.Next() {
			var name, label string
			var last *time.Time
			var momentum float64
			var anchored bool
			if err := rows.Scan(&name, &label, &last, &momentum, &anchored); err != nil {
				return "", err
			}
			state := ReadGrammar(momentum, last, anchored, now)
			fmt.Fprintf(&b, "- %s / %s / %s / 마지막 교류 %s\n",
				name, label, StateLabel[state], describeLastInteraction(last, now))
		}
		if err := rows.Err(); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

// describeLastInteraction은 시각을 사람의 말로 바꾼다.
// 타임스탬프를 그대로 넘기면 AI가 날짜 계산을 틀리기 쉽다.
func describeLastInteraction(at *time.Time, now time.Time) string {
	days := DaysSince(at, now)
	switch {
	case days < 0:
		return "아직 기록 없음"
	case days == 0:
		return "오늘"
	case days < 30:
		return fmt.Sprintf("%d일 전", days)
	case days < 365:
		return fmt.Sprintf("약 %d개월 전", days/30)
	default:
		return fmt.Sprintf("약 %d년 전", days/365)
	}
}

// personConnections는 그 사람과 이어진 다른 사람들을 사람의 말로 돌려준다.
func (s *Server) personConnections(ctx context.Context, userID, personID string) ([]string, error) {
	rows, err := s.store.DB.Query(ctx, `SELECT p.display_name,l.kind FROM person_links l JOIN people p ON p.id = CASE WHEN l.person_a=$2 THEN l.person_b ELSE l.person_a END WHERE l.user_id=$1 AND $2 IN (l.person_a,l.person_b) ORDER BY l.strength DESC LIMIT 20`, userID, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var name, kind string
		if err := rows.Scan(&name, &kind); err != nil {
			return nil, err
		}
		if label := linkKinds[kind]; label != "" {
			out = append(out, fmt.Sprintf("%s(%s)", name, label))
		} else {
			out = append(out, name)
		}
	}
	return out, rows.Err()
}

func (s *Server) proxyAIStream(ctx context.Context, w io.Writer, flusher http.Flusher, settings AISettings, prompt, relationshipContext string, maxTokens int) error {
	endpoint := strings.TrimRight(settings.BaseURL, "/")
	if !strings.HasSuffix(endpoint, "/responses") {
		if strings.HasSuffix(endpoint, "/v1") {
			endpoint += "/responses"
		} else {
			endpoint += "/v1/responses"
		}
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("invalid AI endpoint")
	}
	body := map[string]any{"model": settings.Model, "instructions": settings.SystemPrompt, "input": relationshipContext + "\n\n사용자 질문:\n" + prompt, "max_output_tokens": maxTokens, "stream": true, "store": false}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if settings.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+settings.APIKey)
	}
	timeout := time.Duration(settings.RequestTimeoutSeconds) * time.Second
	if timeout < 5*time.Second {
		timeout = 120 * time.Second
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext, ForceAttemptHTTP2: true, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second}
	client := &http.Client{Transport: transport, Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("provider status %d: %s", resp.StatusCode, strings.TrimSpace(string(limited)))
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	event := ""
	dataLines := []string{}
	flushEvent := func() {
		if len(dataLines) == 0 {
			return
		}
		data := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		if data == "[DONE]" {
			return
		}
		var payload map[string]any
		if json.Unmarshal([]byte(data), &payload) != nil {
			return
		}
		delta := extractDelta(event, payload)
		if delta != "" {
			sendSSE(w, "delta", map[string]string{"text": delta})
			flusher.Flush()
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flushEvent()
			event = ""
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	flushEvent()
	return scanner.Err()
}

func extractDelta(event string, payload map[string]any) string {
	if event == "response.output_text.delta" || payload["type"] == "response.output_text.delta" {
		if v, ok := payload["delta"].(string); ok {
			return v
		}
	}
	if choices, ok := payload["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if delta, ok := choice["delta"].(map[string]any); ok {
				if content, ok := delta["content"].(string); ok {
					return content
				}
			}
		}
	}
	return ""
}
func sendSSE(w io.Writer, event string, value any) {
	raw, _ := json.Marshal(value)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
}
func safeAIError(err error) string {
	message := err.Error()
	if len(message) > 300 {
		message = message[:300]
	}
	return message
}
