/**
 * `datetime-local` 입력칸과 시각 사이를 오가는 순수 변환.
 *
 * 이 칸은 값을 **로컬 벽시계**로 읽는다(`new Date("2026-09-22T13:21")` 은
 * 그 지역 시각이다). 그래서 기본값도 로컬 벽시계로 채워야 한다.
 * `toISOString().slice(0, 16)` 처럼 UTC 벽시계를 넣으면 시차만큼 어긋난
 * 시각이 보이고, 그대로 저장하면 시차만큼 어긋난 시각이 전송된다.
 */

const pad = (n: number) => String(n).padStart(2, "0");

/**
 * 시각을 `datetime-local` 칸이 읽는 `YYYY-MM-DDTHH:mm` 로 바꾼다.
 *
 * 로컬 구성요소를 그대로 적으므로 서머타임 경계에서도 화면에 보이는 시각과
 * 저장되는 시각이 같다.
 */
export function toDatetimeLocalValue(at: Date): string {
  return (
    `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}` +
    `T${pad(at.getHours())}:${pad(at.getMinutes())}`
  );
}
