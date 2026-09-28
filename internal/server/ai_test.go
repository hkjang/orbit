package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func streamAIRequest(personID string) *http.Request {
	body, err := json.Marshal(map[string]any{"prompt": "요즘 소원해진 사람이 있나요?", "person_id": personID})
	if err != nil {
		panic(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream", strings.NewReader(string(body)))
	return req.WithContext(context.WithValue(req.Context(), userContextKey, User{ID: "u1"}))
}

// relationshipContext는 본문 person_id를 uuid 컬럼인 p.id와 맞대므로, 모양이
// 어긋난 값이면 postgres가 22P02를 내고 ai.go의 internalError가 500으로 감싼다.
// 잘못 보낸 요청이니 400이어야 하고, 그 판단은 설정 조회보다 앞이라 store가
// nil이어도 패닉 없이 끝나야 한다.
func TestStreamAIRejectsMalformedPersonIDBeforeDB(t *testing.T) {
	cases := []struct{ name, personID string }{
		{"uuid가 아닌 문자열", "not-a-uuid"},
		{"하이픈 없는 16진수", "0e2f1c9a4b8d4e6fa1b2c3d4e5f60718"},
		{"중괄호로 감싼 형태", "{0e2f1c9a-4b8d-4e6f-a1b2-c3d4e5f60718}"},
		{"따옴표가 섞인 값", "1' OR '1'='1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, panicked := callWithoutStore((&Server{}).streamAI, streamAIRequest(tc.personID))
			if panicked {
				t.Fatalf("설정·질의까지 내려갔다 — person_id 검증이 DB 앞에서 끝나지 않는다")
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

// 정상 uuid와 빈 person_id(전체 관계 요약 경로)는 가드가 건드리지 않아야 한다.
// store가 nil이라 설정 조회(readSetting)에서 패닉이 나는 것이 곧 "가드를 지나
// 원래 가던 길로 갔다"는 증거다.
func TestStreamAILetsWellFormedPersonIDReachDB(t *testing.T) {
	cases := []struct{ name, personID string }{
		{"소문자 uuid", "0e2f1c9a-4b8d-4e6f-a1b2-c3d4e5f60718"},
		{"대문자 uuid", "0E2F1C9A-4B8D-4E6F-A1B2-C3D4E5F60718"},
		{"person_id 없음", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, panicked := callWithoutStore((&Server{}).streamAI, streamAIRequest(tc.personID))
			if !panicked {
				t.Fatalf("DB에 닿지 않고 %d로 끝났다 — 가드가 정상 id를 거부한다", rec.Code)
			}
		})
	}
}
