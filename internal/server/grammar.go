package server

import "time"

// 궤도 문법(Visual Grammar)의 서버 쪽 정본.
//
// 웹의 web/src/orbitGrammar.ts와 같은 임계값을 쓴다. 캔버스가 "다크 오빗"이라고
// 부르는 관계를 AI가 "활성도 0.31"이라고 말하면 같은 제품처럼 보이지 않는다.
// 두 구현이 갈라지지 않도록 임계값은 상수로 두고 테스트로 고정한다.
const (
	// DormantDays를 넘기면 Event Horizon 바깥으로 나간다.
	DormantDays = 180
	// DriftDays를 넘기면 momentum과 무관하게 멀어지는 흐름으로 본다.
	DriftDays = 90
	// MomentumBand 안쪽의 momentum은 흔들림으로 보고 안정 궤도로 둔다.
	MomentumBand = 0.18
)

// RelationState는 관계가 지금 어디로 움직이는지를 나타낸다. 점수가 아니다.
type RelationState string

const (
	StateApproaching RelationState = "approaching"
	StateStable      RelationState = "stable"
	StateDrifting    RelationState = "drifting"
	StateDormant     RelationState = "dormant"
)

// StateLabel은 사람에게 보여줄 우리말 이름이다.
var StateLabel = map[RelationState]string{
	StateApproaching: "다가오는 중",
	StateStable:      "안정 궤도",
	StateDrifting:    "멀어지는 중",
	StateDormant:     "다크 오빗",
}

// StateHint는 그 상태가 무슨 뜻인지 한 문장으로 설명한다.
var StateHint = map[RelationState]string{
	StateApproaching: "최근 교류가 늘며 안쪽 궤도로 들어오고 있습니다.",
	StateStable:      "일정한 리듬으로 같은 궤도를 돌고 있습니다.",
	StateDrifting:    "교류가 줄며 바깥으로 밀려나고 있습니다.",
	StateDormant:     "오래 교류가 없어 Event Horizon 밖에 머무는 관계입니다.",
}

// ReadGrammar는 관계의 현재 상태를 읽는다.
//
// 교류 기록이 아직 없는 사람은 휴면으로 몰지 않고 안정 궤도에서 시작한다.
// 고정된(anchored) 관계는 침묵만으로는 밀려나지 않지만, momentum이 실제로
// 음수라면 멀어지는 흐름은 그대로 드러낸다.
func ReadGrammar(momentum float64, lastInteractionAt *time.Time, anchored bool, now time.Time) RelationState {
	silent := -1
	if lastInteractionAt != nil && !anchored {
		days := int(now.Sub(*lastInteractionAt).Hours() / 24)
		if days < 0 {
			days = 0
		}
		silent = days
	}
	switch {
	case silent >= DormantDays:
		return StateDormant
	case momentum > MomentumBand:
		return StateApproaching
	case momentum < -MomentumBand || silent >= DriftDays:
		return StateDrifting
	default:
		return StateStable
	}
}

// DaysSince는 마지막 교류로부터 흐른 일수를 돌려준다. 기록이 없으면 -1.
func DaysSince(at *time.Time, now time.Time) int {
	if at == nil {
		return -1
	}
	days := int(now.Sub(*at).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}
