package server

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 로그인 무차별 대입 완화.
//
// 비밀번호 로그인은 bcrypt 비교 한 번이 전부라, 막지 않으면 사전을 그대로
// 흘려 넣을 수 있다. 같은 아이디를 같은 주소에서 계속 틀리면 잠시 문을 닫는다.
//
// 열쇠를 아이디와 주소를 합쳐 만드는 이유는 둘 다 따로 쓰면 탈이 나기
// 때문이다. 아이디만 보면 남의 아이디로 일부러 틀려 그 사람을 못 들어오게
// 할 수 있고, 주소만 보면 한 프록시 뒤의 모든 사용자가 함께 잠긴다.
//
// 상태는 메모리에만 둔다. 재시작하면 잊지만 그것으로 충분하다 — 목적은
// 시도 속도를 사람 손 수준으로 늦추는 것이지 영구 봉쇄가 아니다.
const (
	loginMaxFailures = 10
	loginWindow      = 15 * time.Minute
	loginLockout     = 15 * time.Minute
	// 오래된 항목은 실패를 기록할 때마다 조금씩 치운다. 표가 이만큼 커지면
	// 전체를 훑는다.
	loginSweepAt = 4096
)

type loginAttempts struct {
	failures int
	first    time.Time // 창의 시작. 이 시각부터 loginWindow 안의 실패만 센다.
	until    time.Time // 비어 있지 않으면 이 시각까지 막는다.
}

type loginThrottle struct {
	mu      sync.Mutex
	entries map[string]*loginAttempts
}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{entries: map[string]*loginAttempts{}}
}

// loginKey는 아이디와 요청 주소로 열쇠를 만든다. 아이디는 로그인 조회와
// 같은 규칙(대소문자 무시)으로 접는다.
func loginKey(username, remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return strings.ToLower(strings.TrimSpace(username)) + "\x00" + host
}

// blocked는 지금 막혀 있으면 남은 시간을 돌려준다.
func (t *loginThrottle) blocked(key string, now time.Time) (time.Duration, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.entries[key]
	if e == nil || e.until.IsZero() || !now.Before(e.until) {
		return 0, false
	}
	return e.until.Sub(now), true
}

// fail은 실패를 하나 더한다. 이번 실패로 문이 닫혔으면 true를 돌려준다.
func (t *loginThrottle) fail(key string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.entries) >= loginSweepAt {
		t.sweep(now)
	}
	e := t.entries[key]
	if e == nil || now.Sub(e.first) > loginWindow {
		e = &loginAttempts{first: now}
		t.entries[key] = e
	}
	e.failures++
	if e.failures < loginMaxFailures {
		return false
	}
	e.until = now.Add(loginLockout)
	// 잠금이 풀린 뒤 곧바로 다시 잠기지 않도록 셈을 새로 시작한다.
	e.failures = 0
	e.first = now
	return true
}

// reset은 로그인이 성공하면 셈을 지운다.
func (t *loginThrottle) reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, key)
}

// writeLoginBlocked는 429와 함께 언제 다시 올 수 있는지 알려준다.
// 초는 올림해서 Retry-After 를 지킨 클라이언트가 문이 열리기 전에 오지 않게 한다.
func writeLoginBlocked(w http.ResponseWriter, wait time.Duration) {
	seconds := int(math.Ceil(wait.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	minutes := (seconds + 59) / 60
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeError(w, http.StatusTooManyRequests, "too_many_attempts", fmt.Sprintf("로그인 시도가 너무 많습니다. %d분 뒤에 다시 시도해 주세요.", minutes))
}

// sweep은 창도 지나고 잠금도 풀린 항목을 버린다. 잠금 상태에서 부른다.
func (t *loginThrottle) sweep(now time.Time) {
	for key, e := range t.entries {
		if now.Sub(e.first) > loginWindow && !now.Before(e.until) {
			delete(t.entries, key)
		}
	}
}
