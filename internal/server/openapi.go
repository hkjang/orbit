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
			// /orbit 만 리터럴로 풀어 쓴다. operation() 은 요약과 권한만 받아
			// 질의 매개변수를 끼울 자리가 없는데, 그 함수를 고치면 나머지 아홉
			// 경로가 같이 흔들린다. 항목 하나를 펴 두는 쪽이 범위가 작다.
			"/orbit": map[string]any{"get": map[string]any{
				"summary":     "Orbit 그래프 조회",
				"description": "필요 API 키 권한: orbit:read. at 을 주면 그 시점 기준으로 다시 계산한 과거 우주를 돌려주며 응답에 historical=true 와 at 이 함께 붙는다. at 이 있든 없든 earliest_at 은 시간 여행이 가능한 가장 이른 시각이고, 기록이 하나도 없으면 null 이다.",
				"parameters": []map[string]any{{
					"name":        "at",
					"in":          "query",
					"required":    false,
					"description": "과거 시점. RFC3339(예: 2025-06-01T09:00:00Z) 또는 YYYY-MM-DD(UTC 자정으로 읽는다). 생략하면 현재 시점.",
					"schema":      map[string]string{"type": "string"},
				}},
				"responses": map[string]any{"200": map[string]string{"description": "성공"}, "400": map[string]string{"description": "잘못된 요청"}, "401": map[string]string{"description": "인증 필요"}, "403": map[string]string{"description": "권한 부족"}},
			}},
			"/people/":                          map[string]any{"get": operation("사람 검색 및 목록", "people:read"), "post": operation("사람 등록", "people:write")},
			"/people/{personID}":                map[string]any{"get": operation("관계 상세 조회", "people:read"), "put": operation("사람 및 관계 수정", "people:write"), "delete": operation("사람 삭제", "people:write")},
			"/people/{personID}/interactions":   map[string]any{"post": operation("교류 기록", "people:write")},
			"/personal/export":                  map[string]any{"get": operation("내 기록 전체 내보내기", "people:read")},
			"/people/{personID}/anchor":         map[string]any{"post": operation("관계 고정(Anchor) 설정", "people:write")},
			"/people/{personID}/links":          map[string]any{"get": operation("사람 간 연결 목록", "people:read"), "post": operation("사람 간 연결 추가/수정", "people:write")},
			"/people/{personID}/links/{linkID}": map[string]any{"delete": operation("사람 간 연결 삭제", "people:write")},
			"/memories/":                        map[string]any{"get": operation("기억 목록", "memories:read"), "post": operation("기억 생성", "memories:write")},
			"/ai/stream":                        map[string]any{"post": operation("SSE 기반 AI 관계 질문", "ai:invoke")},
		},
		"externalDocs": map[string]string{"description": "MCP Streamable HTTP endpoint", "url": "/mcp"},
	}
	writeJSON(w, 200, doc)
}
func operation(summary, scope string) map[string]any {
	return map[string]any{"summary": summary, "description": "필요 API 키 권한: " + scope, "responses": map[string]any{"200": map[string]string{"description": "성공"}, "400": map[string]string{"description": "잘못된 요청"}, "401": map[string]string{"description": "인증 필요"}, "403": map[string]string{"description": "권한 부족"}}}
}
