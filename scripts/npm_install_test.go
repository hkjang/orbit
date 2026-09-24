// scripts/npm-install.sh 의 재시도 동작을 고정한다.
//
// 이 시험이 왜 Go 시험인가: 이 저장소의 검증 파이프라인이 실제로 도는 것은
// `go test ./...` 와 vitest 둘뿐이다. 설치 스크립트는 npm 이 아직 없을 때도
// 도는 물건이라 vitest 로는 덮을 수 없고, 셸 시험 기반은 따로 없다. 그래서
// 가짜 `npm` 을 PATH 앞에 놓고 진짜 `sh` 로 스크립트를 실행해 — 문자열 검사가
// 아니라 실제 프로세스 실행으로 — 몇 번 부르는지와 종료 코드를 본다.
package scripts_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// 2026-09-23 회차를 두 번 연속으로 깨뜨린 진짜 오류 문구. 가짜 npm 이 이것을 낸다.
const registryTimeout = "npm error network Invalid response body while trying to fetch " +
	"https://registry.npmjs.org/tldts-core: read ETIMEDOUT"

// fakeNPM 은 앞의 failures 번은 레지스트리 시간 초과로 죽고 그 뒤로는 성공하는
// `npm` 을 PATH 앞에 심는다. 호출 횟수와 넘어온 인자를 파일로 돌려준다.
type fakeNPM struct {
	dir       string
	countPath string
	argsPath  string
}

func newFakeNPM(t *testing.T, failures int) *fakeNPM {
	t.Helper()
	f := &fakeNPM{dir: t.TempDir()}
	f.countPath = filepath.Join(f.dir, "count")
	f.argsPath = filepath.Join(f.dir, "args")
	if err := os.WriteFile(f.countPath, []byte("0"), 0o644); err != nil {
		t.Fatalf("호출 횟수 파일을 만들지 못했다: %v", err)
	}
	script := "#!/bin/sh\n" +
		"n=$(cat \"$FAKE_NPM_COUNT\")\n" +
		"n=$((n + 1))\n" +
		"printf '%s' \"$n\" > \"$FAKE_NPM_COUNT\"\n" +
		"printf '%s\\n' \"$*\" >> \"$FAKE_NPM_ARGS\"\n" +
		"if [ \"$n\" -le \"$FAKE_NPM_FAILURES\" ]; then\n" +
		"  echo '" + registryTimeout + "' >&2\n" +
		"  exit 1\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(f.dir, "npm"), []byte(script), 0o755); err != nil {
		t.Fatalf("가짜 npm 을 만들지 못했다: %v", err)
	}
	t.Setenv("FAKE_NPM_FAILURES", strconv.Itoa(failures))
	return f
}

// run 은 스크립트를 실제로 실행하고 종료 코드와 합쳐진 출력을 돌려준다.
func (f *fakeNPM) run(t *testing.T, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{"npm-install.sh"}, args...)...)
	cmd.Env = append(os.Environ(),
		"PATH="+f.dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_NPM_COUNT="+f.countPath,
		"FAKE_NPM_ARGS="+f.argsPath,
		"NPM_INSTALL_RETRY_DELAY=0", // 시험이 실제로 쉬지 않도록
	)
	out, err := cmd.CombinedOutput()
	code := 0
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	default:
		t.Fatalf("스크립트를 실행하지 못했다: %v (출력: %s)", err, out)
	}
	return code, string(out)
}

// calls 는 가짜 npm 이 불린 횟수다.
func (f *fakeNPM) calls(t *testing.T) int {
	t.Helper()
	b, err := os.ReadFile(f.countPath)
	if err != nil {
		t.Fatalf("호출 횟수를 읽지 못했다: %v", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatalf("호출 횟수가 숫자가 아니다(%q): %v", b, err)
	}
	return n
}

// 레지스트리가 잠깐 죽었다 살아나면 설치는 끝내 성공해야 한다.
// npm 자신의 fetch-retries 는 이 오류를 못 잡는다 — 본문을 읽던 중 난 오류는
// make-fetch-happen 의 재시도 래퍼 바깥에서 던져지기 때문이다. 그래서 명령
// 자체를 다시 부르는 것 말고는 이 실패를 넘길 방법이 없다.
func TestRetriesTransientRegistryFailure(t *testing.T) {
	f := newFakeNPM(t, 2)
	code, out := f.run(t)
	if code != 0 {
		t.Fatalf("세 번째 시도에서 성공해야 하는데 종료 코드 %d 였다\n%s", code, out)
	}
	if got := f.calls(t); got != 3 {
		t.Fatalf("npm 을 3번 불러야 하는데 %d번 불렀다\n%s", got, out)
	}
}

// 첫 시도가 성공하면 다시 부르지 않는다 — 평소 설치가 느려지면 안 된다.
func TestDoesNotRetryWhenFirstAttemptSucceeds(t *testing.T) {
	f := newFakeNPM(t, 0)
	code, out := f.run(t)
	if code != 0 {
		t.Fatalf("성공해야 하는데 종료 코드 %d 였다\n%s", code, out)
	}
	if got := f.calls(t); got != 1 {
		t.Fatalf("npm 을 1번만 불러야 하는데 %d번 불렀다\n%s", got, out)
	}
}

// 진짜로 못 고치는 실패(잠금 파일 불일치 등)는 끝내 실패해야 한다.
// 재시도가 검증을 무르게 만들면 안 되므로 횟수 상한과 0 아닌 종료 코드를 고정한다.
func TestFailsAfterAttemptLimit(t *testing.T) {
	f := newFakeNPM(t, 99)
	code, out := f.run(t)
	if code == 0 {
		t.Fatalf("계속 실패하면 0 이 아닌 코드로 끝나야 한다\n%s", out)
	}
	if got := f.calls(t); got != 3 {
		t.Fatalf("상한인 3번만 불러야 하는데 %d번 불렀다\n%s", got, out)
	}
}

// 잠금 파일을 그대로 쓰는 `npm ci` 여야 하고(설치가 잠금 파일을 고쳐 쓰면 안 된다),
// 호출자가 준 인자는 그대로 넘어가야 한다.
func TestPassesCIFlagsAndExtraArguments(t *testing.T) {
	f := newFakeNPM(t, 0)
	code, out := f.run(t, "--ignore-scripts")
	if code != 0 {
		t.Fatalf("성공해야 하는데 종료 코드 %d 였다\n%s", code, out)
	}
	b, err := os.ReadFile(f.argsPath)
	if err != nil {
		t.Fatalf("인자 기록을 읽지 못했다: %v", err)
	}
	got := strings.TrimSpace(string(b))
	want := "ci --no-audit --no-fund --ignore-scripts"
	if got != want {
		t.Fatalf("npm 인자가 %q 여야 하는데 %q 였다", want, got)
	}
}
