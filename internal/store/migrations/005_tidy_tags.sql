-- 소속(relationships.categories)과 주제(memories.topics)를 한 번 정리한다.
--
-- 저장 직전 정규화가 들어가기 전에 쌓인 값에는 앞뒤 공백, 빈 문자열, 중복이
-- 남아 있다. 그대로 두면 "가족"과 "가족 "이 서로 다른 소속으로 남아 화면에서는
-- 같아 보이는데 색과 묶음이 갈린다.
--
-- 적은 순서는 지킨다. 소속 배열의 첫 항목이 행성 테두리 색을 정하므로,
-- 순서가 바뀌면 사람들의 색이 통째로 달라진다.
UPDATE relationships r SET categories = coalesce((
  SELECT jsonb_agg(d.value ORDER BY d.ord)
  FROM (
    SELECT trim(t.value) AS value, min(t.ord) AS ord
    FROM jsonb_array_elements_text(r.categories) WITH ORDINALITY AS t(value, ord)
    WHERE trim(t.value) <> ''
    GROUP BY trim(t.value)
  ) d
), '[]'::jsonb)
WHERE jsonb_typeof(r.categories) = 'array';

UPDATE memories m SET topics = coalesce((
  SELECT jsonb_agg(d.value ORDER BY d.ord)
  FROM (
    SELECT trim(t.value) AS value, min(t.ord) AS ord
    FROM jsonb_array_elements_text(m.topics) WITH ORDINALITY AS t(value, ord)
    WHERE trim(t.value) <> ''
    GROUP BY trim(t.value)
  ) d
), '[]'::jsonb)
WHERE jsonb_typeof(m.topics) = 'array';
