package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hkjang/orbit/internal/id"
)

// 미래 시각은 DB에 묻기 전에 400으로 돌려보낸다. store가 nil인 서버로 부르면
// 사람 존재 확인에 닿는 순간 패닉이 나므로, 통과하면 그 전에 멈춘 것이다.
// INSERT·recalculateRelationship·감사 로그도 모두 그 뒤에 있으니 함께 막힌다.
func TestCreateInteractionRejectsFutureOccurredAt(t *testing.T) {
	future := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	body := `{"kind":"meeting","occurred_at":"` + future + `","weight":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/people/p1/interactions", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, User{ID: "u1"}))
	rec := httptest.NewRecorder()

	(&Server{}).createInteraction(rec, req)

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
}

func TestValidateInteractionInput(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

	t.Run("과거 시각은 그대로 통과한다", func(t *testing.T) {
		past := now.Add(-30 * 24 * time.Hour)
		in := interactionInput{Kind: "call", OccurredAt: past, Weight: 3}
		if err := validateInteractionInput(&in, now); err != nil {
			t.Fatal(err)
		}
		if !in.OccurredAt.Equal(past) {
			t.Fatalf("시각이 바뀌었다: %v", in.OccurredAt)
		}
		if in.Weight != 3 {
			t.Fatalf("weight=%v, 3이어야 한다", in.Weight)
		}
	})

	t.Run("생략하면 현재로 채운다", func(t *testing.T) {
		in := interactionInput{Kind: "note"}
		if err := validateInteractionInput(&in, now); err != nil {
			t.Fatal(err)
		}
		if !in.OccurredAt.Equal(now) {
			t.Fatalf("occurred_at=%v, now로 채워야 한다", in.OccurredAt)
		}
	})

	t.Run("허용 오차 안의 미래는 통과한다", func(t *testing.T) {
		in := interactionInput{Kind: "message", OccurredAt: now.Add(interactionFutureSkew - time.Second)}
		if err := validateInteractionInput(&in, now); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("오차를 넘는 미래는 거부한다", func(t *testing.T) {
		in := interactionInput{Kind: "message", OccurredAt: now.Add(interactionFutureSkew + time.Second)}
		if err := validateInteractionInput(&in, now); err == nil {
			t.Fatal("오차를 넘는 미래는 거부해야 한다")
		}
	})

	t.Run("모르는 유형은 거부한다", func(t *testing.T) {
		in := interactionInput{Kind: "coffee", OccurredAt: now.Add(-time.Hour)}
		if err := validateInteractionInput(&in, now); err == nil {
			t.Fatal("화이트리스트 밖의 유형은 거부해야 한다")
		}
	})

	t.Run("범위 밖 weight는 1로 보정한다", func(t *testing.T) {
		for _, w := range []float64{0, -2, 10.5} {
			in := interactionInput{Kind: "other", OccurredAt: now.Add(-time.Hour), Weight: w}
			if err := validateInteractionInput(&in, now); err != nil {
				t.Fatal(err)
			}
			if in.Weight != 1 {
				t.Fatalf("weight %v가 1로 보정되지 않았다: %v", w, in.Weight)
			}
		}
	})
}

// personRequest는 chi 라우트 파라미터를 실어 준다. 실제 라우팅을 거치지 않고
// 핸들러를 직접 부르면 chi.URLParam은 빈 문자열을 주므로, 라우트 컨텍스트를
// 손수 넣어야 프로덕션과 같은 값이 핸들러에 들어간다.
func personRequest(t *testing.T, method, target, body string, params map[string]string) *http.Request {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, userContextKey, User{ID: "u1"})
	return req.WithContext(ctx)
}

func assertAPIError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("상태가 %d, %d이어야 한다 (본문: %s)", rec.Code, status, rec.Body.String())
	}
	var out apiError
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Error.Code != code || out.Error.Message == "" {
		t.Fatalf("응답 본문이 다르다: %+v", out)
	}
}

// uuid 모양이 아닌 경로 파라미터는 DB에 묻기 전에 404로 끝나야 한다.
// store가 nil인 서버로 부르므로, 질의에 닿으면 패닉으로 빨개진다.
func TestPersonHandlersRejectMalformedPathIDBeforeDB(t *testing.T) {
	const bad = "not-a-uuid"
	good := id.New()
	s := &Server{store: nil}

	t.Run("getPerson", func(t *testing.T) {
		rec := httptest.NewRecorder()
		s.getPerson(rec, personRequest(t, http.MethodGet, "/api/v1/people/"+bad, "", map[string]string{"personID": bad}))
		assertAPIError(t, rec, http.StatusNotFound, "not_found")
	})

	t.Run("updatePerson", func(t *testing.T) {
		rec := httptest.NewRecorder()
		body := `{"display_name":"홍길동","importance":0.5}`
		s.updatePerson(rec, personRequest(t, http.MethodPut, "/api/v1/people/"+bad, body, map[string]string{"personID": bad}))
		assertAPIError(t, rec, http.StatusNotFound, "not_found")
	})

	t.Run("deletePerson", func(t *testing.T) {
		rec := httptest.NewRecorder()
		s.deletePerson(rec, personRequest(t, http.MethodDelete, "/api/v1/people/"+bad, "", map[string]string{"personID": bad}))
		assertAPIError(t, rec, http.StatusNotFound, "not_found")
	})

	t.Run("listPersonLinks", func(t *testing.T) {
		rec := httptest.NewRecorder()
		s.listPersonLinks(rec, personRequest(t, http.MethodGet, "/api/v1/people/"+bad+"/links", "", map[string]string{"personID": bad}))
		assertAPIError(t, rec, http.StatusNotFound, "not_found")
	})

	t.Run("createPersonLink", func(t *testing.T) {
		rec := httptest.NewRecorder()
		body := `{"person_id":"` + good + `","kind":"knows"}`
		s.createPersonLink(rec, personRequest(t, http.MethodPost, "/api/v1/people/"+bad+"/links", body, map[string]string{"personID": bad}))
		assertAPIError(t, rec, http.StatusNotFound, "not_found")
	})

	t.Run("deletePersonLink의 linkID", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := personRequest(t, http.MethodDelete, "/api/v1/people/"+good+"/links/"+bad, "", map[string]string{"personID": good, "linkID": bad})
		s.deletePersonLink(rec, req)
		assertAPIError(t, rec, http.StatusNotFound, "not_found")
	})
}

// 본문 필드는 호출자가 고칠 수 있는 입력이므로 400이다.
func TestCreatePersonLinkRejectsMalformedBodyPersonID(t *testing.T) {
	good := id.New()
	rec := httptest.NewRecorder()
	body := `{"person_id":"not-a-uuid","kind":"knows"}`
	req := personRequest(t, http.MethodPost, "/api/v1/people/"+good+"/links", body, map[string]string{"personID": good})

	(&Server{store: nil}).createPersonLink(rec, req)

	assertAPIError(t, rec, http.StatusBadRequest, "validation_error")
}

func TestLooksLikeUUID(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"id.New()가 만든 값", id.New(), true},
		{"고정된 정상 uuid", "c4329119-1f2a-4b6d-8f0e-6bb9bd380a11", true},
		{"대문자 16진수도 같은 id다", "C4329119-1F2A-4B6D-8F0E-6BB9BD380A11", true},
		{"빈 문자열", "", false},
		{"중괄호로 감싼 것", "{c4329119-1f2a-4b6d-8f0e-6bb9bd380a11}", false},
		{"하이픈이 없는 것", "c43291191f2a4b6d8f0e6bb9bd380a11", false},
		{"길이가 짧은 것", "c4329119-1f2a-4b6d-8f0e-6bb9bd380a1", false},
		{"길이가 긴 것", "c4329119-1f2a-4b6d-8f0e-6bb9bd380a111", false},
		{"하이픈 자리가 어긋난 것", "c432911-91f2a-4b6d-8f0e-6bb9bd380a11", false},
		{"16진수가 아닌 글자", "c4329119-1f2a-4b6d-8f0e-6bb9bd380zzz", false},
		{"SQL을 섞은 것", "1' OR '1'='1", false},
		{"공백이 붙은 것", " c4329119-1f2a-4b6d-8f0e-6bb9bd380a11", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeUUID(tc.value); got != tc.want {
				t.Fatalf("looksLikeUUID(%q)=%v, %v이어야 한다", tc.value, got, tc.want)
			}
		})
	}
}

// malformedPersonIDs는 uuid 컬럼이 받아 주지 않는 네 가지 모양이다.
// workflow_test.go의 같은 목록과 짝을 이룬다 — 두 핸들러가 같은 입력을
// 같은 자리에서 걸러내는지를 함께 고정한다.
var malformedPersonIDs = []struct{ name, personID string }{
	{"uuid가 아닌 문자열", "not-a-uuid"},
	{"하이픈 없는 16진수", "0e2f1c9a4b8d4e6fa1b2c3d4e5f60718"},
	{"중괄호로 감싼 형태", "{0e2f1c9a-4b8d-4e6f-a1b2-c3d4e5f60718}"},
	{"따옴표가 섞인 값", "1' OR '1'='1"},
}

// assertAPIErrorMessage는 코드뿐 아니라 메시지 문장까지 고정한다. 형제
// 핸들러들이 쓰는 리터럴과 한 글자도 달라지면 안 되는 자리에만 쓴다.
func assertAPIErrorMessage(t *testing.T, rec *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("상태가 %d, %d이어야 한다 (본문: %s)", rec.Code, status, rec.Body.String())
	}
	var out apiError
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Error.Code != code {
		t.Fatalf("오류 코드가 %q, %q이어야 한다", out.Error.Code, code)
	}
	if out.Error.Message != message {
		t.Fatalf("메시지가 %q, %q이어야 한다", out.Error.Message, message)
	}
}

// 교류 생성의 personID는 uuid 컬럼(people.id)에 그대로 내려간다. 모양이
// 어긋난 값이 DB까지 가면 postgres 22P02가 나는데, 지금은 그 오류가 사람
// 부재와 한 묶음이라 404로 덮인다. 오류 분기를 가르면 같은 22P02가 500으로
// 승격되므로, 형제 핸들러 다섯 곳과 같은 모양의 가드가 DB 앞에 있어야 한다.
func TestCreateInteractionRejectsMalformedPersonIDBeforeDB(t *testing.T) {
	const body = `{"kind":"meeting","summary":"x"}`
	for _, tc := range malformedPersonIDs {
		t.Run(tc.name, func(t *testing.T) {
			// 핸들러가 읽는 것은 라우트 파라미터이므로 거기에는 날값을 넣는다.
			// 경로 문자열은 httptest.NewRequest가 파싱하므로 공백이 섞인 값은
			// 이스케이프해야 요청 자체를 만들 수 있다.
			req := personRequest(t, http.MethodPost, "/api/v1/people/"+url.PathEscape(tc.personID)+"/interactions", body, map[string]string{"personID": tc.personID})
			rec, panicked := callWithoutStore((&Server{}).createInteraction, req)
			if panicked {
				t.Fatalf("질의까지 내려갔다 — personID 검증이 DB 앞에서 끝나지 않는다")
			}
			assertAPIErrorMessage(t, rec, http.StatusNotFound, "not_found", "사람을 찾을 수 없습니다.")
		})
	}
}

// 가드가 정상 id를 함께 삼키면 교류를 아예 적을 수 없게 된다. 대문자
// 16진수도 postgres가 같은 id로 읽으므로 통과해야 한다. store가 nil이라
// 질의에서 패닉이 나는 것이 곧 "가드를 지나 DB까지 갔다"는 증거다.
func TestCreateInteractionLetsWellFormedPersonIDReachDB(t *testing.T) {
	const body = `{"kind":"meeting","summary":"x"}`
	cases := []struct{ name, personID string }{
		{"소문자 uuid", "0e2f1c9a-4b8d-4e6f-a1b2-c3d4e5f60718"},
		{"대문자 uuid", "0E2F1C9A-4B8D-4E6F-A1B2-C3D4E5F60718"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := personRequest(t, http.MethodPost, "/api/v1/people/"+tc.personID+"/interactions", body, map[string]string{"personID": tc.personID})
			rec, panicked := callWithoutStore((&Server{}).createInteraction, req)
			if !panicked {
				t.Fatalf("DB에 닿지 않고 %d로 끝났다 — 가드가 정상 id를 거부한다", rec.Code)
			}
		})
	}
}

// 입력 검증이 모양 가드보다 먼저 돈다는 순서를 고정한다. personID와 본문이
// 둘 다 잘못된 요청에서 호출자가 먼저 듣는 것은 본문 쪽 사유여야 한다 —
// 지금 순서가 그러하므로 가드를 validateInteractionInput 앞으로 올리면 안 된다.
func TestCreateInteractionValidatesInputBeforePersonIDGuard(t *testing.T) {
	const bad = "not-a-uuid"
	req := personRequest(t, http.MethodPost, "/api/v1/people/"+bad+"/interactions", `{"kind":"telepathy","summary":"x"}`, map[string]string{"personID": bad})

	rec, panicked := callWithoutStore((&Server{}).createInteraction, req)

	if panicked {
		t.Fatalf("질의까지 내려갔다 — 입력 검증이 DB 앞에서 끝나지 않는다")
	}
	assertAPIErrorMessage(t, rec, http.StatusBadRequest, "validation_error", "교류 유형을 확인해 주세요.")
}
