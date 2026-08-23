import { describe, expect, it } from "vitest";
import {
  countByState,
  filterByState,
  isPeopleSort,
  sortPeople,
} from "./peopleView";
import type { Person } from "./types";

const NOW = Date.parse("2026-08-23T00:00:00Z");
const daysAgo = (days: number) =>
  new Date(NOW - days * 86_400_000).toISOString();

function person(
  name: string,
  extra: Partial<Person> = {},
): Person {
  return {
    id: name,
    display_name: name,
    company: "",
    role_title: "",
    avatar_url: "",
    email: "",
    phone: "",
    note: "",
    importance: 0.5,
    closeness: 0.5,
    momentum: 0,
    stable_x: 1,
    stable_y: 0,
    categories: [],
    relationship_label: "",
    created_at: daysAgo(900),
    ...extra,
  };
}

describe("countByState", () => {
  it("counts each state and omits the ones nobody is in", () => {
    const counts = countByState(
      [
        person("a", { momentum: 0.5, last_interaction_at: daysAgo(2) }),
        person("b", { momentum: 0, last_interaction_at: daysAgo(5) }),
        person("c", { momentum: 0, last_interaction_at: daysAgo(400) }),
      ],
      NOW,
    );
    expect(counts.approaching).toBe(1);
    expect(counts.stable).toBe(1);
    expect(counts.dormant).toBe(1);
    expect(counts.drifting).toBeUndefined();
  });
});

describe("filterByState", () => {
  const people = [
    person("가까움", { momentum: 0.5, last_interaction_at: daysAgo(2) }),
    person("멀어짐", { momentum: -0.5, last_interaction_at: daysAgo(20) }),
    person("고정", {
      momentum: 0,
      last_interaction_at: daysAgo(400),
      anchored: true,
    }),
  ];

  it("returns everyone when no state is chosen", () => {
    expect(filterByState(people, "", NOW)).toHaveLength(3);
  });

  it("keeps only the chosen state", () => {
    expect(
      filterByState(people, "drifting", NOW).map((p) => p.display_name),
    ).toEqual(["멀어짐"]);
  });

  it("respects anchoring, like the canvas does", () => {
    // 고정된 관계는 400일 침묵해도 다크 오빗이 아니다.
    expect(filterByState(people, "dormant", NOW)).toHaveLength(0);
    expect(
      filterByState(people, "stable", NOW).map((p) => p.display_name),
    ).toEqual(["고정"]);
  });
});

describe("sortPeople", () => {
  const oldest = person("오래", { last_interaction_at: daysAgo(300) });
  const recent = person("최근", { last_interaction_at: daysAgo(1) });
  const middle = person("중간", { last_interaction_at: daysAgo(30) });
  const never = person("신규", { last_interaction_at: undefined });
  const people = [middle, never, recent, oldest];

  it("puts the longest silence first when asked for oldest", () => {
    expect(sortPeople(people, "quiet", NOW).map((p) => p.display_name)).toEqual(
      ["오래", "중간", "최근", "신규"],
    );
  });

  it("reverses for most recent", () => {
    expect(sortPeople(people, "recent", NOW).map((p) => p.display_name)).toEqual(
      ["최근", "중간", "오래", "신규"],
    );
  });

  it("keeps people with no interactions last either way", () => {
    for (const sort of ["quiet", "recent"] as const) {
      const last = sortPeople(people, sort, NOW).at(-1)!;
      expect(last.display_name).toBe("신규");
    }
  });

  it("sorts by importance then name, and never mutates the input", () => {
    const list = [
      person("나중", { importance: 0.5 }),
      person("가장", { importance: 0.9 }),
      person("같음", { importance: 0.5 }),
    ];
    const snapshot = list.map((p) => p.display_name);
    expect(sortPeople(list, "importance", NOW).map((p) => p.display_name)).toEqual(
      ["가장", "같음", "나중"],
    );
    expect(list.map((p) => p.display_name)).toEqual(snapshot);
  });

  it("sorts Korean names in collation order", () => {
    const list = [person("한지민"), person("김도현"), person("박준호")];
    expect(sortPeople(list, "name", NOW).map((p) => p.display_name)).toEqual([
      "김도현",
      "박준호",
      "한지민",
    ]);
  });
});

describe("isPeopleSort", () => {
  it("accepts known sorts and rejects anything else", () => {
    expect(isPeopleSort("quiet")).toBe(true);
    expect(isPeopleSort("nonsense")).toBe(false);
    expect(isPeopleSort(null)).toBe(false);
  });
});
