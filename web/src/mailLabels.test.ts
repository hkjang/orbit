import { describe, expect, it } from "vitest";
import {
  defaultTestRecipient,
  deliveryStatus,
  mailEventLabel,
} from "./mailLabels";

describe("mailLabels", () => {
  it("names the events Orbit actually sends and passes unknown ones through", () => {
    expect(mailEventLabel("approval.requested")).toBe("검토 요청");
    expect(mailEventLabel("approval.decided")).toBe("검토 결과");
    expect(mailEventLabel("account.created")).toBe("계정 준비");
    expect(mailEventLabel("test")).toBe("시험 발송");
    expect(mailEventLabel("future.event")).toBe("future.event");
  });

  it("maps delivery statuses to a label and colour without hiding new ones", () => {
    expect(deliveryStatus("sent")).toEqual({ label: "발송됨", color: "success" });
    expect(deliveryStatus("failed")).toEqual({ label: "실패", color: "error" });
    expect(deliveryStatus("queued")).toEqual({ label: "대기", color: "default" });
    expect(deliveryStatus("bounced")).toEqual({
      label: "bounced",
      color: "default",
    });
  });

  it("prefills the test recipient only with a usable address", () => {
    expect(defaultTestRecipient(" admin@example.internal ")).toBe(
      "admin@example.internal",
    );
    expect(defaultTestRecipient("")).toBe("");
    expect(defaultTestRecipient(undefined)).toBe("");
    expect(defaultTestRecipient("not-an-address")).toBe("");
  });
});
