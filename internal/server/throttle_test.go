package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 로그인 문지기의 기본 성질: 한도까지는 열려 있고, 한도에 닿으면 닫히고,
// 시간이 지나면 다시 열린다.
func TestLoginThrottleLocksAfterRepeatedFailures(t *testing.T) {
	th := newLoginThrottle()
	now := time.Date(2026, 9, 17, 9, 0, 0, 0, time.UTC)
	key := loginKey("Admin", "10.0.0.7:51234")

	for i := 1; i < loginMaxFailures; i++ {
		if th.fail(key, now) {
			t.Fatalf("%d번째 실패에서 벌써 잠겼다", i)
		}
		if _, ok := th.blocked(key, now); ok {
			t.Fatalf("%d번째 실패 뒤에 막혀 있다", i)
		}
	}
	if !th.fail(key, now) {
		t.Fatalf("%d번째 실패에서 잠겨야 한다", loginMaxFailures)
	}
	wait, ok := th.blocked(key, now.Add(time.Minute))
	if !ok {
		t.Fatal("잠긴 직후에는 막혀 있어야 한다")
	}
	if wait != loginLockout-time.Minute {
		t.Fatalf("남은 시간이 틀리다: %v", wait)
	}
	if _, ok := th.blocked(key, now.Add(loginLockout)); ok {
		t.Fatal("잠금 시간이 지나면 열려야 한다")
	}
}

// 창 밖의 실패는 세지 않는다. 하루에 한 번 비밀번호를 틀리는 사람이
// 몇 주 뒤에 잠기면 안 된다.
func TestLoginThrottleForgetsOldFailures(t *testing.T) {
	th := newLoginThrottle()
	now := time.Date(2026, 9, 17, 9, 0, 0, 0, time.UTC)
	key := loginKey("admin", "10.0.0.7:1")
	for i := 1; i < loginMaxFailures; i++ {
		th.fail(key, now)
	}
	later := now.Add(loginWindow + time.Second)
	if th.fail(key, later) {
		t.Fatal("창이 지난 실패까지 더해 잠갔다")
	}
}

// 성공하면 셈을 지운다. 비밀번호를 몇 번 헷갈리다 맞춘 사람이 다음번에
// 한 번 틀렸다고 잠기면 안 된다.
func TestLoginThrottleResetOnSuccess(t *testing.T) {
	th := newLoginThrottle()
	now := time.Now()
	key := loginKey("admin", "10.0.0.7:1")
	for i := 1; i < loginMaxFailures; i++ {
		th.fail(key, now)
	}
	th.reset(key)
	if th.fail(key, now) {
		t.Fatal("성공 뒤 첫 실패에서 잠겼다")
	}
}

// 열쇠는 아이디와 주소를 함께 본다. 아이디 대소문자와 포트는 무시한다.
func TestLoginKeyScope(t *testing.T) {
	if loginKey("Admin", "10.0.0.7:1") != loginKey(" admin ", "10.0.0.7:2") {
		t.Fatal("같은 아이디·같은 주소는 같은 열쇠여야 한다")
	}
	if loginKey("admin", "10.0.0.7:1") == loginKey("admin", "10.0.0.8:1") {
		t.Fatal("주소가 다르면 다른 열쇠여야 한다 — 프록시 뒤 이웃이 함께 잠기지 않게")
	}
	if loginKey("admin", "10.0.0.7:1") == loginKey("alice", "10.0.0.7:1") {
		t.Fatal("아이디가 다르면 다른 열쇠여야 한다 — 남의 아이디로 나를 잠글 수 없게")
	}
	if loginKey("admin", "[::1]:8080") != loginKey("admin", "::1") {
		t.Fatal("IPv6 주소의 포트도 벗겨야 한다")
	}
}

// 잠긴 열쇠는 DB에 묻기 전에 429로 돌려보낸다. store가 nil인 서버로 부르면
// 조회에 닿는 순간 패닉이 나므로, 통과하면 그 전에 멈춘 것이다.
func TestLocalLoginRespondsTooManyRequestsWhileLocked(t *testing.T) {
	s := &Server{logins: newLoginThrottle()}
	key := loginKey("admin", "203.0.113.5")
	for i := 0; i < loginMaxFailures; i++ {
		s.logins.fail(key, time.Now())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"Admin","password":"x"}`))
	req.RemoteAddr = "203.0.113.5:40000"
	rec := httptest.NewRecorder()
	s.localLogin(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("상태가 %d, 429여야 한다", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got == "" || got == "0" {
		t.Fatalf("Retry-After 가 없다: %q", got)
	}
	var body apiError
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "too_many_attempts" || !strings.Contains(body.Error.Message, "분 뒤에") {
		t.Fatalf("응답 본문이 다르다: %+v", body)
	}
}

func TestWriteLoginBlockedRoundsUp(t *testing.T) {
	rec := httptest.NewRecorder()
	writeLoginBlocked(rec, 61*time.Second)
	if got := rec.Header().Get("Retry-After"); got != "61" {
		t.Fatalf("Retry-After=%q", got)
	}
	if !strings.Contains(rec.Body.String(), "2분") {
		t.Fatalf("61초는 2분으로 올려 말해야 한다: %s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	writeLoginBlocked(rec, 300*time.Millisecond)
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("1초 미만도 최소 1이어야 한다: %q", got)
	}
}

// 표가 커지면 지난 항목을 치운다. 공격자가 아이디를 바꿔 가며 표를 무한히
// 키우지 못하게 한다.
func TestLoginThrottleSweepsStaleEntries(t *testing.T) {
	th := newLoginThrottle()
	now := time.Date(2026, 9, 17, 9, 0, 0, 0, time.UTC)
	for i := 0; i < loginSweepAt; i++ {
		th.fail(loginKey(fmt.Sprintf("user%d", i), "10.0.0.1:1"), now)
	}
	if len(th.entries) != loginSweepAt {
		t.Fatalf("항목 %d개를 기대했으나 %d", loginSweepAt, len(th.entries))
	}
	th.fail(loginKey("late", "10.0.0.1:1"), now.Add(loginWindow+time.Second))
	if len(th.entries) != 1 {
		t.Fatalf("창이 지난 항목을 치워야 한다: %d개 남음", len(th.entries))
	}
}
