import { daysSince, DRIFT_DAYS, readGrammar } from "./orbitGrammar";
import type { OrbitLink, OrbitNode } from "./types";

/**
 * Eclipse — 서로 이어진 사람들이 같은 시기에 함께 조용해진 현상.
 *
 * 한 사람이 멀어지는 것은 개인적인 일이지만, 서로 아는 사람들이 비슷한 시점에
 * 동시에 멀어졌다면 대개 공통 원인이 있습니다(프로젝트 종료, 부서 이동, 이사).
 * 그 사실을 사람별 경고가 아니라 그룹 단위 사건으로 보여줍니다.
 */

/** 사건으로 볼 최소 인원. 두 사람이 뜸해진 것은 아직 그룹의 일이 아닙니다. */
export const MIN_GROUP = 3;
/** 이 비율 이상이 조용해져야 그룹 전체의 일로 봅니다. */
export const FADED_RATIO = 2 / 3;
/** 마지막 교류 시점이 이 기간 안에 몰려 있어야 "함께" 멀어진 것입니다. */
export const SPREAD_DAYS = 60;

export interface EclipseMember {
  id: string;
  name: string;
  quietDays: number | null;
}

export interface EclipseGroup {
  members: EclipseMember[];
  faded: EclipseMember[];
  /** 조용해진 사람들의 마지막 교류가 얼마나 지났는지(중앙값). */
  quietDays: number;
  /** 그 시점들이 얼마나 몰려 있는지. 작을수록 동시에 멀어진 것입니다. */
  spreadDays: number;
}

/** person_links를 무방향 그래프로 보고 연결 성분을 나눕니다. */
export function connectedComponents(
  nodes: OrbitNode[],
  links: OrbitLink[],
): OrbitNode[][] {
  const byId = new Map(nodes.map((node) => [node.id, node]));
  const neighbors = new Map<string, string[]>();
  for (const link of links) {
    if (!byId.has(link.a) || !byId.has(link.b)) continue;
    neighbors.set(link.a, [...(neighbors.get(link.a) ?? []), link.b]);
    neighbors.set(link.b, [...(neighbors.get(link.b) ?? []), link.a]);
  }
  const seen = new Set<string>();
  const components: OrbitNode[][] = [];
  for (const node of nodes) {
    if (seen.has(node.id) || !neighbors.has(node.id)) continue;
    const group: OrbitNode[] = [];
    const queue = [node.id];
    seen.add(node.id);
    while (queue.length) {
      const current = queue.shift()!;
      const member = byId.get(current);
      if (member) group.push(member);
      for (const next of neighbors.get(current) ?? [])
        if (!seen.has(next)) {
          seen.add(next);
          queue.push(next);
        }
    }
    components.push(group);
  }
  return components;
}

function median(values: number[]) {
  const sorted = [...values].sort((a, b) => a - b);
  const middle = Math.floor(sorted.length / 2);
  return sorted.length % 2
    ? sorted[middle]
    : (sorted[middle - 1] + sorted[middle]) / 2;
}

/**
 * 함께 조용해진 그룹을 찾습니다.
 * 조용함의 기준은 momentum이 아니라 마지막 교류로부터 흐른 시간입니다 —
 * "같은 시기에 멈췄다"를 말하려면 시점이 근거여야 합니다.
 */
export function findEclipses(
  nodes: OrbitNode[],
  links: OrbitLink[],
  now = Date.now(),
): EclipseGroup[] {
  const groups: EclipseGroup[] = [];
  for (const component of connectedComponents(nodes, links)) {
    if (component.length < MIN_GROUP) continue;
    const members: EclipseMember[] = component.map((node) => ({
      id: node.id,
      name: node.name,
      quietDays: daysSince(node.last_interaction_at, now),
    }));
    const faded = members.filter(
      (member) => member.quietDays !== null && member.quietDays >= DRIFT_DAYS,
    );
    if (faded.length / members.length < FADED_RATIO) continue;
    const quiet = faded.map((member) => member.quietDays!);
    const spreadDays = Math.max(...quiet) - Math.min(...quiet);
    if (spreadDays > SPREAD_DAYS) continue;
    groups.push({ members, faded, quietDays: median(quiet), spreadDays });
  }
  // 더 오래, 더 많이 조용해진 그룹을 먼저 보여줍니다.
  return groups.sort(
    (a, b) => b.faded.length - a.faded.length || b.quietDays - a.quietDays,
  );
}

/**
 * 앞말의 받침에 따라 "와/과"를 고릅니다.
 * 한글 음절의 종성 유무로 판정하고, 한글이 아니면 "과"로 둡니다.
 */
export function josaWaGwa(text: string) {
  const last = text.trim().slice(-1);
  const code = last.charCodeAt(0) - 0xac00;
  if (code < 0 || code > 11171) return "과";
  return code % 28 === 0 ? "와" : "과";
}

/** 그룹을 한 문장으로 설명합니다. 숫자를 나열하지 않고 사건으로 말합니다. */
export function describeEclipse(group: EclipseGroup, nodes: OrbitNode[]) {
  const months = Math.max(1, Math.round(group.quietDays / 30));
  const stillClose = group.members.filter((member) => {
    const node = nodes.find((n) => n.id === member.id);
    return node && readGrammar(node).state === "approaching";
  });
  const names = group.faded
    .slice(0, 3)
    .map((member) => member.name)
    .join(", ");
  const rest = group.faded.length - Math.min(3, group.faded.length);
  const subject = `${names}${rest > 0 ? ` 외 ${rest}명` : ""}`;
  return {
    headline: `${subject}${josaWaGwa(subject)}의 교류가 ${months}개월쯤 전부터 함께 멈췄습니다.`,
    hint: stillClose.length
      ? `${stillClose[0].name}님과는 아직 활발합니다. 이 분을 통해 소식을 들을 수 있습니다.`
      : "같은 시기에 함께 멀어진 걸 보면 공통된 계기가 있었을 수 있습니다.",
  };
}
