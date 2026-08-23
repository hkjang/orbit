import { Chip } from "@mui/material";
import { readGrammar } from "../orbitGrammar";
import type { OrbitNode } from "../types";

/**
 * 관계 상태 배지. 캔버스의 궤도 문법을 목록·상세 화면에서도 같은 말로 반복합니다.
 * 점수를 노출하지 않고 상태 이름만 보여줍니다.
 */
export function StateChip({
  person,
  hideStable,
}: {
  person: Pick<OrbitNode, "momentum" | "last_interaction_at">;
  hideStable?: boolean;
}) {
  const grammar = readGrammar(person);
  if (hideStable && grammar.state === "stable") return null;
  return (
    <Chip
      size="small"
      variant="outlined"
      label={grammar.label}
      title={grammar.hint}
      sx={{ color: grammar.tone, borderColor: grammar.tone, opacity: 0.9 }}
    />
  );
}
