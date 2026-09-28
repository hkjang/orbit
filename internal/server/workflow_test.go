package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// callWithoutStore는 store가 nil인 서버로 핸들러를 부르고, 응답과 함께 "DB에
// 닿았는가"를 돌려준다. store가 nil이면 첫 질의에서 nil 포인터 패닉이 나므로,
// panicked가 거짓이라는 것은 그 핸들러가 DB 앞에서 판단을 끝냈다는 뜻이다.
// 손으로 만든 대역 없이 실제 핸들러를 그대로 부르므로, 가드가 실제 배선에
// 들어 있는지까지 함께 증명된다.
func callWithoutStore(h func(http.ResponseWriter, *http.Request), req *http.Request) (rec *httptest.ResponseRecorder, panicked bool) {
	rec = httptest.NewRecorder()
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	h(rec, req)
	return rec, false
}

func memoriesRequest(personID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/?person_id="+url.QueryEscape(personID), nil)
	return req.WithContext(context.WithValue(req.Context(), userContextKey, User{ID: "u1"}))
}

// memories.person_id는 uuid 컬럼이고 queryMemoriesLimit이 이 값을 uuid로
// 캐스팅하므로, 모양이 어긋난 값을 그대로 넘기면 postgres가 22P02를 내고
// 핸들러가 internalError로 감싸 500이 나간다. 호출자가 잘못 보낸 요청이니 400이어야 한다.
func TestListMemoriesRejectsMalformedPersonIDBeforeDB(t *testing.T) {
	cases := []struct{ name, personID string }{
		{"uuid가 아닌 문자열", "not-a-uuid"},
		{"하이픈 없는 16진수", "0e2f1c9a4b8d4e6fa1b2c3d4e5f60718"},
		{"중괄호로 감싼 형태", "{0e2f1c9a-4b8d-4e6f-a1b2-c3d4e5f60718}"},
		{"따옴표가 섞인 값", "1' OR '1'='1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, panicked := callWithoutStore((&Server{}).listMemories, memoriesRequest(tc.personID))
			if panicked {
				t.Fatalf("질의까지 내려갔다 — person_id 검증이 DB 앞에서 끝나지 않는다")
			}
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("상태가 %d, 400이어야 한다", rec.Code)
			}
			var out apiError
			if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
			if out.Error.Code != "validation_error" || out.Error.Message == "" {
				t.Fatalf("응답 본문이 다르다: %+v", out)
			}
		})
	}
}

// 가드가 정상 id를 함께 삼키면 지금 찾아지던 기억이 조용히 사라진다. 대문자
// 16진수도 postgres가 같은 id로 읽으므로 통과해야 하고, person_id가 비면
// 전체 목록 경로가 지금처럼 그대로 돌아야 한다. store가 nil이라 질의에서
// 패닉이 나는 것이 곧 "가드를 지나 DB까지 갔다"는 증거다.
func TestListMemoriesLetsWellFormedPersonIDReachDB(t *testing.T) {
	cases := []struct{ name, personID string }{
		{"소문자 uuid", "0e2f1c9a-4b8d-4e6f-a1b2-c3d4e5f60718"},
		{"대문자 uuid", "0E2F1C9A-4B8D-4E6F-A1B2-C3D4E5F60718"},
		{"person_id 없음", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, panicked := callWithoutStore((&Server{}).listMemories, memoriesRequest(tc.personID))
			if !panicked {
				t.Fatalf("DB에 닿지 않고 %d로 끝났다 — 가드가 정상 id를 거부한다", rec.Code)
			}
		})
	}
}
