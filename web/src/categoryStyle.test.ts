import { describe, expect, it } from "vitest";
import {
  assignCategoryStyles,
  CATEGORY_COLORS,
  DISTINCT_STYLES,
  NO_CATEGORY,
  stateTones,
  styleFor,
} from "./categoryStyle";

const SAMPLE = [
  "AI 추진단",
  "가족",
  "친구",
  "알파 프로젝트",
  "멘토",
  "대학 동기",
  "전 직장",
  "동아리",
];

const look = (s: { color: string; double: boolean }) =>
  `${s.color}/${s.double}`;

describe("assignCategoryStyles", () => {
  it("gives every category its own look", () => {
    const styles = assignCategoryStyles(SAMPLE);
    expect(new Set([...styles.values()].map(look)).size).toBe(SAMPLE.length);
  });

  it("keeps all 16 combinations distinct before repeating", () => {
    const many = Array.from({ length: DISTINCT_STYLES }, (_, i) => `그룹 ${i}`);
    const styles = assignCategoryStyles(many);
    expect(new Set([...styles.values()].map(look)).size).toBe(DISTINCT_STYLES);
  });

  it("uses both ring weights once colours run out", () => {
    const many = Array.from({ length: DISTINCT_STYLES }, (_, i) => `그룹 ${i}`);
    const weights = new Set(
      [...assignCategoryStyles(many).values()].map((s) => s.double),
    );
    expect(weights.size).toBe(2);
  });

  it("is stable for the same set, whatever order it arrives in", () => {
    const forward = assignCategoryStyles(SAMPLE);
    const backward = assignCategoryStyles([...SAMPLE].reverse());
    for (const name of SAMPLE) {
      expect(backward.get(name)).toEqual(forward.get(name));
    }
  });

  it("ignores duplicates", () => {
    expect(assignCategoryStyles(["가족", "가족", "친구"]).size).toBe(2);
  });

  it("never reuses a relationship state colour", () => {
    // 본체는 상태, 테두리는 소속. 같은 색을 쓰면 무엇을 말하는 색인지 알 수 없다.
    const tones = stateTones();
    for (const color of CATEGORY_COLORS) {
      expect(tones).not.toContain(color.toLowerCase());
    }
  });

  it("has no duplicate colours in the palette", () => {
    expect(new Set(CATEGORY_COLORS).size).toBe(CATEGORY_COLORS.length);
  });
});

describe("styleFor", () => {
  const styles = assignCategoryStyles(SAMPLE);

  it("looks up a known category", () => {
    expect(styleFor(styles, "가족")).toEqual(styles.get("가족"));
  });

  it("recedes for someone with no category", () => {
    // 소속 없음은 정보의 부재다. 자기 색을 갖게 두면 없는 소속이 있는 것처럼 보인다.
    expect(styleFor(styles, undefined)).toEqual(NO_CATEGORY);
    expect(styleFor(styles, "")).toEqual(NO_CATEGORY);
  });

  it("recedes for a category the assignment has not seen", () => {
    expect(styleFor(styles, "아직 모르는 소속")).toEqual(NO_CATEGORY);
  });

  it("never gives the neutral colour to a real category", () => {
    for (const style of styles.values()) {
      expect(style.color).not.toBe(NO_CATEGORY.color);
    }
  });
});
