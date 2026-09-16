-- 사내 SMTP 릴레이로 보내는 이벤트 알림.
--
-- mail_deliveries 는 시도마다 한 줄이다. 성공과 실패를 모두 남겨야 "안 왔다" 는
-- 문의에 답할 수 있다. 본문은 담지 않는다 — 제목과 수신자면 충분하고, 본문까지
-- 담으면 알림 기록이 그 자체로 유출 경로가 된다.
CREATE TABLE IF NOT EXISTS mail_deliveries (
  id uuid PRIMARY KEY,
  event text NOT NULL,
  recipient text NOT NULL,
  subject text NOT NULL,
  actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
  resource_type text NOT NULL DEFAULT '',
  resource_id text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','sent','failed')),
  attempts integer NOT NULL DEFAULT 0,
  error_message text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS mail_deliveries_time_idx ON mail_deliveries(created_at DESC);
-- 같은 사람에게 같은 이벤트를 방금 보냈는지 보는 묶어 보내기 조회용.
CREATE INDEX IF NOT EXISTS mail_deliveries_recent_idx ON mail_deliveries(event, lower(recipient), created_at DESC);

-- 기본값은 꺼짐이다. 이미 운영 중인 곳은 Bootstrap 이 다시 돌지 않으므로
-- 여기서 행을 만들어 두어야 설정 화면이 열린다.
INSERT INTO settings(namespace, key, value)
VALUES ('mail', 'smtp', '{"enabled":false,"smtp_host":"","smtp_port":25,"security":"auto","skip_tls_verify":false,"username":"","from_address":"","from_name":"Orbit","base_url":"","timeout_seconds":10,"notify_approval_request":true,"notify_approval_decided":true,"notify_account_created":true}'::jsonb)
ON CONFLICT DO NOTHING;
