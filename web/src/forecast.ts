import { DORMANT_DAYS, readGrammar, type Grammar } from "./orbitGrammar";
import type { OrbitNode } from "./types";

/**
 * Orbit Forecast — 지금의 리듬이 이어진다면 이 관계가 어디에 있을지.
 *
 * 예언이 아니라 외삽입니다. 새 교류가 없다는 가정 아래 침묵이 어떻게 쌓이는지를
 * 계산해 같은 궤도 문법으로 다시 읽습니다. 그래서 예측 결과도 점수가 아니라
 * 상태(다가오는 중·안정 궤도·멀어지는 중·다크 오빗)로 나옵니다.
 */

/** 기본 예측 지평. 한 달·두 달·세 달 뒤를 봅니다. */
export const HORIZONS = [30, 60, 90];

/** momentum이 절반으로 식는 데 걸리는 기간. 교류가 없으면 흐름은 0으로 수렴합니다. */
export const MOMENTUM_HALF_LIFE = 60;

export interface Forecast {
  horizonDays: number;
  /** 그 시점에 예상되는 마지막 교류 이후 일수. */
  quietDays: number;
  grammar: Grammar;
}

/**
 * 지평까지 쌓일 침묵을 추정합니다.
 *
 * - 다가오는 중: 교류가 늘고 있으니 침묵이 쌓이지 않고 오히려 짧아집니다.
 * - 안정 궤도: 지금의 리듬이 유지되므로 침묵은 제자리입니다.
 * - 멀어지는 중·다크 오빗: 새 교류를 가정하지 않으므로 시간만큼 그대로 쌓입니다.
 */
export function projectQuietDays(
  current: number,
  state: Grammar["state"],
  horizonDays: number,
) {
  if (state === "approaching")
    return Math.max(0, current - horizonDays * 0.5);
  if (state === "stable") return current;
  return current + horizonDays;
}

/** 지평 하나에 대한 예측. 교류 기록이 없으면 판단하지 않습니다. */
export function forecastAt(
  node: Pick<OrbitNode, "momentum" | "last_interaction_at" | "anchored">,
  horizonDays: number,
  now = Date.now(),
): Forecast | null {
  const today = readGrammar(node, now);
  if (today.dormantDays === null) return null;
  const quietDays = projectQuietDays(
    today.dormantDays,
    today.state,
    horizonDays,
  );
  // 교류가 없으면 momentum도 식습니다. 지금의 기울기가 영원히 가지는 않습니다.
  const momentum =
    node.momentum * Math.pow(0.5, horizonDays / MOMENTUM_HALF_LIFE);
  const future = now + horizonDays * 86_400_000;
  const grammar = readGrammar(
    {
      momentum,
      anchored: node.anchored,
      last_interaction_at: new Date(
        future - quietDays * 86_400_000,
      ).toISOString(),
    },
    future,
  );
  return { horizonDays, quietDays, grammar };
}

/** 여러 지평을 한 번에. */
export function forecastSeries(
  node: Pick<OrbitNode, "momentum" | "last_interaction_at" | "anchored">,
  horizons = HORIZONS,
  now = Date.now(),
): Forecast[] {
  return horizons
    .map((horizon) => forecastAt(node, horizon, now))
    .filter((entry): entry is Forecast => entry !== null);
}

/**
 * 이대로 두면 며칠 뒤 Event Horizon을 넘는지.
 * 이미 넘었거나, 넘지 않을 관계면 null.
 */
export function daysUntilDarkOrbit(
  node: Pick<OrbitNode, "momentum" | "last_interaction_at" | "anchored">,
  now = Date.now(),
): number | null {
  const today = readGrammar(node, now);
  if (today.anchored || today.state === "dormant") return null;
  if (today.dormantDays === null) return null;
  if (today.state === "approaching" || today.state === "stable") return null;
  const remaining = DORMANT_DAYS - today.dormantDays;
  return remaining > 0 ? Math.ceil(remaining) : null;
}

/**
 * 예측이 지금보다 나빠지는 관계인지.
 *
 * "다가오는 중 → 안정 궤도"는 악화로 세지 않습니다. 뜨겁던 관계가 일정한
 * 리듬으로 자리잡는 것이니, 그걸 경고로 띄우면 가장 건강한 관계가 가장
 * 시끄러워집니다. 바깥으로 향할 때만 경고입니다.
 */
export function isDegrading(
  node: Pick<OrbitNode, "momentum" | "last_interaction_at" | "anchored">,
  horizonDays = 90,
  now = Date.now(),
) {
  const rank = { approaching: 0, stable: 1, drifting: 2, dormant: 3 };
  const today = readGrammar(node, now);
  const later = forecastAt(node, horizonDays, now);
  if (!later) return false;
  const outward =
    later.grammar.state === "drifting" || later.grammar.state === "dormant";
  return outward && rank[later.grammar.state] > rank[today.state];
}
