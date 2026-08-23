import { describe, expect, it } from "vitest";
import { DIRECT_COST, findApproach, linkCost } from "./gravityAssist";
import type { OrbitLink, OrbitNode } from "./types";

const NOW = Date.parse("2026-08-23T00:00:00Z");
const daysAgo = (days: number) =>
  new Date(NOW - days * 86_400_000).toISOString();

function person(
  id: string,
  momentum: number,
  days: number,
  extra: Partial<OrbitNode> = {},
): OrbitNode {
  return {
    id,
    name: id,
    avatar_url: "",
    importance: 0.5,
    closeness: 0.5,
    momentum,
    x: 1,
    y: 0,
    categories: [],
    label: "",
    last_interaction_at: daysAgo(days),
    ...extra,
  };
}

describe("linkCost", () => {
  it("makes strong ties cheaper to travel than weak ones", () => {
    expect(linkCost(1)).toBeLessThan(linkCost(0.2));
  });

  it("clamps nonsense strengths instead of producing NaN", () => {
    expect(linkCost(Number.NaN)).toBe(linkCost(0));
    expect(linkCost(9)).toBe(linkCost(1));
    expect(linkCost(-3)).toBe(linkCost(0));
  });
});

describe("findApproach", () => {
  it("says to just reach out when the person is already close", () => {
    const nodes = [person("active", 0.5, 2)];
    const route = findApproach("active", nodes, [], NOW)!;
    expect(route.direct).toBe(true);
    expect(route.steps).toHaveLength(1);
    expect(route.cost).toBe(DIRECT_COST.approaching);
  });

  it("routes through an active relationship to reach a dormant one", () => {
    const nodes = [
      person("bridge", 0.5, 2), // 다가오는 중
      person("lost", 0, 400), // 다크 오빗
    ];
    const links: OrbitLink[] = [
      { a: "bridge", b: "lost", kind: "colleague", strength: 0.9 },
    ];
    const route = findApproach("lost", nodes, links, NOW)!;
    expect(route.direct).toBe(false);
    expect(route.steps.map((s) => s.person_id)).toEqual(["bridge", "lost"]);
    expect(route.cost).toBeLessThan(DIRECT_COST.dormant);
  });

  it("keeps the direct route when the detour costs more", () => {
    const nodes = [
      person("faraway", -0.9, 120), // 멀어지는 중
      person("target", 0, 10), // 안정 궤도
    ];
    const links: OrbitLink[] = [
      { a: "faraway", b: "target", kind: "knows", strength: 0.1 },
    ];
    const route = findApproach("target", nodes, links, NOW)!;
    expect(route.direct).toBe(true);
  });

  it("prefers the stronger of two possible bridges", () => {
    const nodes = [
      person("weakBridge", 0.5, 2),
      person("strongBridge", 0.5, 2),
      person("lost", 0, 400),
    ];
    const links: OrbitLink[] = [
      { a: "weakBridge", b: "lost", kind: "knows", strength: 0.1 },
      { a: "lost", b: "strongBridge", kind: "family", strength: 1 },
    ];
    const route = findApproach("lost", nodes, links, NOW)!;
    expect(route.steps[0].person_id).toBe("strongBridge");
  });

  it("can chain two hops when every direct route is expensive", () => {
    const nodes = [
      person("near", 0.5, 2),
      person("middle", 0, 400),
      person("lost", 0, 400),
    ];
    const links: OrbitLink[] = [
      { a: "near", b: "middle", kind: "colleague", strength: 1 },
      { a: "middle", b: "lost", kind: "colleague", strength: 1 },
    ];
    const route = findApproach("lost", nodes, links, NOW)!;
    expect(route.steps.map((s) => s.person_id)).toEqual([
      "near",
      "middle",
      "lost",
    ]);
    expect(route.cost).toBeLessThan(DIRECT_COST.dormant);
  });

  it("ignores links pointing at people outside this orbit", () => {
    const nodes = [person("lost", 0, 400)];
    const links: OrbitLink[] = [
      { a: "lost", b: "ghost", kind: "knows", strength: 1 },
    ];
    const route = findApproach("lost", nodes, links, NOW)!;
    expect(route.direct).toBe(true);
  });

  it("returns null for someone who is not in the orbit", () => {
    expect(findApproach("nobody", [person("a", 0, 1)], [], NOW)).toBeNull();
  });

  it("respects anchoring when pricing the direct route", () => {
    const silent = person("held", 0, 400, { anchored: true });
    const route = findApproach("held", [silent], [], NOW)!;
    // 고정된 관계는 침묵해도 다크 오빗이 아니므로 직접 연락이 비싸지 않습니다.
    expect(route.cost).toBe(DIRECT_COST.stable);
  });
});
