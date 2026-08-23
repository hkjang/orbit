import { afterEach, describe, expect, it, vi } from "vitest";
import { api, setUnauthorizedHandler } from "./api";

function respondWith(status: number, body: unknown = {}) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({
      ok: status >= 200 && status < 300,
      status,
      json: async () => body,
    })),
  );
}

afterEach(() => {
  setUnauthorizedHandler(undefined);
  vi.unstubAllGlobals();
});

describe("세션 만료 알림", () => {
  it("일반 요청이 401이면 알린다", async () => {
    const seen = vi.fn();
    setUnauthorizedHandler(seen);
    respondWith(401);
    await expect(api("/people/")).rejects.toThrow();
    expect(seen).toHaveBeenCalledOnce();
  });

  it("로그인 실패의 401은 세션 만료가 아니다", async () => {
    // 자격 증명이 틀린 것이지 세션이 끊긴 게 아니다. 여기서 알리면
    // 로그인 화면이 "세션이 만료되었습니다"를 띄우며 스스로를 설명하지 못한다.
    const seen = vi.fn();
    setUnauthorizedHandler(seen);
    respondWith(401);
    await expect(
      api("/auth/login", { method: "POST", body: "{}" }),
    ).rejects.toThrow();
    expect(seen).not.toHaveBeenCalled();
  });

  it("다른 오류는 알리지 않는다", async () => {
    const seen = vi.fn();
    setUnauthorizedHandler(seen);
    for (const status of [400, 403, 404, 500]) {
      respondWith(status);
      await expect(api("/people/")).rejects.toThrow();
    }
    expect(seen).not.toHaveBeenCalled();
  });

  it("성공한 요청은 알리지 않는다", async () => {
    const seen = vi.fn();
    setUnauthorizedHandler(seen);
    respondWith(200, { people: [] });
    await api("/people/");
    expect(seen).not.toHaveBeenCalled();
  });

  it("등록된 처리기가 없어도 요청은 정상적으로 실패한다", async () => {
    respondWith(401);
    await expect(api("/people/")).rejects.toThrow();
  });
});
