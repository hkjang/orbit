import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { formatDate } from "../api";
import { TimeTravel } from "./TimeTravel";

const DAY = 86_400_000;
const NOW = Date.parse("2026-09-19T12:00:00Z");
// 슬라이더 100칸: 100px 폭이면 1px 이 하루다.
const EARLIEST = new Date(NOW - 100 * DAY).toISOString();

/**
 * MUI 공식 테스트처럼 jsdom 에 없는 것을 흉내 낸다: 배치 정보와 포인터 캡처.
 * (jsdom 은 `hasPointerCapture` 가 없어 손을 떼는 순간 MUI 가 던진다.)
 */
function sliderRoot() {
  const input = screen.getByLabelText("돌아볼 시점");
  const root = input.closest(".MuiSlider-root") as HTMLElement;
  root.getBoundingClientRect = () =>
    ({
      width: 100,
      height: 10,
      bottom: 10,
      left: 0,
      top: 0,
      right: 100,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    }) as DOMRect;
  root.setPointerCapture = () => {};
  root.hasPointerCapture = () => false;
  root.releasePointerCapture = () => {};
  return root;
}

// MUI 는 pointermove 에서 buttons 가 0 이면 손을 뗀 것으로 보므로 누른 상태를 실어 보낸다.
const pressed = { clientY: 5, button: 0, buttons: 1, pointerId: 1 };

function dragTo(root: HTMLElement, from: number, to: number) {
  fireEvent.pointerDown(root, { ...pressed, clientX: from });
  fireEvent.pointerMove(document, { ...pressed, clientX: to });
}

function release(at: number) {
  fireEvent.pointerUp(document, { ...pressed, buttons: 0, clientX: at });
}

describe("TimeTravel", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(NOW);
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("does not ask for a new universe while the thumb is being dragged", () => {
    const onChange = vi.fn();
    render(<TimeTravel earliest={EARLIEST} onChange={onChange} />);
    const root = sliderRoot();
    dragTo(root, 100, 60);
    fireEvent.pointerMove(document, { ...pressed, clientX: 40 });
    expect(onChange).not.toHaveBeenCalled();
  });

  it("shows the day under the thumb while dragging", () => {
    render(<TimeTravel earliest={EARLIEST} onChange={() => {}} />);
    const root = sliderRoot();
    dragTo(root, 100, 40);
    // 40칸 = earliest 에서 40일 뒤. 슬라이더의 값 라벨도 같은 글자를 쓰므로 제목(p)만 본다.
    const title = screen.getByText("TIME TRAVEL").nextElementSibling;
    expect(title).toHaveTextContent(
      formatDate(new Date(Date.parse(EARLIEST) + 40 * DAY).toISOString()),
    );
    expect(screen.queryByText("지금의 우주")).toBeNull();
  });

  it("asks once, with the last day dragged to, when the thumb is let go", () => {
    const onChange = vi.fn();
    render(<TimeTravel earliest={EARLIEST} onChange={onChange} />);
    const root = sliderRoot();
    dragTo(root, 100, 60);
    fireEvent.pointerMove(document, { ...pressed, clientX: 40 });
    release(40);
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith(
      new Date(Date.parse(EARLIEST) + 40 * DAY).toISOString(),
    );
  });

  it("returns to the present when released on the last notch", () => {
    const onChange = vi.fn();
    render(
      <TimeTravel
        earliest={EARLIEST}
        value={new Date(NOW - 30 * DAY).toISOString()}
        onChange={onChange}
      />,
    );
    const root = sliderRoot();
    dragTo(root, 70, 100);
    release(100);
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith(undefined);
  });

  it("follows the parent back to the present after the '현재로' button", () => {
    const onChange = vi.fn();
    const past = new Date(NOW - 30 * DAY).toISOString();
    const { rerender } = render(
      <TimeTravel earliest={EARLIEST} value={past} onChange={onChange} />,
    );
    const root = sliderRoot();
    // 드래그하다가 손을 떼지 않은 채 부모가 '현재로' 를 눌러 값을 지운 경우.
    dragTo(root, 70, 20);
    rerender(
      <TimeTravel earliest={EARLIEST} value={undefined} onChange={onChange} />,
    );
    expect(screen.getByText("지금의 우주")).toBeInTheDocument();
    expect(screen.getByLabelText("돌아볼 시점")).toHaveValue("100");
  });
});
