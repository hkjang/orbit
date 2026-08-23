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
  // anchored를 빠뜨리면 타입상으로는 통과하지만(선택 필드), 고정된 관계를
  // 다크 오빗으로 잘못 표시하게 된다. readGrammar가 쓰는 값은 모두 받는다.
  person: Pick<OrbitNode, "momentum" | "last_interaction_at" | "anchored">;
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
