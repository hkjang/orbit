import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  beginSilentSso,
  clearSilentSsoState,
  markSignedOut,
  safeReturnTo,
  shouldAttemptSilentSso,
} from "./silentSso";
import type { PublicConfig } from "./types";

function config(oidc: Partial<PublicConfig["oidc"]> = {}): PublicConfig {
  return {
    service_name: "Orbit",
    version: "test",
    commit: "",
    built_at: "",
    oidc: {
      enabled: true,
      display_name: "Keycloak SSO",
      auto_login: true,
      ...oidc,
    },
  };
}

const home = { pathname: "/orbit", search: "" };

beforeEach(() => window.sessionStorage.clear());
afterEach(() => vi.unstubAllGlobals());

describe("조용한 SSO 시도 규칙", () => {
  it("auto_login이 켜진 첫 방문에는 시도한다", () => {
    expect(shouldAttemptSilentSso(config(), home)).toBe(true);
  });

  it("auto_login이 꺼져 있거나 OIDC가 없으면 시도하지 않는다", () => {
    expect(shouldAttemptSilentSso(config({ auto_login: false }), home)).toBe(
      false,
    );
    expect(shouldAttemptSilentSso(config({ enabled: false }), home)).toBe(
      false,
    );
    expect(shouldAttemptSilentSso(null, home)).toBe(false);
  });

  it("한 탭 세션에 한 번만 시도한다", () => {
    // 거절당한 뒤 새로고침해도 다시 제공자로 가면 브라우저가 끝없이 오간다.
    expect(shouldAttemptSilentSso(config(), home)).toBe(true);
    beginSilentSso("/orbit");
    expect(shouldAttemptSilentSso(config(), home)).toBe(false);
  });

  it("콜백이 거절 표시를 붙여 보낸 주소에서는 시도하지 않는다", () => {
    // sessionStorage가 지워졌더라도 주소의 표시만으로 막혀야 한다.
    window.sessionStorage.clear();
    expect(
      shouldAttemptSilentSso(config(), { pathname: "/orbit", search: "?sso=none" }),
    ).toBe(false);
    expect(
      shouldAttemptSilentSso(config(), { pathname: "/orbit", search: "?sso=error" }),
    ).toBe(false);
  });

  it("스스로 로그아웃한 뒤에는 시도하지 않고, 다시 로그인하면 풀린다", () => {
    markSignedOut();
    expect(shouldAttemptSilentSso(config(), home)).toBe(false);
    clearSilentSsoState();
    expect(shouldAttemptSilentSso(config(), home)).toBe(true);
  });

  it("저장소를 읽지 못하면 이미 시도한 것으로 친다", () => {
    // 사생활 보호 모드의 예외를 "아직 안 했다"로 읽으면 바로 루프가 된다.
    const broken = new Proxy(
      {},
      {
        get: () => () => {
          throw new Error("SecurityError");
        },
      },
    );
    vi.stubGlobal("sessionStorage", broken);
    expect(shouldAttemptSilentSso(config(), home)).toBe(false);
    // 쓰기 실패도 예외를 밖으로 내지 않는다.
    expect(() => beginSilentSso("/orbit")).not.toThrow();
  });

  it("콜백·로그인·API·MCP·헬스 경로에서는 시도하지 않는다", () => {
    for (const pathname of [
      "/login",
      "/login?sso=none",
      "/api/v1/auth/oidc/callback",
      "/mcp",
      "/healthz",
      "/readyz",
    ]) {
      expect(shouldAttemptSilentSso(config(), { pathname, search: "" })).toBe(
        false,
      );
    }
  });
});

describe("돌아갈 자리", () => {
  it("깊은 링크를 return_to로 들고 간다", () => {
    expect(beginSilentSso("/people/abc?tab=links")).toBe(
      "/api/v1/auth/oidc/start?prompt=none&return_to=%2Fpeople%2Fabc%3Ftab%3Dlinks",
    );
  });

  it("사이트 밖으로 나가는 값은 받지 않는다", () => {
    expect(safeReturnTo("/orbit")).toBe("/orbit");
    expect(safeReturnTo("//evil.example/x")).toBe("/");
    expect(safeReturnTo("/\\evil.example")).toBe("/");
    expect(safeReturnTo("https://evil.example")).toBe("/");
    expect(safeReturnTo("")).toBe("/");
  });
});
