-- Orbit 캔버스의 Memory Nebula는 인물별 승인된 기억 수를 매 조회마다 셉니다.
-- 기존 memories 인덱스는 (user_id, occurred_at) 기준이라 인물별 집계에는 쓰이지 못합니다.
CREATE INDEX IF NOT EXISTS memories_person_status_idx ON memories(user_id, person_id, status);
