import { describe, expect, it } from "vitest";
import {
  MAX_SNIPPET_BYTES,
  TRACKING_PROVIDERS,
  defaultTrackingSettings,
  policySources,
  snippetBytes,
  snippetOrigins,
  splitHosts,
  trackingIssue,
} from "./tracking";

function momento(proxy = true) {
  return {
    ...defaultTrackingSettings(),
    enabled: true,
    provider: "momento" as const,
    momento_url: "https://momento.corp.example",
    momento_site_id: "SITE_ORBIT_001",
    momento_proxy: proxy,
  };
}

describe("tracking defaults", () => {
  it("꺼져 있고 Momento 가 첫 자리다", () => {
    const settings = defaultTrackingSettings();
    expect(settings.enabled).toBe(false);
    expect(settings.provider).toBe("none");
    expect(settings.momento_proxy).toBe(true);
    expect(TRACKING_PROVIDERS[0].value).toBe("momento");
    expect(trackingIssue(settings)).toBe("");
  });
});

describe("trackingIssue", () => {
  it("켜면 고른 도구의 빈 칸을 말한다", () => {
    expect(trackingIssue({ ...defaultTrackingSettings(), enabled: true })).toBe(
      "추적 도구를 고르세요.",
    );
    expect(trackingIssue({ ...momento(), momento_site_id: " " })).toContain(
      "Momento",
    );
    expect(
      trackingIssue({ ...momento(), momento_url: "momento.corp" }),
    ).toContain("Momento");
    expect(trackingIssue(momento())).toBe("");
    expect(
      trackingIssue({ ...momento(), provider: "ga4", measurement_id: "" }),
    ).toContain("측정 ID");
    expect(
      trackingIssue({ ...momento(), provider: "custom", custom_snippet: "" }),
    ).toContain("비어");
  });

  it("꺼져 있어도 8KB 상한은 지킨다", () => {
    const large = "가".repeat(MAX_SNIPPET_BYTES / 3 + 1);
    expect(snippetBytes(large)).toBeGreaterThan(MAX_SNIPPET_BYTES);
    expect(
      trackingIssue({ ...defaultTrackingSettings(), custom_snippet: large }),
    ).toContain("바이트");
    expect(
      trackingIssue({
        ...defaultTrackingSettings(),
        custom_snippet: "a".repeat(MAX_SNIPPET_BYTES),
      }),
    ).toBe("");
  });
});

describe("policySources", () => {
  it("Momento 프록시는 외부 출처를 더하지 않는다", () => {
    expect(policySources(momento(true))).toEqual([]);
    expect(policySources(momento(false))).toEqual([
      "https://momento.corp.example",
    ]);
  });

  it("붙여 넣은 스니펫에서 출처를 읽고 허용 목록을 합친다", () => {
    const snippet =
      'İ<script src=\'HTTPS://Cdn.Example/loader.js\'></script><script>fetch("https://collect.example/v1/events");new Image().src="https://cdn.example/px.gif"+q;</script>';
    expect(snippetOrigins(snippet)).toEqual([
      "https://cdn.example",
      "https://collect.example",
    ]);
    expect(
      policySources({
        ...defaultTrackingSettings(),
        provider: "custom",
        custom_snippet: snippet,
        allowed_hosts:
          "https://px.example, https://cdn.example\nhttps://b.example",
      }),
    ).toEqual([
      "https://cdn.example",
      "https://collect.example",
      "https://px.example",
      "https://b.example",
    ]);
    expect(splitHosts("  ")).toEqual([]);
  });
});
