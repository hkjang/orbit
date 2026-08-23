package server

import (
	"testing"
	"time"
)

// 웹의 web/src/orbitGrammar.test.ts와 같은 사례를 그대로 검증한다.
// 두 구현이 갈라지면 이 테스트가 먼저 깨진다.
func TestReadGrammarMatchesTheCanvasGrammar(t *testing.T) {
	now := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	ago := func(days int) *time.Time {
		at := now.AddDate(0, 0, -days)
		return &at
	}
	cases := []struct {
		name     string
		momentum float64
		last     *time.Time
		anchored bool
		want     RelationState
	}{
		{"교류가 늘면 다가오는 중", 0.42, ago(3), false, StateApproaching},
		{"작은 흔들림은 안정 궤도", 0.1, ago(10), false, StateStable},
		{"음의 흔들림도 안정 궤도", -0.12, ago(10), false, StateStable},
		{"교류가 줄면 멀어지는 중", -0.4, ago(20), false, StateDrifting},
		{"침묵만으로도 멀어지는 중", 0, ago(DriftDays + 1), false, StateDrifting},
		{"오래 침묵하면 다크 오빗", 0.9, ago(DormantDays + 5), false, StateDormant},
		{"교류 기록이 없으면 안정 궤도", 0, nil, false, StateStable},
		{"고정된 관계는 침묵해도 남는다", 0, ago(DormantDays + 90), true, StateStable},
		{"고정되어도 실제 쇠퇴는 드러난다", -0.5, ago(4), true, StateDrifting},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReadGrammar(tc.momentum, tc.last, tc.anchored, now); got != tc.want {
				t.Fatalf("상태가 %q, 기대 %q", got, tc.want)
			}
		})
	}
}

func TestEveryStateHasWordsForPeople(t *testing.T) {
	for _, state := range []RelationState{StateApproaching, StateStable, StateDrifting, StateDormant} {
		if StateLabel[state] == "" || StateHint[state] == "" {
			t.Fatalf("상태 %q에 사람이 읽을 이름이나 설명이 없다", state)
		}
	}
}

func TestDaysSinceReportsUnknownAsNegative(t *testing.T) {
	now := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	if got := DaysSince(nil, now); got != -1 {
		t.Fatalf("기록이 없으면 -1이어야 한다, got %d", got)
	}
	at := now.AddDate(0, 0, -7)
	if got := DaysSince(&at, now); got != 7 {
		t.Fatalf("7일이어야 한다, got %d", got)
	}
	future := now.AddDate(0, 0, 3)
	if got := DaysSince(&future, now); got != 0 {
		t.Fatalf("미래 시각은 0으로 눌러야 한다, got %d", got)
	}
}

// AI에게는 타임스탬프 대신 사람의 말을 넘긴다. 원시 시각을 주면 모델이 날짜
// 계산을 틀리기 쉽고, 답변에 내부 표현이 그대로 새어 나온다.
func TestDescribeLastInteractionSpeaksInHumanTime(t *testing.T) {
	now := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	ago := func(days int) *time.Time {
		at := now.AddDate(0, 0, -days)
		return &at
	}
	cases := []struct {
		at   *time.Time
		want string
	}{
		{nil, "아직 기록 없음"},
		{ago(0), "오늘"},
		{ago(9), "9일 전"},
		{ago(70), "약 2개월 전"},
		{ago(400), "약 1년 전"},
	}
	for _, tc := range cases {
		if got := describeLastInteraction(tc.at, now); got != tc.want {
			t.Fatalf("%q를 기대했으나 %q", tc.want, got)
		}
	}
}

// 검색어의 와일드카드는 글자 그대로 다뤄야 한다. 이 처리가 없으면 "100%"가
// "100"으로 시작하는 모든 것을 끌고 오고, 밑줄은 아무 글자나 대신한다.
func TestEscapeLikeKeepsWildcardsLiteral(t *testing.T) {
	cases := map[string]string{
		"김도현":        "김도현",
		"":           "",
		"100%":       `100\%`,
		"C_O":        `C\_O`,
		`back\slash`: `back\\slash`,
		"%_%":        `\%\_\%`,
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Fatalf("escapeLike(%q) = %q, 기대 %q", in, got, want)
		}
	}
}
