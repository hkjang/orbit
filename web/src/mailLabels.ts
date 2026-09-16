/**
 * 메일 알림 화면의 우리말 이름.
 *
 * 이벤트와 상태는 서버가 정한 식별자다. 모르는 값은 원문 그대로 보여 준다 —
 * 새 이벤트가 생겼을 때 "알 수 없음"으로 뭉뚱그려지면 정작 봐야 할 기록을 놓친다.
 */
export const MAIL_EVENT_LABELS: Record<string, string> = {
  "approval.requested": "검토 요청",
  "approval.decided": "검토 결과",
  "account.created": "계정 준비",
  test: "시험 발송",
};

export function mailEventLabel(event: string) {
  return MAIL_EVENT_LABELS[event] ?? event;
}

export type DeliveryStatus = "queued" | "sent" | "failed";

export const DELIVERY_STATUS: Record<
  DeliveryStatus,
  { label: string; color: "default" | "success" | "error" }
> = {
  queued: { label: "대기", color: "default" },
  sent: { label: "발송됨", color: "success" },
  failed: { label: "실패", color: "error" },
};

export function deliveryStatus(status: string) {
  return (
    DELIVERY_STATUS[status as DeliveryStatus] ?? {
      label: status,
      color: "default" as const,
    }
  );
}

/**
 * 시험 발송의 받는 사람 기본값. 관리자 자신의 주소가 있으면 그것이 가장
 * 빨리 확인할 수 있는 곳이고, 없으면 비워 두어 직접 적게 한다.
 */
export function defaultTestRecipient(email?: string) {
  const trimmed = (email ?? "").trim();
  return trimmed.includes("@") ? trimmed : "";
}
