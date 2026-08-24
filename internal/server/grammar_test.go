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

// 기억 검색은 제목만이 아니라 본문·주제·인물까지 훑어야 한다.
// 본문은 암호문이라 SQL이 아니라 복호화 뒤 Go에서 거른다.
func TestMatchesMemoryLooksEverywhere(t *testing.T) {
	m := Memory{
		Title:      "제주 워크숍",
		Content:    "바닷가에서 로드맵을 정리했다",
		PersonName: "김도현",
		Topics:     []string{"전략", "Offsite"},
	}
	for _, needle := range []string{"제주", "로드맵", "김도현", "전략", "offsite"} {
		if !matchesMemory(m, needle) {
			t.Fatalf("%q로 찾지 못했다", needle)
		}
	}
	for _, needle := range []string{"부산", "예산"} {
		if matchesMemory(m, needle) {
			t.Fatalf("%q는 걸리면 안 된다", needle)
		}
	}
}

func TestMatchesMemoryIgnoresCase(t *testing.T) {
	m := Memory{Title: "Alpha Kickoff", Topics: []string{}}
	if !matchesMemory(m, "alpha") {
		t.Fatal("대소문자와 무관하게 찾아야 한다")
	}
}

// 소속·주제 꼬리표는 저장 직전에 다듬는다. 다듬지 않으면 "가족"과 "가족 "이
// 서로 다른 소속이 되어, 화면에서는 같아 보이는데 색과 묶음이 갈린다.
func TestNormalizeTagsTidiesFreeformLabels(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"앞뒤 공백을 뗀다", []string{"  가족 ", "친구"}, []string{"가족", "친구"}},
		{"빈 값은 버린다", []string{"가족", "", "   "}, []string{"가족"}},
		{"같은 것은 한 번만", []string{"가족", "가족", "친구"}, []string{"가족", "친구"}},
		{"공백만 다른 것도 같은 것으로", []string{"가족", " 가족"}, []string{"가족"}},
		{"적은 순서를 지킨다", []string{"친구", "가족", "멘토"}, []string{"친구", "가족", "멘토"}},
		{"가운데 공백은 그대로 둔다", []string{"AI 추진단"}, []string{"AI 추진단"}},
		{"빈 목록", []string{}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeTags(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("%v를 기대했으나 %v", tc.want, got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("%v를 기대했으나 %v", tc.want, got)
				}
			}
		})
	}
}

// 개수 제한은 다듬은 뒤에 세야 한다. 중복 때문에 한도에 걸리면 사용자는
// 왜 막히는지 알 수 없다.
func TestPersonInputCountsCategoriesAfterTidying(t *testing.T) {
	in := &personInput{DisplayName: "김도현", Importance: .5}
	for i := 0; i < 20; i++ {
		in.Categories = append(in.Categories, " 가족 ")
	}
	if err := validatePersonInput(in); err != nil {
		t.Fatalf("같은 소속 스무 번은 하나로 줄어야 한다: %v", err)
	}
	if len(in.Categories) != 1 || in.Categories[0] != "가족" {
		t.Fatalf("소속이 %v", in.Categories)
	}
}

func TestRelationshipMetricsReadRecentContact(t *testing.T) {
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	ago := func(days int) time.Time { return now.AddDate(0, 0, -days) }

	t.Run("교류가 없으면 모두 0", func(t *testing.T) {
		score, closeness, momentum := relationshipMetrics(nil, now)
		if score != 0 || closeness != 0 || momentum != 0 {
			t.Fatalf("0을 기대했으나 %v %v %v", score, closeness, momentum)
		}
	})

	t.Run("최근 교류가 옛 교류보다 친밀도를 크게 올린다", func(t *testing.T) {
		_, recentCloseness, _ := relationshipMetrics(
			[]interactionPoint{{ago(3), 1}}, now)
		_, oldCloseness, _ := relationshipMetrics(
			[]interactionPoint{{ago(300), 1}}, now)
		if recentCloseness <= oldCloseness {
			t.Fatalf("최근이 더 커야 한다: %v vs %v", recentCloseness, oldCloseness)
		}
	})

	t.Run("교류가 늘면 momentum이 양수", func(t *testing.T) {
		_, _, momentum := relationshipMetrics([]interactionPoint{
			{ago(5), 1}, {ago(20), 1}, {ago(80), 1},
		}, now)
		if momentum <= 0 {
			t.Fatalf("양수를 기대했으나 %v", momentum)
		}
	})

	t.Run("교류가 줄면 momentum이 음수", func(t *testing.T) {
		_, _, momentum := relationshipMetrics([]interactionPoint{
			{ago(60), 1}, {ago(70), 1}, {ago(85), 1},
		}, now)
		if momentum >= 0 {
			t.Fatalf("음수를 기대했으나 %v", momentum)
		}
	})

	// 스키마가 0~1만 받는다. 교류가 아주 많으면 1로 포화되는데, 그것이
	// "더 가까울 수 없다"는 뜻이므로 정상이다. 넘지만 않으면 된다.
	t.Run("친밀도는 0과 1 사이를 벗어나지 않는다", func(t *testing.T) {
		many := make([]interactionPoint, 500)
		for i := range many {
			many[i] = interactionPoint{ago(1), 10}
		}
		_, closeness, _ := relationshipMetrics(many, now)
		if closeness < 0 || closeness > 1 {
			t.Fatalf("0~1 범위를 벗어났다: %v", closeness)
		}
	})

	// 사용자가 앞날 일정을 미리 적어 둘 수 있다. 경과 일수가 음수가 되어도
	// 지수가 폭주하지 않도록 0으로 눌러 둔다.
	t.Run("미래 시각도 폭주하지 않는다", func(t *testing.T) {
		_, closeness, _ := relationshipMetrics(
			[]interactionPoint{{now.AddDate(0, 0, 30), 1}}, now)
		if closeness < 0 || closeness > 1 {
			t.Fatalf("0~1 범위를 벗어났다: %v", closeness)
		}
	})
}
