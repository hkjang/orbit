import type { PublicConfig } from "./types";

/**
 * 조용한 SSO(prompt=none)를 언제 시도할지 정하는 규칙.
 *
 * prompt=none은 화면을 그리지 않는다. 제공자에 세션이 있으면 코드가 곧바로
 * 돌아오고, 없으면 login_required로 돌아온다. 그때 다시 시도하면 브라우저가
 * 제공자와 앱 사이를 끝없이 오가고 사용자는 깜빡이는 화면만 본다. 그래서 막는
 * 장치를 세 겹으로 둔다: 한 탭 세션에 한 번, 스스로 로그아웃했으면 억제,
 * 콜백이 거절을 받으면 주소(/login?sso=none)에도 표시.
 */

// localStorage가 아니라 sessionStorage다. 새 탭에서는 다시 시도하고, 거절당한
// 뒤 새로고침하면 다시 시도하지 않는 것이 맞다.
const ATTEMPTED_KEY = "orbit:sso:silent-attempted";
const SIGNED_OUT_KEY = "orbit:sso:signed-out";

function readFlag(key: string): boolean {
  try {
    return window.sessionStorage.getItem(key) === "true";
  } catch {
    // 사생활 보호 모드나 사이트 데이터가 막힌 브라우저에서는 예외가 난다.
    // 그것을 "아직 안 했다"로 읽으면 바로 루프가 되므로 "이미 했다"로 친다.
    return true;
  }
}

function writeFlag(key: string, value: boolean) {
  try {
    if (value) window.sessionStorage.setItem(key, "true");
    else window.sessionStorage.removeItem(key);
  } catch {
    /* 읽기 쪽이 이미 막히는 방향으로 실패한다 */
  }
}

/** 스스로 로그아웃했음을 남긴다. 직후에 다시 조용히 로그인시키면 로그아웃이 고장 난 것처럼 보인다. */
export function markSignedOut() {
  writeFlag(SIGNED_OUT_KEY, true);
  writeFlag(ATTEMPTED_KEY, true);
}

/** 세션이 다시 생기면 억제를 푼다. */
export function clearSilentSsoState() {
  writeFlag(SIGNED_OUT_KEY, false);
  writeFlag(ATTEMPTED_KEY, false);
}

// 콜백·로그인 경로는 가장 흔한 루프의 출처고, API·MCP·헬스 경로는 브라우저
// 이동이 아니다. 화면이 이 경로에서 뜨는 일은 드물지만 규칙은 여기서 지킨다.
const EXCLUDED_PREFIXES = ["/login", "/api/", "/mcp", "/healthz", "/readyz"];

export function silentSsoExcludedPath(pathname: string): boolean {
  return EXCLUDED_PREFIXES.some((prefix) => pathname.startsWith(prefix));
}

/** 로그인 뒤 돌아갈 자리로 같은 사이트 안의 경로만 받는다. */
export function safeReturnTo(value: string): string {
  return value.startsWith("/") && !value.startsWith("//") && !value.startsWith("/\\")
    ? value
    : "/";
}

/**
 * 로그인 화면을 그리기 전에 조용한 로그인을 시도할지 답한다.
 *
 * 한 탭의 브라우징 세션에서 두 번 이상 참이 되어서는 안 된다.
 */
export function shouldAttemptSilentSso(
  config: PublicConfig | null,
  location: { pathname: string; search: string },
): boolean {
  if (!config?.oidc.enabled || !config.oidc.auto_login) return false;
  if (silentSsoExcludedPath(location.pathname)) return false;
  // 콜백이 거절을 받으면 이 표시를 붙여 보낸다. 그 사이 sessionStorage가
  // 지워졌더라도 이 표시가 있으면 다시 시도하지 않는다.
  const sso = new URLSearchParams(location.search).get("sso");
  if (sso === "none" || sso === "error") return false;
  if (readFlag(SIGNED_OUT_KEY)) return false;
  if (readFlag(ATTEMPTED_KEY)) return false;
  return true;
}

/** 조용한 시도가 시작할 주소. 시도했다는 표시를 먼저 남긴다. */
export function beginSilentSso(returnTo: string): string {
  writeFlag(ATTEMPTED_KEY, true);
  return `/api/v1/auth/oidc/start?prompt=none&return_to=${encodeURIComponent(safeReturnTo(returnTo))}`;
}
