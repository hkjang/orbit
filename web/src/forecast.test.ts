import { describe, expect, it } from "vitest";
import {
  daysUntilDarkOrbit,
  forecastAt,
  forecastSeries,
  isDegrading,
  projectQuietDays,
} from "./forecast";
import { DORMANT_DAYS, DRIFT_DAYS } from "./orbitGrammar";

const NOW = Date.parse("2026-08-23T00:00:00Z");
const daysAgo = (days: number) =>
  new Date(NOW - days * 86_400_000).toISOString();

describe("projectQuietDays", () => {
  it("keeps a stable rhythm from accumulating silence", () => {
    expect(projectQuietDays(20, "stable", 90)).toBe(20);
  });

  it("shortens silence for a relationship that is warming up", () => {
    expect(projectQuietDays(20, "approaching", 90)).toBe(0);
    expect(projectQuietDays(80, "approaching", 60)).toBe(50);
  });

  it("accumulates the full horizon when nothing is expected", () => {
    expect(projectQuietDays(100, "drifting", 60)).toBe(160);
    expect(projectQuietDays(200, "dormant", 30)).toBe(230);
  });
});

describe("forecastAt", () => {
  it("sees a drifting relationship crossing into dark orbit", () => {
    const node = { momentum: -0.4, last_interaction_at: daysAgo(150) };
    expect(forecastAt(node, 30, NOW)!.grammar.state).toBe("dormant");
  });

  it("keeps a warm relationship warm", () => {
    const node = { momentum: 0.6, last_interaction_at: daysAgo(3) };
    const later = forecastAt(node, 90, NOW)!;
    expect(later.quietDays).toBe(0);
    expect(later.grammar.state).not.toBe("dormant");
  });

  it("lets momentum cool off instead of running forever", () => {
    const node = { momentum: 0.9, last_interaction_at: daysAgo(10) };
    const near = forecastAt(node, 30, NOW)!;
    const far = forecastAt(node, 90, NOW)!;
    // 90일 뒤에도 여전히 다가오는 중이라고 우기지 않습니다.
    expect(far.grammar.vector).toBeLessThan(near.grammar.vector);
  });

  it("holds an anchored relationship out of dark orbit even in the forecast", () => {
    const node = {
      momentum: 0,
      last_interaction_at: daysAgo(400),
      anchored: true,
    };
    expect(forecastAt(node, 90, NOW)!.grammar.state).toBe("stable");
  });

  it("declines to guess when there is no interaction history", () => {
    expect(forecastAt({ momentum: 0 }, 30, NOW)).toBeNull();
    expect(forecastSeries({ momentum: 0 }, [30, 60], NOW)).toHaveLength(0);
  });

  it("returns one entry per horizon", () => {
    const node = { momentum: -0.3, last_interaction_at: daysAgo(100) };
    expect(forecastSeries(node, [30, 60, 90], NOW).map((f) => f.horizonDays))
      .toEqual([30, 60, 90]);
  });
});

describe("daysUntilDarkOrbit", () => {
  it("counts the days left for a drifting relationship", () => {
    const node = { momentum: -0.4, last_interaction_at: daysAgo(150) };
    expect(daysUntilDarkOrbit(node, NOW)).toBe(DORMANT_DAYS - 150);
  });

  it("stays silent for relationships that are not heading there", () => {
    expect(
      daysUntilDarkOrbit(
        { momentum: 0.5, last_interaction_at: daysAgo(2) },
        NOW,
      ),
    ).toBeNull();
    expect(
      daysUntilDarkOrbit({ momentum: 0, last_interaction_at: daysAgo(10) }, NOW),
    ).toBeNull();
  });

  it("says nothing about someone already there, or anchored", () => {
    expect(
      daysUntilDarkOrbit(
        { momentum: 0, last_interaction_at: daysAgo(400) },
        NOW,
      ),
    ).toBeNull();
    expect(
      daysUntilDarkOrbit(
        { momentum: -0.9, last_interaction_at: daysAgo(120), anchored: true },
        NOW,
      ),
    ).toBeNull();
  });
});

describe("isDegrading", () => {
  it("flags a relationship whose state gets worse", () => {
    expect(
      isDegrading(
        { momentum: 0, last_interaction_at: daysAgo(DRIFT_DAYS + 5) },
        90,
        NOW,
      ),
    ).toBe(true);
  });

  it("leaves steady and warming relationships alone", () => {
    expect(
      isDegrading({ momentum: 0, last_interaction_at: daysAgo(10) }, 90, NOW),
    ).toBe(false);
    expect(
      isDegrading({ momentum: 0.5, last_interaction_at: daysAgo(3) }, 90, NOW),
    ).toBe(false);
  });

  it("cannot degrade past dark orbit", () => {
    expect(
      isDegrading({ momentum: 0, last_interaction_at: daysAgo(400) }, 90, NOW),
    ).toBe(false);
  });
});
