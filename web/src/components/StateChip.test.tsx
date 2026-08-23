import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StateChip } from "./StateChip";

const daysAgo = (days: number) =>
  new Date(Date.now() - days * 86_400_000).toISOString();

describe("StateChip", () => {
  it("names the state a person is in", () => {
    render(<StateChip person={{ momentum: 0.5, last_interaction_at: daysAgo(2) }} />);
    expect(screen.getByText("다가오는 중")).toBeInTheDocument();
  });

  it("does not call an anchored relationship dark orbit", () => {
    // 고정된 관계는 오래 침묵해도 바깥으로 밀려나지 않는다.
    render(
      <StateChip
        person={{ momentum: 0, last_interaction_at: daysAgo(400), anchored: true }}
      />,
    );
    expect(screen.queryByText("다크 오빗")).toBeNull();
    expect(screen.getByText("안정 궤도")).toBeInTheDocument();
  });

  it("stays quiet for a stable orbit when asked to", () => {
    const { container } = render(
      <StateChip person={{ momentum: 0, last_interaction_at: daysAgo(5) }} hideStable />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
