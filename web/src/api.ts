export class APIError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

/**
 * 세션이 끊겼을 때 알림을 받을 곳.
 *
 * 만료는 서버만 안다. 화면은 로그인한 사용자를 기억한 채로 남아 있으므로,
 * 알려주지 않으면 모든 요청이 "로그인이 필요합니다"로 실패하는 화면에
 * 갇힌다. 로그인 화면으로 돌아갈 길을 여기서 연다.
 */
let onUnauthorized: (() => void) | undefined;

export function setUnauthorizedHandler(handler: (() => void) | undefined) {
  onUnauthorized = handler;
}

// 로그인 자체의 401은 자격 증명이 틀린 것이지 세션이 끊긴 게 아니다.
const AUTH_PATHS = ["/auth/login", "/auth/logout", "/auth/oidc"];

function reportUnauthorized(path: string, status: number) {
  if (status !== 401) return;
  if (AUTH_PATHS.some((prefix) => path.startsWith(prefix))) return;
  onUnauthorized?.();
}

export async function api<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    credentials: "same-origin",
    ...options,
    headers: { "Content-Type": "application/json", ...options.headers },
  });
  if (!response.ok) {
    let data: { error?: { code?: string; message?: string } } = {};
    try {
      data = await response.json();
    } catch {
      /* response was not JSON */
    }
    reportUnauthorized(path, response.status);
    throw new APIError(
      response.status,
      data.error?.code ?? "request_failed",
      data.error?.message ?? "요청을 처리하지 못했습니다.",
    );
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export async function streamAI(
  payload: { prompt: string; person_id?: string; max_output_tokens?: number },
  onDelta: (text: string) => void,
  signal?: AbortSignal,
) {
  const response = await fetch("/api/v1/ai/stream", {
    method: "POST",
    credentials: "same-origin",
    signal,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    reportUnauthorized("/ai/stream", response.status);
    throw new APIError(
      response.status,
      data.error?.code ?? "ai_failed",
      data.error?.message ?? "AI 응답을 시작하지 못했습니다.",
    );
  }
  if (!response.body) throw new Error("스트리밍 응답을 사용할 수 없습니다.");
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const frames = buffer.split("\n\n");
    buffer = frames.pop() ?? "";
    for (const frame of frames) {
      const event = frame
        .split("\n")
        .find((line) => line.startsWith("event:"))
        ?.slice(6)
        .trim();
      const dataLine = frame
        .split("\n")
        .find((line) => line.startsWith("data:"))
        ?.slice(5)
        .trim();
      if (!dataLine) continue;
      const data = JSON.parse(dataLine);
      if (event === "delta") onDelta(data.text);
      if (event === "error") throw new Error(data.message);
    }
  }
}

export function formatDate(value?: string, withTime = false) {
  if (!value) return "기록 없음";
  return new Intl.DateTimeFormat(
    "ko-KR",
    withTime
      ? { dateStyle: "medium", timeStyle: "short" }
      : { dateStyle: "medium" },
  ).format(new Date(value));
}
