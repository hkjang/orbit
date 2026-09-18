-- MCP 를 Keycloak 액세스 토큰(OAuth)으로도 열 수 있게 하는 설정 행. 기본은 꺼짐이라
-- 이미 돌고 있는 설치에서는 아무것도 달라지지 않는다.
INSERT INTO settings(namespace, key, value)
VALUES ('mcp', 'oauth', '{"enabled":false,"resource":"","audience":[],"scopes":["people:read","memories:read"]}'::jsonb)
ON CONFLICT DO NOTHING;
