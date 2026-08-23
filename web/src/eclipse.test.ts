import { describe, expect, it } from "vitest";
import {
  connectedComponents,
  describeEclipse,
  findEclipses,
  josaWaGwa,
  SPREAD_DAYS,
} from "./eclipse";
import type { OrbitLink, OrbitNode } from "./types";

const NOW = Date.parse("2026-08-23T00:00:00Z");
const daysAgo = (days: number) =>
  new Date(NOW - days * 86_400_000).toISOString();

function person(id: string, quietDays: number | null): OrbitNode {
  return {
    id,
    name: id,
    avatar_url: "",
    importance: 0.5,
    closeness: 0.5,
    momentum: 0,
    x: 1,
    y: 0,
    categories: [],
    label: "",
    last_interaction_at:
      quietDays === null ? undefined : daysAgo(quietDays),
  };
}

const chain = (...ids: string[]): OrbitLink[] =>
  ids.slice(1).map((id, index) => ({
    a: ids[index],
    b: id,
    kind: "colleague",
    strength: 0.8,
  }));

describe("connectedComponents", () => {
  it("groups linked people and leaves unlinked ones out", () => {
    const nodes = [person("a", 1), person("b", 1), person("solo", 1)];
    const components = connectedComponents(nodes, chain("a", "b"));
    expect(components).toHaveLength(1);
    expect(components[0].map((n) => n.id).sort()).toEqual(["a", "b"]);
  });

  it("separates two disconnected clusters", () => {
    const nodes = ["a", "b", "c", "d"].map((id) => person(id, 1));
    const components = connectedComponents(nodes, [
      ...chain("a", "b"),
      ...chain("c", "d"),
    ]);
    expect(components).toHaveLength(2);
  });

  it("ignores links to people outside this orbit", () => {
    const nodes = [person("a", 1)];
    const components = connectedComponents(nodes, [
      { a: "a", b: "ghost", kind: "knows", strength: 1 },
    ]);
    expect(components).toHaveLength(0);
  });
});

describe("findEclipses", () => {
  it("finds a group that went quiet around the same time", () => {
    const nodes = [person("a", 180), person("b", 195), person("c", 172)];
    const found = findEclipses(nodes, chain("a", "b", "c"), NOW);
    expect(found).toHaveLength(1);
    expect(found[0].faded).toHaveLength(3);
    expect(found[0].quietDays).toBe(180);
    expect(found[0].spreadDays).toBe(23);
  });

  it("ignores a pair — two people are not yet a group", () => {
    const nodes = [person("a", 200), person("b", 200)];
    expect(findEclipses(nodes, chain("a", "b"), NOW)).toHaveLength(0);
  });

  it("ignores a group whose silences are spread far apart", () => {
    const nodes = [
      person("a", 100),
      person("b", 100 + SPREAD_DAYS + 30),
      person("c", 120),
    ];
    expect(findEclipses(nodes, chain("a", "b", "c"), NOW)).toHaveLength(0);
  });

  it("ignores a group that is still active", () => {
    const nodes = [person("a", 3), person("b", 5), person("c", 200)];
    expect(findEclipses(nodes, chain("a", "b", "c"), NOW)).toHaveLength(0);
  });

  it("still reports when one member stays in touch", () => {
    const nodes = [person("a", 200), person("b", 210), person("live", 2)];
    const found = findEclipses(nodes, chain("a", "b", "live"), NOW);
    expect(found).toHaveLength(1);
    expect(found[0].faded.map((m) => m.id)).toEqual(["a", "b"]);
    expect(found[0].members).toHaveLength(3);
  });

  it("treats a person with no interactions as unknown, not faded", () => {
    const nodes = [person("a", 200), person("b", 210), person("new", null)];
    const found = findEclipses(nodes, chain("a", "b", "new"), NOW);
    // 교류 기록이 없는 사람은 조용해진 쪽으로 세지 않습니다. 알 수 없을 뿐입니다.
    expect(found[0].faded.map((m) => m.id)).toEqual(["a", "b"]);
    expect(found[0].members.map((m) => m.id)).toContain("new");
  });

  it("needs enough of the group to have gone quiet", () => {
    const nodes = [
      person("a", 200),
      person("b", 3),
      person("c", 5),
      person("d", 4),
    ];
    // 넷 중 하나만 조용해진 것은 그룹의 사건이 아닙니다.
    expect(findEclipses(nodes, chain("a", "b", "c", "d"), NOW)).toHaveLength(0);
  });

  it("ranks the larger, older eclipse first", () => {
    const nodes = [
      person("a1", 300),
      person("a2", 310),
      person("a3", 305),
      person("a4", 300),
      person("b1", 100),
      person("b2", 110),
      person("b3", 105),
    ];
    const found = findEclipses(
      nodes,
      [...chain("a1", "a2", "a3", "a4"), ...chain("b1", "b2", "b3")],
      NOW,
    );
    expect(found).toHaveLength(2);
    expect(found[0].faded).toHaveLength(4);
  });
});

describe("describeEclipse", () => {
  it("names the group and points at whoever is still reachable", () => {
    const nodes = [person("혜진", 200), person("동우", 205)];
    const bridge = { ...person("수민", 2), momentum: 0.6 };
    const all = [...nodes, bridge];
    const group = findEclipses(all, chain("혜진", "동우", "수민"), NOW)[0];
    const copy = describeEclipse(group, all);
    expect(copy.headline).toContain("혜진");
    expect(copy.headline).toContain("개월");
    expect(copy.hint).toContain("수민");
  });

  it("falls back to a neutral hint when nobody is still close", () => {
    const nodes = [person("a", 200), person("b", 205), person("c", 210)];
    const group = findEclipses(nodes, chain("a", "b", "c"), NOW)[0];
    expect(describeEclipse(group, nodes).hint).toContain("계기");
  });
});

describe("josaWaGwa", () => {
  it("picks the particle from the final consonant", () => {
    expect(josaWaGwa("장세라")).toBe("와");
    expect(josaWaGwa("김도현")).toBe("과");
    expect(josaWaGwa("외 2명")).toBe("과");
  });

  it("falls back for names that are not Hangul", () => {
    expect(josaWaGwa("Alex")).toBe("과");
    expect(josaWaGwa("")).toBe("과");
  });

  it("is applied to the assembled subject, not just the first name", () => {
    const nodes = [person("장세라", 200), person("한결", 205), person("민아", 210)];
    const group = findEclipses(nodes, chain("장세라", "한결", "민아"), NOW)[0];
    expect(describeEclipse(group, nodes).headline).toContain("민아와의");
  });
});
