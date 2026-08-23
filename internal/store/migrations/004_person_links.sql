-- 사람↔사람 관계(Relationship Graph).
-- 지금까지 Orbit은 나→사람 관계만 알고 있었습니다. 이 표가 열리면
-- Introduction Path(Gravity Assist)와 그룹 동시 변화(Eclipse) 분석이 가능해집니다.
--
-- 방향 없는 간선으로만 저장합니다(person_a < person_b로 정규화).
-- 자유 텍스트 메모는 두지 않습니다 — 키 회전 루틴이 암호화 컬럼을 명시적으로
-- 열거하므로, 새 암호문 컬럼은 회전에서 누락될 위험이 있습니다.
CREATE TABLE IF NOT EXISTS person_links (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  person_a uuid NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  person_b uuid NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  kind text NOT NULL DEFAULT 'knows' CHECK (kind IN ('colleague','family','friend','community','knows')),
  strength numeric(5,4) NOT NULL DEFAULT .5 CHECK (strength >= 0 AND strength <= 1),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (person_a <> person_b),
  CHECK (person_a < person_b),
  UNIQUE (user_id, person_a, person_b)
);
CREATE INDEX IF NOT EXISTS person_links_a_idx ON person_links(user_id, person_a);
CREATE INDEX IF NOT EXISTS person_links_b_idx ON person_links(user_id, person_b);
