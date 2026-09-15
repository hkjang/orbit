-- 다른 서비스로 기억을 넘길 때 쓰는 표(claim).
--
-- 표 자체는 저장하지 않고 다이제스트만 둔다. 이 표를 읽어도 누구도 내밀 수
-- 있는 것을 얻지 못한다. 본문은 표를 발급하는 순간 만들어 함께 두므로, 표가
-- 알린 바이트 수가 곧 받아 가는 바이트 수다.
CREATE TABLE IF NOT EXISTS handoff_claims (
  claim_digest text PRIMARY KEY,
  memory_id uuid NOT NULL,
  issued_by uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  filename text NOT NULL,
  content_type text NOT NULL,
  body bytea NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS handoff_claims_expiry_idx ON handoff_claims(expires_at);

-- 보낼 곳 허용 목록. 기본은 비어 있고, 그때는 보내기 단추가 보이지 않는다.
INSERT INTO settings(namespace,key,value) VALUES('system','handoff','{"targets":[]}'::jsonb)
ON CONFLICT DO NOTHING;
