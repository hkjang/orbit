import { describe, expect, it } from "vitest";
import {
  constellationEdges,
  daysSince,
  DORMANT_DAYS,
  DRIFT_DAYS,
  readGrammar,
} from "./orbitGrammar";

const NOW = Date.parse("2026-08-23T00:00:00Z");
const daysAgo = (days: number) =>
  new Date(NOW - days * 86_400_000).toISOString();

describe("readGrammar", () => {
  it("reads rising interaction as approaching", () => {
    const g = readGrammar(
      { momentum: 0.42, last_interaction_at: daysAgo(3) },
      NOW,
    );
    expect(g.state).toBe("approaching");
    expect(g.vector).toBeGreaterThan(0);
    expect(g.dormantDays).toBe(3);
  });

  it("treats small momentum swings as a stable orbit", () => {
    expect(
      readGrammar({ momentum: 0.1, last_interaction_at: daysAgo(10) }, NOW)
        .state,
    ).toBe("stable");
    expect(
      readGrammar({ momentum: -0.12, last_interaction_at: daysAgo(10) }, NOW)
        .state,
    ).toBe("stable");
  });

  it("reads falling interaction as drifting", () => {
    const g = readGrammar(
      { momentum: -0.4, last_interaction_at: daysAgo(20) },
      NOW,
    );
    expect(g.state).toBe("drifting");
    expect(g.vector).toBeLessThan(0);
  });

  it("drifts on dormancy alone even when momentum is flat", () => {
    expect(
      readGrammar(
        { momentum: 0, last_interaction_at: daysAgo(DRIFT_DAYS + 1) },
        NOW,
      ).state,
    ).toBe("drifting");
  });

  it("moves long-silent relationships past the event horizon", () => {
    const g = readGrammar(
      { momentum: 0.9, last_interaction_at: daysAgo(DORMANT_DAYS + 5) },
      NOW,
    );
    expect(g.state).toBe("dormant");
    expect(g.vector).toBe(-1);
  });

  it("keeps a person with no interactions yet out of dark orbit", () => {
    const g = readGrammar({ momentum: 0, last_interaction_at: undefined }, NOW);
    expect(g.state).toBe("stable");
    expect(g.dormantDays).toBeNull();
  });

  it("survives an unparsable timestamp", () => {
    expect(daysSince("not-a-date", NOW)).toBeNull();
    expect(
      readGrammar({ momentum: 0, last_interaction_at: "not-a-date" }, NOW)
        .state,
    ).toBe("stable");
  });
});

describe("constellationEdges", () => {
  it("returns nothing for a lone star", () => {
    expect(constellationEdges([{ px: 0, py: 0 }])).toHaveLength(0);
    expect(constellationEdges([])).toHaveLength(0);
  });

  it("links every member exactly once, nearest first", () => {
    const a = { px: 0, py: 0 },
      b = { px: 10, py: 0 },
      c = { px: 200, py: 0 },
      d = { px: 210, py: 0 };
    const edges = constellationEdges([a, b, c, d]);
    expect(edges).toHaveLength(3);
    const linked = new Set(edges.flat());
    expect(linked.size).toBe(4);
    // 가까운 쌍이 먼저 이어지고, 먼 성단은 한 줄로만 건너갑니다.
    expect(edges[0]).toEqual([a, b]);
    expect(edges.filter(([x, y]) => x === c || y === c)).toHaveLength(2);
  });
});
