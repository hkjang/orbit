/**
 * 기억을 다른 서비스로 넘기기 — 화면 쪽 규칙.
 *
 * 사내 표준(HANDOFF-STANDARD.md)의 보내는 쪽이다. 서버가 표(claim)를 발급하면
 * 브라우저는 새 창에서 받는 쪽의 `/handoff?source=…&claim=…` 를 연다. 사람이
 * 파일을 내려받지 않는다.
 *
 * 보낼 곳 목록은 관리자가 적은 허용 목록에서 오고, 기본은 비어 있다. 비어
 * 있으면 단추가 그려지지 않으므로 새로 설치한 곳에서는 아무것도 달라지지 않는다.
 */

export interface HandoffTarget {
  name: string;
  origin: string;
}

export interface HandoffTargets {
  targets: HandoffTarget[];
  source: string;
  format: string;
}

export interface HandoffClaim {
  claim: string;
  source: string;
  filename: string;
  content_type: string;
  bytes: number;
  expires_at: string;
}

/**
 * 받는 쪽이 열릴 주소. `source` 와 `claim` 은 쿼리에 실리므로 반드시 인코딩한다 —
 * 오리진의 `:`·`/` 가 그대로 들어가면 받는 쪽이 다른 값을 읽는다.
 */
export function handoffURL(
  target: HandoffTarget,
  claim: Pick<HandoffClaim, "claim" | "source">,
) {
  const params = new URLSearchParams({
    source: claim.source,
    claim: claim.claim,
  });
  return `${target.origin.replace(/\/+$/, "")}/handoff?${params}`;
}

/** 이 기억에 보내기 단추를 그릴지. 목록이 비었거나 검토가 안 끝났으면 아니다. */
export function canHandoff(
  targets: HandoffTarget[] | undefined,
  status: string,
) {
  return Boolean(targets && targets.length > 0 && status === "approved");
}
