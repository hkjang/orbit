/**
 * 감사 로그 액션의 우리말 이름.
 *
 * 모르는 액션은 원문 그대로 보여준다. 새 감사 항목이 생겼을 때 화면에서
 * 사라지거나 "알 수 없음"으로 뭉뚱그려지면, 정작 봐야 할 기록을 놓친다.
 */
export const AUDIT_LABELS: Record<string, string> = {
  "ai.invoke": "AI 질의",
  "api_key.create": "API 키 발급",
  "api_key.revoke": "API 키 폐기",
  "approval.approved": "요청 승인",
  "approval.rejected": "요청 반려",
  "auth.login": "로그인",
  "data.export": "내 기록 내보내기",
  "interaction.create": "교류 기록",
  "key.rotate": "키 회전",
  "key_permission.update": "키 권한 변경",
  "memory.create": "기억 생성",
  "person.create": "인물 등록",
  "person.delete": "인물 삭제",
  "person.update": "인물 수정",
  "person_link.delete": "관계 연결 해제",
  "person_link.upsert": "관계 연결 추가·수정",
  "setting.update": "설정 변경",
  "user.create": "사용자 생성",
  "user.update": "사용자 수정",
};

export function auditLabel(action: string) {
  return AUDIT_LABELS[action] ?? action;
}
