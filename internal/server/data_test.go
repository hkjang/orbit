package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
