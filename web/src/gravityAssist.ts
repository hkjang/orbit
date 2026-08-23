import { readGrammar, type RelationState } from "./orbitGrammar";
import type { OrbitLink, OrbitNode } from "./types";

/**
 * Gravity Assist — 탐사선이 행성의 중력을 빌려 더 멀리 가듯,
 * 소원해진 사람에게 곧장 연락하는 대신 지금 활발한 관계를 거쳐 닿는 길을 찾습니다.
 *
 * 최단 경로가 아니라 최소 비용 경로입니다. 내 우주의 모든 사람은 나와 직접
 * 이어져 있으므로 경로 길이로는 아무것도 구분되지 않습니다. 대신 "지금 얼마나
 * 말 걸기 쉬운가"를 간선 비용으로 두고 다익스트라를 돌립니다.
 */

/** 나 → 그 사람에게 직접 연락하는 비용. 관계 상태가 그대로 저항이 됩니다. */
export const DIRECT_COST: Record<RelationState, number> = {
  approaching: 1,
  stable: 1.6,
  drifting: 3.4,
  dormant: 6.5,
};

/** 사람 → 사람으로 건너가는 비용. 두 사람이 끈끈할수록 부탁하기 쉽습니다. */
export function linkCost(strength: number) {
  const bounded = Math.max(0, Math.min(1, Number.isFinite(strength) ? strength : 0));
  return 1.2 + (1 - bounded) * 2.2;
}

export interface AssistStep {
  person_id: string;
  name: string;
  /** 이 사람에게 어떻게 닿는지. 첫 걸음은 언제나 나로부터입니다. */
  from: "me" | "person";
  cost: number;
}

export interface AssistRoute {
  steps: AssistStep[];
  cost: number;
  /** 직접 연락이 이미 가장 값싼 길이면 true. 굳이 우회할 이유가 없습니다. */
  direct: boolean;
}

/**
 * 대상에게 닿는 가장 값싼 길을 찾습니다.
 * 대상이 없거나 도달할 수 없으면 null.
 */
export function findApproach(
  targetId: string,
  nodes: OrbitNode[],
  links: OrbitLink[],
  now = Date.now(),
): AssistRoute | null {
  const byId = new Map(nodes.map((node) => [node.id, node]));
  if (!byId.has(targetId)) return null;

  const neighbors = new Map<string, { to: string; cost: number }[]>();
  for (const link of links) {
    if (!byId.has(link.a) || !byId.has(link.b)) continue;
    const cost = linkCost(link.strength);
    for (const [from, to] of [
      [link.a, link.b],
      [link.b, link.a],
    ]) {
      const list = neighbors.get(from) ?? [];
      list.push({ to, cost });
      neighbors.set(from, list);
    }
  }

  const distance = new Map<string, number>();
  const previous = new Map<string, string | null>();
  for (const node of nodes) {
    const state = readGrammar(node, now).state;
    distance.set(node.id, DIRECT_COST[state]);
    previous.set(node.id, null);
  }

  // 노드 수가 많아야 수백이라 단순 선형 추출로 충분합니다.
  const settled = new Set<string>();
  for (;;) {
    let current: string | undefined;
    let best = Infinity;
    for (const [nodeId, cost] of distance)
      if (!settled.has(nodeId) && cost < best) {
        best = cost;
        current = nodeId;
      }
    if (current === undefined) break;
    settled.add(current);
    if (current === targetId) break;
    for (const edge of neighbors.get(current) ?? []) {
      if (settled.has(edge.to)) continue;
      const relaxed = best + edge.cost;
      if (relaxed < (distance.get(edge.to) ?? Infinity)) {
        distance.set(edge.to, relaxed);
        previous.set(edge.to, current);
      }
    }
  }

  const total = distance.get(targetId);
  if (total === undefined || !Number.isFinite(total)) return null;

  const chain: string[] = [];
  for (let at: string | null | undefined = targetId; at; at = previous.get(at))
    chain.unshift(at);

  let running = 0;
  const steps = chain.map((personId, index): AssistStep => {
    const node = byId.get(personId)!;
    const cost =
      index === 0
        ? DIRECT_COST[readGrammar(node, now).state]
        : (distance.get(personId) ?? 0) - running;
    running = distance.get(personId) ?? running;
    return {
      person_id: personId,
      name: node.name,
      from: index === 0 ? "me" : "person",
      cost,
    };
  });
  return { steps, cost: total, direct: steps.length === 1 };
}
