import { describe, expect, it } from "vitest";
import { createLatestGuard, dayToTravelValue } from "./timeTravel";

const DAY = 86_400_000;
const START = Date.parse("2026-01-10T09:30:00Z");

describe("dayToTravelValue", () => {
  it("treats the last notch as the present", () => {
    expect(dayToTravelValue(START, 30, 30)).toBeUndefined();
  });

  it("does not let a notch past the end pretend to be the past", () => {
    expect(dayToTravelValue(START, 30, 31)).toBeUndefined();
  });

  it("maps the first notch onto the earliest day", () => {
    const at = dayToTravelValue(START, 30, 0);
    expect(at).toBe(new Date(START).toISOString());
  });

  it("returns an ISO timestamp for a notch before the end", () => {
    const at = dayToTravelValue(START, 30, 7);
    expect(at).toBe(new Date(START + 7 * DAY).toISOString());
    expect(Date.parse(at!)).toBe(START + 7 * DAY);
  });

  it("treats the notch just before the end as the past", () => {
    expect(dayToTravelValue(START, 30, 29)).toBe(
      new Date(START + 29 * DAY).toISOString(),
    );
  });
});

describe("createLatestGuard", () => {
  it("accepts the only request in flight", () => {
    const guard = createLatestGuard();
    const token = guard.next();
    expect(guard.isCurrent(token)).toBe(true);
  });

  it("drops an earlier request once a later one has started", () => {
    const guard = createLatestGuard();
    const first = guard.next();
    const second = guard.next();
    expect(guard.isCurrent(first)).toBe(false);
    expect(guard.isCurrent(second)).toBe(true);
  });

  it("applies only the last response even when they arrive out of order", async () => {
    const guard = createLatestGuard();
    const applied: string[] = [];
    // 각 요청은 자기 순번을 쥐고 있다가 응답이 오면 아직 최신인지 묻는다.
    const request = (name: string, delay: number) => {
      const token = guard.next();
      return new Promise<void>((resolve) =>
        setTimeout(() => {
          if (guard.isCurrent(token)) applied.push(name);
          resolve();
        }, delay),
      );
    };
    // 먼저 보낸 요청이 가장 늦게 돌아온다.
    await Promise.all([request("a", 30), request("b", 20), request("c", 5)]);
    expect(applied).toEqual(["c"]);
  });

  it("hands out a fresh token for every request", () => {
    const guard = createLatestGuard();
    const tokens = [guard.next(), guard.next(), guard.next()];
    expect(new Set(tokens).size).toBe(3);
  });
});
