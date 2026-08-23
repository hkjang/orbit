import { STATE_META, STATE_ORDER } from "./orbitGrammar";

/**
 * 카테고리(소속)를 행성 테두리로 표현하는 규칙.
 *
 * 두 가지를 지킨다.
 *
 * 1. 상태 색과 겹치지 않는다. 행성 본체는 관계 상태, 테두리는 카테고리를
 *    말한다. 두 축이 같은 색을 쓰면 무엇을 말하는 색인지 알 수 없다.
 * 2. 색 하나에만 기대지 않는다. 테두리 두께(홑/겹)를 두 번째 축으로 두어,
 *    색을 구별하기 어려운 사람도 서로 다른 소속임을 알 수 있게 한다.
 */

/** 상태 색(초록·보라·호박·슬레이트)과 충분히 떨어진 색만 고른다. */
export const CATEGORY_COLORS = [
  "#5fd0d8", // 청록
  "#78b7f1", // 하늘
  "#c98cf0", // 보랏빛 분홍
  "#d58fce", // 자홍
  "#f08fa8", // 장미
  "#f59276", // 산호
  "#c9dd5c", // 라임
  "#93cf6d", // 잎
];

/** 소속이 없는 사람의 테두리. 정보의 부재이므로 눈에 띄지 않게 물러난다. */
export const NO_CATEGORY: CategoryStyle = { color: "#5b6478", double: false };

export interface CategoryStyle {
  color: string;
  /** 참이면 테두리를 두 겹으로 그린다. 색 외의 두 번째 구분 축. */
  double: boolean;
}

/** 색 8가지 × 테두리 겹수 2가지 = 서로 구별되는 소속 16가지. */
export const DISTINCT_STYLES = CATEGORY_COLORS.length * 2;

/**
 * 소속 목록에 서로 다른 모양을 배정한다.
 *
 * 해시로 색을 고르면 소속이 몇 개든 상관없이 계산할 수 있지만, 여덟 가지 색에
 * 여덟 개 소속만 넣어도 흔히 겹친다. 실제로 무엇이 있는지 알고 배정하면
 * 16개까지는 충돌이 없다. 이름 순으로 배정해 같은 구성에서는 늘 같은 모양이
 * 나오게 한다.
 */
export function assignCategoryStyles(
  categories: Iterable<string>,
): Map<string, CategoryStyle> {
  const sorted = [...new Set(categories)].sort((a, b) =>
    a.localeCompare(b, "ko"),
  );
  const styles = new Map<string, CategoryStyle>();
  sorted.forEach((name, index) => {
    styles.set(name, {
      color: CATEGORY_COLORS[index % CATEGORY_COLORS.length],
      // 색을 한 바퀴 다 쓰고 나서 겹 테두리로 넘어간다.
      double: Math.floor(index / CATEGORY_COLORS.length) % 2 === 1,
    });
  });
  return styles;
}

/**
 * 한 사람의 소속 모양. 소속이 없으면 물러난 회색을 준다.
 * 배정표에 없는 이름(목록을 받기 전 등)도 같은 취급을 한다.
 */
export function styleFor(
  styles: Map<string, CategoryStyle>,
  category: string | undefined,
): CategoryStyle {
  return (category && styles.get(category)) || NO_CATEGORY;
}

/** 상태 색 목록. 카테고리 색이 여기에 들어가면 안 된다. */
export function stateTones() {
  return STATE_ORDER.map((state) => STATE_META[state].tone.toLowerCase());
}
