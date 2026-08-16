package server

import "net/http"

func (s *Server) openAPI(w http.ResponseWriter, _ *http.Request) {
	doc := map[string]any{
		"openapi":    "3.1.0",
		"info":       map[string]string{"title": "Orbit API", "version": s.version, "description": "개인 관계 우주, 기억, 키 및 AI를 위한 REST API. 외부 호출은 개인 API 키를 Bearer 토큰으로 사용합니다."},
		"servers":    []map[string]string{{"url": "/api/v1"}},
		"components": map[string]any{"securitySchemes": map[string]any{"bearerAuth": map[string]string{"type": "http", "scheme": "bearer"}}},
		"security":   []map[string]any{{"bearerAuth": []string{}}},
		"paths": map[string]any{
			"/orbit":                          map[string]any{"get": operation("Orbit 그래프 조회", "orbit:read")},
			"/people/":                        map[string]any{"get": operation("사람 검색 및 목록", "people:read"), "post": operation("사람 등록", "people:write")},
			"/people/{personID}":              map[string]any{"get": operation("관계 상세 조회", "people:read"), "put": operation("사람 및 관계 수정", "people:write"), "delete": operation("사람 삭제", "people:write")},
			"/people/{personID}/interactions": map[string]any{"post": operation("교류 기록", "people:write")},
			"/memories/":                      map[string]any{"get": operation("기억 목록", "memories:read"), "post": operation("기억 생성", "memories:write")},
			"/ai/stream":                      map[string]any{"post": operation("SSE 기반 AI 관계 질문", "ai:invoke")},
		},
		"externalDocs": map[string]string{"description": "MCP Streamable HTTP endpoint", "url": "/mcp"},
	}
	writeJSON(w, 200, doc)
}
func operation(summary, scope string) map[string]any {
	return map[string]any{"summary": summary, "description": "필요 API 키 권한: " + scope, "responses": map[string]any{"200": map[string]string{"description": "성공"}, "400": map[string]string{"description": "잘못된 요청"}, "401": map[string]string{"description": "인증 필요"}, "403": map[string]string{"description": "권한 부족"}}}
}
