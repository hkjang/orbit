#!/bin/sh
# web 의존성을 잠금 파일 그대로 설치한다. 레지스트리가 잠깐 죽으면 다시 해 본다.
#
# 왜 재시도를 손으로 감싸는가 — npm 의 fetch-retries 로는 안 되기 때문이다.
# 2026-09-23 회차의 검증이 두 번 연속 이 오류 하나로 끝났다:
#
#   npm error network Invalid response body while trying to fetch
#   https://registry.npmjs.org/tldts-core: read ETIMEDOUT
#
# 이 문구는 minipass-fetch/lib/body.js 가 응답 **본문을 읽던 중** 내는 것이고,
# npm 의 재시도는 make-fetch-happen/lib/remote.js 의 promiseRetry 안, 즉 요청
# 단계에서만 돈다. 게다가 거기서도 RETRY_TYPES 가 'request-timeout' 하나뿐이라
# type 이 'system' 인 이 오류는 걸리지 않는다. 그래서 fetch-retries 를 아무리
# 올려도 설치는 첫 실패에서 그대로 죽는다(실제로 16초 만에 끝났다 — 기본
# 재시도 간격 10초를 한 번도 기다리지 않았다는 뜻이다).
# 넘길 방법은 npm ci 자체를 다시 부르는 것뿐이다.
#
# 재시도는 `npm ci` 로만 한다 — 잠금 파일을 고쳐 쓰는 `npm install` 로 물러서면
# 설치가 조용히 다른 의존성 트리를 만들어 검증이 무의미해진다. 횟수 상한이
# 있으므로 진짜 고장(잠금 파일 불일치 등)은 여전히 실패로 끝난다.
set -e

attempts=${NPM_INSTALL_ATTEMPTS:-3}
delay=${NPM_INSTALL_RETRY_DELAY:-5}

n=1
while :; do
	if npm ci --no-audit --no-fund "$@"; then
		exit 0
	fi
	if [ "$n" -ge "$attempts" ]; then
		echo "npm-install: ${n}번 시도했으나 모두 실패했다" >&2
		exit 1
	fi
	echo "npm-install: ${n}/${attempts}번째 시도 실패 — ${delay}초 뒤 다시 시도한다" >&2
	n=$((n + 1))
	sleep "$delay"
done
