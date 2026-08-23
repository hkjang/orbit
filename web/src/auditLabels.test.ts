import { describe, expect, it } from "vitest";
import { AUDIT_LABELS, auditLabel } from "./auditLabels";

describe("auditLabel", () => {
  it("names the actions we know about", () => {
    expect(auditLabel("key.rotate")).toBe("키 회전");
    expect(auditLabel("person_link.upsert")).toBe("관계 연결 추가·수정");
  });

  it("shows an unknown action as-is instead of hiding it", () => {
    // 새 감사 항목이 생겨도 화면에서 사라지면 안 된다.
    expect(auditLabel("brand_new.action")).toBe("brand_new.action");
    expect(auditLabel("")).toBe("");
  });

  it("has no empty labels", () => {
    for (const [action, label] of Object.entries(AUDIT_LABELS)) {
      expect(label.trim(), `${action}의 이름이 비어 있다`).not.toBe("");
    }
  });
});
