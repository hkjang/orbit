-- 방문 추적 설정 행을 만든다.
--
-- 관리 화면의 저장은 UPDATE 이므로 행이 있어야 한다. 이미 운영 중인 설치는
-- Bootstrap 의 기본값 목록을 다시 지나지 않으므로 여기서 한 번 넣는다.
-- 기본은 꺼짐이다 — 이 행이 생겨도 화면과 정책은 아무것도 달라지지 않는다.
INSERT INTO settings(namespace, key, value)
VALUES ('system', 'tracking', '{"enabled":false,"provider":"none","momento_url":"","momento_site_id":"","momento_proxy":true,"measurement_id":"","matomo_url":"","matomo_site_id":"","custom_snippet":"","allowed_hosts":"","include_admin":false,"placement":"head"}')
ON CONFLICT DO NOTHING;
