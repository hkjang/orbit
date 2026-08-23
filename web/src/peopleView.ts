import { daysSince, readGrammar, type RelationState } from "./orbitGrammar";
import type { Person } from "./types";

/**
 * 관계 목록을 상태로 거르고 순서를 바꾸는 규칙.
 *
 * 목록은 지금까지 중요도 한 축으로만 정렬됐다. 그러나 실제로 자주 묻게 되는
 * 질문은 "누가 멀어지고 있지", "누구를 제일 오래 못 봤지"에 가깝다.
 * 캔버스가 이미 답하는 질문을 목록도 답할 수 있게 한다.
 */
export type PeopleSort = "importance" | "quiet" | "recent" | "name";

export const SORTS: { value: PeopleSort; label: string }[] = [
  { value: "importance", label: "중요도순" },
  { value: "quiet", label: "오래된 순" },
  { value: "recent", label: "최근 교류순" },
  { value: "name", label: "이름순" },
];

export function isPeopleSort(value: string | null): value is PeopleSort {
  return SORTS.some((sort) => sort.value === value);
}

/** 상태별 인원 수. 0인 상태는 칩으로 띄우지 않는다. */
export function countByState(people: Person[], now = Date.now()) {
  const counts = {} as Record<RelationState, number>;
  for (const person of people) {
    const state = readGrammar(person, now).state;
    counts[state] = (counts[state] ?? 0) + 1;
  }
  return counts;
}

export function filterByState(
  people: Person[],
  state: RelationState | "",
  now = Date.now(),
) {
  if (!state) return people;
  return people.filter((person) => readGrammar(person, now).state === state);
}

/**
 * 정렬. 교류 기록이 없는 사람은 "가장 오래됐다"고도 "가장 최근"이라고도
 * 말할 수 없으므로, 어느 방향으로 정렬하든 뒤로 보낸다.
 */
export function sortPeople(
  people: Person[],
  sort: PeopleSort,
  now = Date.now(),
) {
  const byName = (a: Person, b: Person) =>
    a.display_name.localeCompare(b.display_name, "ko");
  const quiet = (person: Person) => daysSince(person.last_interaction_at, now);
  return [...people].sort((a, b) => {
    switch (sort) {
      case "name":
        return byName(a, b);
      case "quiet":
      case "recent": {
        const da = quiet(a);
        const db = quiet(b);
        if (da === null && db === null) return byName(a, b);
        if (da === null) return 1;
        if (db === null) return -1;
        if (da === db) return byName(a, b);
        return sort === "quiet" ? db - da : da - db;
      }
      default:
        return b.importance - a.importance || byName(a, b);
    }
  });
}
