package server

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/hkjang/orbit/internal/id"
)

// api_keys.id는 uuid 컬럼(001_init.sql)이라, 모양이 어긋난 keyID를 그대로
// UPDATE에 넘기면 postgres 22P02가 나고 핸들러가 internalError로 감싸
// 500 internal_error + slog.Error가 된다. 호출자가 잘못 보낸 요청이므로
// 형제 핸들러들처럼 DB 앞에서 "없는 키"와 같은 404로 끝나야 한다.
// store가 nil인 서버로 부르므로, 질의에 닿으면 패닉으로 빨개진다.
func TestRevokeAPIKeyRejectsMalformedKeyIDBeforeDB(t *testing.T) {
	cases := []struct{ name, keyID string }{
		{"uuid가 아닌 문자열", "not-a-uuid"},
		{"공백뿐인 값", " "},
		{"중괄호로 감싼 형태", "{0e2f1c9a-4b8d-4e6f-a1b2-c3d4e5f60718}"},
		{"하이픈 없는 16진수", "0e2f1c9a4b8d4e6fa1b2c3d4e5f60718"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, panicked := callWithoutStore((&Server{store: nil}).revokeAPIKey, apiKeyRequest(t, tc.keyID))
			if panicked {
				t.Fatalf("질의까지 내려갔다 — keyID 검증이 DB 앞에서 끝나지 않는다")
			}
			assertAPIErrorMessage(t, rec, http.StatusNotFound, "not_found", "활성 API 키를 찾을 수 없습니다.")
		})
	}
}

// 가드가 정상 id를 함께 삼키면 지금 폐기되던 키를 폐기할 수 없게 된다.
// 대문자 16진수도 postgres가 같은 id로 읽으므로 통과해야 한다. store가
// nil이라 질의에서 패닉이 나는 것이 곧 "가드를 지나 DB까지 갔다"는 증거다.
func TestRevokeAPIKeyLetsWellFormedKeyIDReachDB(t *testing.T) {
	good := id.New()
	cases := []struct{ name, keyID string }{
		{"id.New()가 만든 값", good},
		{"대문자 uuid", strings.ToUpper(good)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, panicked := callWithoutStore((&Server{store: nil}).revokeAPIKey, apiKeyRequest(t, tc.keyID))
			if !panicked {
				t.Fatalf("DB에 닿지 않고 %d로 끝났다 — 가드가 정상 id를 거부한다", rec.Code)
			}
		})
	}
}

// 경로에 공백·중괄호가 들면 httptest.NewRequest의 경로 파싱이 터지므로
// 경로만 escape하고 라우트 파라미터에는 날값을 넣는다 — 핸들러가 읽는 것은
// 후자다.
func apiKeyRequest(t *testing.T, keyID string) *http.Request {
	t.Helper()
	target := "/api/v1/personal/api-keys/" + url.PathEscape(keyID)
	return personRequest(t, http.MethodDelete, target, "", map[string]string{"keyID": keyID})
}
