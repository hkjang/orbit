/**
 * 방문 추적 설정의 화면 쪽 규칙.
 *
 * 서버(internal/tracking)가 최종 판단을 하지만, 관리자가 저장을 누르기 전에
 * 무엇이 빠졌고 정책에 무엇이 더해질지 보여 주려면 같은 규칙이 화면에도
 * 있어야 한다. 여기 있는 것은 그 미리보기다 — 서버와 어긋나면 서버가 맞다.
 */

export type TrackingProvider =
  "none" | "momento" | "ga4" | "gtm" | "matomo" | "custom";

export interface TrackingSettings {
  enabled: boolean;
  provider: TrackingProvider;
  momento_url: string;
  momento_site_id: string;
  momento_proxy: boolean;
  measurement_id: string;
  matomo_url: string;
  matomo_site_id: string;
  custom_snippet: string;
  allowed_hosts: string;
  include_admin: boolean;
  placement: "head" | "body";
}

export interface TrackingViolation {
  origin: string;
  directive: string;
  page: string;
  count: number;
  first_seen: string;
  last_seen: string;
  allowed: boolean;
}

/** Momento 가 첫 자리다. 사내 수집기라 데이터가 밖으로 나가지 않는 유일한 선택지다. */
export const TRACKING_PROVIDERS: { value: TrackingProvider; label: string }[] =
  [
    { value: "momento", label: "Momento (사내 수집기)" },
    { value: "ga4", label: "Google Analytics 4" },
    { value: "gtm", label: "Google Tag Manager" },
    { value: "matomo", label: "Matomo" },
    { value: "custom", label: "직접 붙여 넣기" },
  ];

export const MAX_SNIPPET_BYTES = 8 * 1024;

/** 같은 오리진 프록시 경로. 서버의 tracking.ProxyPath 와 같아야 한다. */
export const MOMENTO_PROXY_PATH = "/momento";

export function defaultTrackingSettings(): TrackingSettings {
  return {
    enabled: false,
    provider: "none",
    momento_url: "",
    momento_site_id: "",
    momento_proxy: true,
    measurement_id: "",
    matomo_url: "",
    matomo_site_id: "",
    custom_snippet: "",
    allowed_hosts: "",
    include_admin: false,
    placement: "head",
  };
}

/** 스니펫 상한은 글자 수가 아니라 바이트다. 한글 한 글자는 3바이트다. */
export function snippetBytes(snippet: string): number {
  return new TextEncoder().encode(snippet).length;
}

function isHttpUrl(raw: string): boolean {
  try {
    const url = new URL(raw.trim());
    return (
      (url.protocol === "http:" || url.protocol === "https:") && !!url.host
    );
  } catch {
    return false;
  }
}

/**
 * 저장을 막을 문제를 한 줄로 말한다. 없으면 빈 문자열.
 * 꺼져 있으면 값을 미리 적어 둘 수 있으므로 상한만 본다.
 */
export function trackingIssue(settings: TrackingSettings): string {
  if (snippetBytes(settings.custom_snippet) > MAX_SNIPPET_BYTES)
    return `추적 코드는 ${MAX_SNIPPET_BYTES.toLocaleString()}바이트를 넘을 수 없습니다.`;
  if (!settings.enabled) return "";
  switch (settings.provider) {
    case "none":
      return "추적 도구를 고르세요.";
    case "momento":
      if (!isHttpUrl(settings.momento_url) || !settings.momento_site_id.trim())
        return "Momento 수집기 주소와 사이트 ID를 확인해 주세요.";
      return "";
    case "ga4":
    case "gtm":
      return settings.measurement_id.trim() ? "" : "측정 ID를 입력해 주세요.";
    case "matomo":
      if (!isHttpUrl(settings.matomo_url) || !settings.matomo_site_id.trim())
        return "Matomo 주소와 사이트 ID를 확인해 주세요.";
      return "";
    case "custom":
      return settings.custom_snippet.trim()
        ? ""
        : "붙여 넣을 추적 코드가 비어 있습니다.";
  }
}

function originOf(raw: string): string {
  try {
    const url = new URL(raw.trim());
    return url.host ? `${url.protocol}//${url.host}`.toLowerCase() : "";
  } catch {
    return "";
  }
}

/** 스니펫에 적힌 http(s) 출처. 서버의 SnippetOrigins 와 같은 답을 내야 한다. */
export function snippetOrigins(snippet: string): string[] {
  const seen = new Set<string>();
  for (const match of snippet.matchAll(/https?:\/\/[^"'`<>\s),;\\+]+/gi)) {
    const origin = originOf(match[0]);
    if (origin) seen.add(origin);
  }
  return [...seen];
}

export function splitHosts(list: string): string[] {
  return list
    .split(/[,\s]+/)
    .map((host) => host.trim())
    .filter(Boolean);
}

/**
 * 이 설정으로 정책에 더해질 출처. 비어 있으면 외부 출처 없이 nonce 만으로
 * 동작한다는 뜻이다 — Momento 프록시가 그렇다.
 */
export function policySources(settings: TrackingSettings): string[] {
  const sources: string[] = [];
  switch (settings.provider) {
    case "momento":
      if (!settings.momento_proxy) {
        const origin = originOf(settings.momento_url);
        if (origin) sources.push(origin);
      }
      break;
    case "ga4":
    case "gtm":
      sources.push(
        "https://www.googletagmanager.com",
        "https://www.google-analytics.com",
        "https://analytics.google.com",
        "https://*.google-analytics.com",
      );
      break;
    case "matomo": {
      const origin = originOf(settings.matomo_url);
      if (origin) sources.push(origin);
      break;
    }
    case "custom":
      sources.push(...snippetOrigins(settings.custom_snippet));
      break;
  }
  for (const host of splitHosts(settings.allowed_hosts))
    if (!sources.includes(host)) sources.push(host);
  return sources;
}
