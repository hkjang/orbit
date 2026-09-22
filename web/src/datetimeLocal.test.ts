import { afterEach, describe, expect, it, vi } from "vitest";
import { toDatetimeLocalValue } from "./datetimeLocal";

function withTZ<T>(tz: string, run: () => T): T {
  vi.stubEnv("TZ", tz);
  return run();
}

afterEach(() => {
  vi.unstubAllEnvs();
});

describe("toDatetimeLocalValue", () => {
  it("writes the local wall clock, not the UTC one", () => {
    const at = new Date(Date.UTC(2026, 8, 22, 2, 30));
    expect(withTZ("America/New_York", () => toDatetimeLocalValue(at))).toBe(
      "2026-09-21T22:30",
    );
    expect(withTZ("Asia/Seoul", () => toDatetimeLocalValue(at))).toBe(
      "2026-09-22T11:30",
    );
  });

  it("round-trips through the field back to the same minute", () => {
    // 칸에 넣은 값을 `new Date(...)` 로 다시 읽는 것이 저장 경로다.
    // 시차만큼 어긋나면 미래 시각이 되어 서버가 400 으로 막는다.
    const at = new Date(Date.UTC(2026, 8, 22, 2, 30, 45));
    for (const tz of ["America/New_York", "Asia/Seoul", "UTC"]) {
      const sent = withTZ(tz, () =>
        new Date(toDatetimeLocalValue(at)).getTime(),
      );
      expect(sent - at.getTime()).toBe(-45_000);
    }
  });

  it("pads single digit months, days, hours and minutes", () => {
    expect(
      withTZ("UTC", () =>
        toDatetimeLocalValue(new Date(Date.UTC(2026, 0, 2, 3, 4))),
      ),
    ).toBe("2026-01-02T03:04");
  });
});
