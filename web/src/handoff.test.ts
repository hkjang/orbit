import { describe, expect, it } from "vitest";
import { canHandoff, handoffURL } from "./handoff";

describe("다른 서비스로 보내기", () => {
  it("받는 쪽의 /handoff 에 source 와 claim 을 인코딩해 싣는다", () => {
    const url = handoffURL(
      { name: "Ptium", origin: "https://ptium.intra" },
      { claim: "abc+def/=", source: "https://orbit.intra:8443" },
    );
    expect(url).toBe(
      "https://ptium.intra/handoff?source=https%3A%2F%2Forbit.intra%3A8443&claim=abc%2Bdef%2F%3D",
    );
    const parsed = new URL(url);
    expect(parsed.searchParams.get("source")).toBe("https://orbit.intra:8443");
    expect(parsed.searchParams.get("claim")).toBe("abc+def/=");
  });

  it("오리진 끝의 슬래시로 경로가 두 겹이 되지 않는다", () => {
    const url = handoffURL(
      { name: "Muni", origin: "https://muni.intra/" },
      { claim: "c", source: "https://orbit.intra" },
    );
    expect(url.startsWith("https://muni.intra/handoff?")).toBe(true);
  });

  it("허용 목록이 비어 있으면 단추가 없다", () => {
    expect(canHandoff([], "approved")).toBe(false);
    expect(canHandoff(undefined, "approved")).toBe(false);
  });

  it("검토가 끝난 기억에만 단추가 있다", () => {
    const targets = [{ name: "Ptium", origin: "https://ptium.intra" }];
    expect(canHandoff(targets, "approved")).toBe(true);
    expect(canHandoff(targets, "pending")).toBe(false);
    expect(canHandoff(targets, "rejected")).toBe(false);
  });
});
