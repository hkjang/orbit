/**
 * Time Travel 슬라이더의 순수 로직.
 *
 * 화면(OrbitPage)에서 떼어 둔 두 가지: 슬라이더 칸을 시점으로 바꾸는 계산과,
 * 응답이 순서대로 오지 않아도 마지막 요청의 결과만 화면에 남기는 순번 지킴이.
 */

const DAY = 86_400_000;

/**
 * 슬라이더 칸(day)을 서버에 보낼 시점으로 바꾼다.
 *
 * 마지막 칸(`days`) 이상은 '지금'이라 `undefined`, 그 앞은 `earliest`(start)
 * 에서 하루씩 더한 ISO 문자열. 첫 칸은 `earliest` 그 자체다.
 */
export function dayToTravelValue(
  startMs: number,
  days: number,
  day: number,
): string | undefined {
  const at = new Date(startMs + day * DAY);
  const dayOf = Math.round((at.getTime() - startMs) / DAY);
  return dayOf >= days ? undefined : at.toISOString();
}

/**
 * 요청 순번 지킴이.
 *
 * 요청을 보낼 때 `next()` 로 순번을 받아 두고, 응답이 왔을 때
 * `isCurrent(순번)` 이 거짓이면 그 사이 더 새 요청이 나간 것이니 버린다.
 */
export function createLatestGuard() {
  let latest = 0;
  return {
    next(): number {
      latest += 1;
      return latest;
    },
    isCurrent(token: number): boolean {
      return token === latest;
    },
  };
}
