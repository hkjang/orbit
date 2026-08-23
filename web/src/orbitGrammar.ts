import type { OrbitNode } from "./types";

/**
 * Orbit 시각 문법(Visual Grammar).
 *
 * 크기 = 중요도 · 거리 = 친밀도 · 상태 = 최근 움직임.
 * 사람을 점수로 환산하지 않고, 관계가 지금 어느 방향으로 움직이는지만 말합니다.
 * 캔버스·목록·상세가 모두 이 모듈 하나를 공유합니다.
 */
export type RelationState =
  | "approaching"
  | "stable"
  | "drifting"
  | "dormant";

/** 마지막 교류로부터 이 일수를 넘기면 Event Horizon 바깥(Dark Orbit)으로 이동합니다. */
export const DORMANT_DAYS = 180;
/** 교류가 이만큼 끊기면 momentum과 무관하게 멀어지는 흐름으로 봅니다. */
export const DRIFT_DAYS = 90;
/** 이 폭 안의 momentum은 흔들림으로 보고 안정 궤도로 둡니다. */
export const MOMENTUM_BAND = 0.18;

export interface Grammar {
  state: RelationState;
  /** 고정된 관계인지. 고정되면 침묵만으로는 바깥으로 밀려나지 않습니다. */
  anchored: boolean;
  /** 마지막 교류 이후 경과 일수. 교류 기록이 아직 없으면 null. */
  dormantDays: number | null;
  label: string;
  hint: string;
  /** 상태 색(온도). 카테고리 색과 분리해 씁니다. */
  tone: string;
  /**
   * 중심을 향한 부호 있는 이동량(-1..1).
   * 양수면 다가오는 중, 음수면 멀어지는 중. Momentum Vector 길이로 씁니다.
   */
  vector: number;
}

export const STATE_ORDER: RelationState[] = [
  "approaching",
  "stable",
  "drifting",
  "dormant",
];

export const STATE_META: Record<
  RelationState,
  { label: string; hint: string; tone: string }
> = {
  approaching: {
    label: "다가오는 중",
    hint: "최근 교류가 늘며 안쪽 궤도로 들어오고 있습니다.",
    tone: "#7be2a9",
  },
  stable: {
    label: "안정 궤도",
    hint: "일정한 리듬으로 같은 궤도를 돌고 있습니다.",
    tone: "#a99bf8",
  },
  drifting: {
    label: "멀어지는 중",
    hint: "교류가 줄며 바깥으로 밀려나고 있습니다.",
    tone: "#f0bd69",
  },
  dormant: {
    label: "다크 오빗",
    hint: "오래 교류가 없어 Event Horizon 밖에 머무는 관계입니다.",
    tone: "#7c86a8",
  },
};

export function daysSince(
  iso: string | undefined,
  now = Date.now(),
): number | null {
  if (!iso) return null;
  const at = Date.parse(iso);
  if (Number.isNaN(at)) return null;
  return Math.max(0, Math.floor((now - at) / 86_400_000));
}

/**
 * 관계의 현재 상태를 읽습니다.
 * 교류 기록이 아직 없는 사람은 휴면으로 몰지 않고 안정 궤도에서 시작합니다.
 */
export function readGrammar(
  node: Pick<OrbitNode, "momentum" | "last_interaction_at" | "anchored">,
  now = Date.now(),
): Grammar {
  const dormantDays = daysSince(node.last_interaction_at, now);
  const momentum = Number.isFinite(node.momentum) ? node.momentum : 0;
  const anchored = node.anchored === true;
  // 고정된 관계는 침묵을 근거로 밀려나지 않습니다. 다만 실제 교류가 줄고 있다면
  // 그 사실까지 숨기지는 않으므로, momentum이 만든 흐름은 그대로 보여줍니다.
  const silent = anchored ? null : dormantDays;
  let state: RelationState;
  if (silent !== null && silent >= DORMANT_DAYS) state = "dormant";
  else if (momentum > MOMENTUM_BAND) state = "approaching";
  else if (
    momentum < -MOMENTUM_BAND ||
    (silent !== null && silent >= DRIFT_DAYS)
  )
    state = "drifting";
  else state = "stable";
  const meta = STATE_META[state];
  const vector =
    state === "dormant"
      ? -1
      : Math.max(-1, Math.min(1, momentum + (state === "drifting" ? -0.1 : 0)));
  return { state, dormantDays, anchored, vector, ...meta };
}

/**
 * 같은 별자리(카테고리)에 속한 사람들을 잇는 선을 고릅니다.
 * 모든 쌍을 잇지 않고 최소 신장 트리로 이어, 실제 별자리처럼 읽히게 합니다.
 */
export function constellationEdges<T extends { px: number; py: number }>(
  members: T[],
): [T, T][] {
  if (members.length < 2) return [];
  const linked = [members[0]];
  const rest = members.slice(1);
  const edges: [T, T][] = [];
  while (rest.length) {
    let best = { from: 0, to: 0, distance: Infinity };
    for (let i = 0; i < linked.length; i++)
      for (let j = 0; j < rest.length; j++) {
        const distance = Math.hypot(
          rest[j].px - linked[i].px,
          rest[j].py - linked[i].py,
        );
        if (distance < best.distance) best = { from: i, to: j, distance };
      }
    edges.push([linked[best.from], rest[best.to]]);
    linked.push(rest[best.to]);
    rest.splice(best.to, 1);
  }
  return edges;
}

/**
 * Memory Nebula 반경.
 * 기억이 쌓일수록 행성 주위에 성운이 넓어지되, 수집 경쟁이 되지 않도록
 * 제곱근으로 완만하게 자라고 상한에서 멈춥니다.
 */
export function nebulaRadius(memoryCount: number | undefined, radius: number) {
  const count = Math.max(0, Math.floor(memoryCount ?? 0));
  if (count === 0) return 0;
  return radius + 10 + Math.min(Math.sqrt(count), 4.5) * 9;
}
