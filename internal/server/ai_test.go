package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
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

// safeAIError는 제공자 오류를 slog로 내보내기 전에 길이를 자른다. 자르는 자리가
// 바이트 단위라 한국어(룬 3바이트) 오류가 300바이트 경계를 걸치면 마지막 룬이
// 쪼개져 꼬리가 U+FFFD로 깨진다 — 로그를 읽으려고 남기는 값이니 깨지면 안 된다.
// 반대로 300바이트 이하는 한 바이트도 손대지 않아야 한다(지금 동작 유지).
func TestSafeAIError(t *testing.T) {
	// 앞에 ASCII 한 글자를 두어 300이 룬 경계와 어긋나게 만든다. 한국어 룬은
	// 1+3k 바이트 자리에서 시작하므로 300번째 바이트는 어떤 룬의 중간이다.
	split := "x" + strings.Repeat("한", 150)
	if utf8.RuneStart(split[300]) {
		t.Fatalf("시험 입력이 300바이트에서 룬을 쪼개지 않는다 — 입력을 고쳐야 한다")
	}

	t.Run("룬을 쪼개지 않는다", func(t *testing.T) {
		got := safeAIError(errors.New(split))
		if !utf8.ValidString(got) {
			t.Fatalf("반환값이 올바른 UTF-8이 아니다: %q", got)
		}
		if strings.ContainsRune(got, utf8.RuneError) {
			t.Fatalf("반환값에 U+FFFD가 섞였다: %q", got)
		}
		if len(got) > 300 {
			t.Fatalf("길이가 %d바이트 — 300바이트를 넘는다", len(got))
		}
		// 자르기 전 온전한 룬은 그대로 남아야 한다(너무 많이 물러나지 않았는지).
		if len(got) < 300-utf8.UTFMax {
			t.Fatalf("길이가 %d바이트 — 룬 경계보다 더 물러났다", len(got))
		}
		if !strings.HasPrefix(split, got) {
			t.Fatalf("반환값이 입력의 접두사가 아니다: %q", got)
		}
	})

	t.Run("300바이트 이하는 그대로 돌려준다", func(t *testing.T) {
		cases := []string{
			"",
			"provider status 401: unauthorized",
			"제공자 오류: 인증에 실패했습니다",
			strings.Repeat("a", 300),
			"x" + strings.Repeat("한", 99) + "ab", // 정확히 300바이트
		}
		for _, in := range cases {
			if len(in) > 300 {
				t.Fatalf("시험 입력이 %d바이트 — 300 이하여야 한다", len(in))
			}
			if got := safeAIError(errors.New(in)); got != in {
				t.Fatalf("safeAIError(%q) = %q — 바뀌었다", in, got)
			}
		}
	})
}
