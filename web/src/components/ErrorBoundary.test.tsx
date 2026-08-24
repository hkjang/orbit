import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ErrorBoundary, isStaleChunkError } from "./ErrorBoundary";

function Boom({ message }: { message: string }): never {
  throw new Error(message);
}

// 경계가 잡은 오류를 React가 콘솔로도 내보낸다. 테스트 출력만 시끄러워지므로 가린다.
beforeEach(() => {
  vi.spyOn(console, "error").mockImplementation(() => {});
});
afterEach(() => {
  vi.restoreAllMocks();
});

describe("isStaleChunkError", () => {
  it("조각을 가져오지 못한 오류를 알아본다", () => {
    for (const message of [
      "Failed to fetch dynamically imported module: /assets/PeoplePage-abc.js",
      "error loading dynamically imported module",
      "Importing a module script failed.",
      "ChunkLoadError: Loading chunk 42 failed.",
    ]) {
      expect(isStaleChunkError(new Error(message)), message).toBe(true);
    }
  });

  it("보통의 오류는 그렇게 보지 않는다", () => {
    expect(isStaleChunkError(new Error("undefined is not a function"))).toBe(
      false,
    );
    expect(isStaleChunkError(undefined)).toBe(false);
    expect(isStaleChunkError("문자열 오류")).toBe(false);
  });
});

describe("ErrorBoundary", () => {
  it("문제가 없으면 그대로 보여준다", () => {
    render(
      <ErrorBoundary>
        <p>정상 화면</p>
      </ErrorBoundary>,
    );
    expect(screen.getByText("정상 화면")).toBeInTheDocument();
  });

  it("렌더가 실패해도 흰 화면 대신 안내를 남긴다", () => {
    render(
      <ErrorBoundary>
        <Boom message="undefined is not a function" />
      </ErrorBoundary>,
    );
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.getByText("화면을 그리지 못했어요")).toBeInTheDocument();
    // 기록이 사라진 게 아니라는 것을 알려야 사용자가 겁먹지 않는다.
    expect(screen.getByText(/기록은 그대로 있습니다/)).toBeInTheDocument();
  });

  it("배포로 조각이 사라진 경우는 고장이 아니라 업데이트로 안내한다", () => {
    render(
      <ErrorBoundary>
        <Boom message="Failed to fetch dynamically imported module: /assets/x.js" />
      </ErrorBoundary>,
    );
    expect(screen.getByText("새 버전이 준비되었어요")).toBeInTheDocument();
    expect(screen.queryByText("화면을 그리지 못했어요")).toBeNull();
  });

  it("어느 경우든 빠져나갈 버튼을 준다", () => {
    render(
      <ErrorBoundary>
        <Boom message="아무 오류" />
      </ErrorBoundary>,
    );
    expect(screen.getByRole("button", { name: /새로고침/ })).toBeInTheDocument();
  });
});
