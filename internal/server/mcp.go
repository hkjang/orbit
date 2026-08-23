package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/hkjang/orbit/internal/id"
	"github.com/jackc/pgx/v5"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

var supportedMCPVersions = []string{"2026-07-28", "2025-11-25", "2025-06-18"}

func (s *Server) mcp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, 405, "method_not_allowed", "MCP는 POST 요청만 지원합니다.")
		return
	}
	var req rpcRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.JSONRPC != "2.0" {
		s.rpcWrite(w, req.ID, nil, &rpcError{Code: -32600, Message: "Invalid Request"})
		return
	}
	requestedVersion := mcpRequestedVersion(r, req)
	if requestedVersion != "" && !mcpVersionSupported(requestedVersion) {
		writeJSON(w, http.StatusBadRequest, rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32022, Message: "Unsupported protocol version", Data: map[string]any{"supported": supportedMCPVersions, "requested": requestedVersion}}})
		return
	}
	if req.Method == "notifications/initialized" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	switch req.Method {
	case "server/discover":
		s.rpcWrite(w, req.ID, map[string]any{
			"resultType":        "complete",
			"supportedVersions": supportedMCPVersions,
			"capabilities":      map[string]any{"tools": map[string]any{}},
			"_meta":             map[string]any{"io.modelcontextprotocol/serverInfo": map[string]string{"name": "orbit", "version": s.version}},
			"instructions":      "Orbit의 개인 관계와 기억을 근거 중심으로 조회하고 기록합니다.",
			"ttlMs":             3600000,
			"cacheScope":        "private",
		}, nil)
	case "initialize":
		if requestedVersion == "" {
			requestedVersion = "2025-06-18"
		}
		s.rpcWrite(w, req.ID, map[string]any{"protocolVersion": requestedVersion, "capabilities": map[string]any{"tools": map[string]any{"listChanged": false}}, "serverInfo": map[string]string{"name": "orbit", "version": s.version}, "instructions": "Orbit의 개인 관계와 기억을 근거 중심으로 조회하고 기록합니다."}, nil)
	case "ping":
		s.rpcWrite(w, req.ID, map[string]any{}, nil)
	case "tools/list":
		s.rpcWrite(w, req.ID, map[string]any{"tools": mcpTools()}, nil)
	case "tools/call":
		s.mcpCall(w, r, req)
	default:
		s.rpcWrite(w, req.ID, nil, &rpcError{Code: -32601, Message: "Method not found"})
	}
}

func mcpRequestedVersion(r *http.Request, req rpcRequest) string {
	if header := strings.TrimSpace(r.Header.Get("MCP-Protocol-Version")); header != "" {
		return header
	}
	var params map[string]any
	if json.Unmarshal(req.Params, &params) != nil {
		return ""
	}
	if req.Method == "initialize" {
		value, _ := params["protocolVersion"].(string)
		return value
	}
	meta, _ := params["_meta"].(map[string]any)
	value, _ := meta["io.modelcontextprotocol/protocolVersion"].(string)
	return value
}

func mcpVersionSupported(version string) bool {
	for _, supported := range supportedMCPVersions {
		if version == supported {
			return true
		}
	}
	return false
}
func (s *Server) rpcWrite(w http.ResponseWriter, idValue, result any, rpcErr *rpcError) {
	writeJSON(w, 200, rpcResponse{JSONRPC: "2.0", ID: idValue, Result: result, Error: rpcErr})
}
func mcpTools() []map[string]any {
	return []map[string]any{
		{"name": "orbit_search_people", "description": "이름 또는 회사로 내 Orbit의 사람을 검색합니다.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]string{"type": "string", "description": "검색어"}}, "required": []string{"query"}}},
		{"name": "orbit_get_relationship", "description": "특정 사람과의 관계, 궤도 상태, 이어진 사람들, 승인된 기억을 조회합니다.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"person_id": map[string]string{"type": "string", "format": "uuid"}}, "required": []string{"person_id"}}},
		{"name": "orbit_list_memories", "description": "내 관계 기억을 최신순으로 조회합니다.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"person_id": map[string]string{"type": "string"}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}}}},
		{"name": "orbit_create_memory", "description": "새 관계 기억을 기록합니다. 관리자 설정에 따라 팀장 승인 대기가 될 수 있습니다.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"person_id": map[string]string{"type": "string"}, "title": map[string]string{"type": "string"}, "content": map[string]string{"type": "string"}, "topics": map[string]any{"type": "array", "items": map[string]string{"type": "string"}}}, "required": []string{"title", "content"}}},
	}
}

func (s *Server) mcpCall(w http.ResponseWriter, r *http.Request, req rpcRequest) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal(req.Params, &call) != nil {
		s.rpcWrite(w, req.ID, nil, &rpcError{Code: -32602, Message: "Invalid params"})
		return
	}
	u := userFromContext(r.Context())
	requiredScopes := map[string]string{"orbit_search_people": "people:read", "orbit_get_relationship": "people:read", "orbit_list_memories": "memories:read", "orbit_create_memory": "memories:write"}
	if scope := requiredScopes[call.Name]; scope != "" && !requestHasScope(r, scope) {
		s.rpcWrite(w, req.ID, map[string]any{"isError": true, "content": []map[string]string{{"type": "text", "text": "이 MCP 도구에 필요한 API 키 권한이 없습니다: " + scope}}}, nil)
		return
	}
	var result any
	var err error
	switch call.Name {
	case "orbit_search_people":
		var args struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(call.Arguments, &args)
		rows, e := s.store.DB.Query(r.Context(), `SELECT p.id,p.display_name,p.company,p.role_title,r.relationship_label,r.last_interaction_at FROM people p JOIN relationships r ON r.person_id=p.id WHERE p.user_id=$1 AND (p.display_name ILIKE '%'||$2||'%' OR p.company ILIKE '%'||$2||'%') ORDER BY r.importance DESC LIMIT 30`, u.ID, strings.TrimSpace(args.Query))
		if e != nil {
			err = e
			break
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var personID, name, company, role, label string
			var last *time.Time
			if e = rows.Scan(&personID, &name, &company, &role, &label, &last); e != nil {
				err = e
				break
			}
			items = append(items, map[string]any{"id": personID, "name": name, "company": company, "role": role, "relationship": label, "last_interaction_at": last})
		}
		result = items
	case "orbit_get_relationship":
		var args struct {
			PersonID string `json:"person_id"`
		}
		_ = json.Unmarshal(call.Arguments, &args)
		var name, label string
		var importance, closeness, momentum float64
		var last *time.Time
		var anchored bool
		e := s.store.DB.QueryRow(r.Context(), `SELECT p.display_name,r.relationship_label,r.importance,r.closeness,r.momentum,r.last_interaction_at,r.anchored FROM people p JOIN relationships r ON r.person_id=p.id WHERE p.user_id=$1 AND p.id=$2`, u.ID, args.PersonID).Scan(&name, &label, &importance, &closeness, &momentum, &last, &anchored)
		if e != nil {
			err = e
			break
		}
		memories, e := s.queryMemories(r.Context(), u.ID, args.PersonID, "approved")
		if e != nil {
			err = e
			break
		}
		if len(memories) > 30 {
			memories = memories[:30]
		}
		connections, e := s.personConnections(r.Context(), u.ID, args.PersonID)
		if e != nil {
			err = e
			break
		}
		// 원시 수치는 그대로 둔다. MCP는 기계가 읽는 표면이라 모델 값이 쓸모 있다.
		// 다만 사람에게 그대로 옮겨도 되는 말(state_label)을 함께 실어, 외부
		// 에이전트가 점수를 되읊는 대신 상태로 말할 수 있게 한다.
		state := ReadGrammar(momentum, last, anchored, time.Now())
		result = map[string]any{"person_id": args.PersonID, "name": name, "relationship": label, "importance": importance, "closeness": closeness, "momentum": momentum, "last_interaction_at": last, "state": string(state), "state_label": StateLabel[state], "state_hint": StateHint[state], "anchored": anchored, "connections": connections, "memories": memories}
	case "orbit_list_memories":
		var args struct {
			PersonID string `json:"person_id"`
			Limit    int    `json:"limit"`
		}
		_ = json.Unmarshal(call.Arguments, &args)
		if args.Limit <= 0 || args.Limit > 100 {
			args.Limit = 30
		}
		memories, e := s.queryMemories(r.Context(), u.ID, args.PersonID, "approved")
		if e != nil {
			err = e
			break
		}
		if len(memories) > args.Limit {
			memories = memories[:args.Limit]
		}
		result = memories
	case "orbit_create_memory":
		var args struct {
			PersonID string   `json:"person_id"`
			Title    string   `json:"title"`
			Content  string   `json:"content"`
			Topics   []string `json:"topics"`
		}
		_ = json.Unmarshal(call.Arguments, &args)
		result, err = s.mcpCreateMemory(r, u, args.PersonID, args.Title, args.Content, args.Topics)
	default:
		s.rpcWrite(w, req.ID, map[string]any{"isError": true, "content": []map[string]string{{"type": "text", "text": "알 수 없는 도구입니다."}}}, nil)
		return
	}
	if err != nil {
		message := "요청을 처리하지 못했습니다."
		if errors.Is(err, pgx.ErrNoRows) {
			message = "대상을 찾을 수 없습니다."
		}
		s.rpcWrite(w, req.ID, map[string]any{"isError": true, "content": []map[string]string{{"type": "text", "text": message}}}, nil)
		return
	}
	raw, _ := json.Marshal(result)
	s.rpcWrite(w, req.ID, map[string]any{"content": []map[string]string{{"type": "text", "text": string(raw)}}, "structuredContent": result}, nil)
}

func requestHasScope(r *http.Request, scope string) bool {
	info, _ := r.Context().Value(authContextKey).(authInfo)
	return !info.APIKey || info.Scopes[scope]
}

func (s *Server) mcpCreateMemory(r *http.Request, u User, personID, title, content string, topics []string) (map[string]any, error) {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" || content == "" {
		return nil, errors.New("title and content required")
	}
	if personID != "" {
		var exists bool
		if err := s.store.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM people WHERE id=$1 AND user_id=$2)`, personID, u.ID).Scan(&exists); err != nil || !exists {
			return nil, pgx.ErrNoRows
		}
	}
	key, version, err := s.activeDataKey(r.Context(), u.ID)
	if err != nil {
		return nil, err
	}
	memoryID := id.New()
	cipher, err := s.store.Vault.Encrypt(key, content, "memory:"+memoryID+":content")
	if err != nil {
		return nil, err
	}
	settings, err := s.approvalSettings(r.Context())
	if err != nil {
		return nil, err
	}
	status := "approved"
	approval := settings.Enabled && contains(settings.ResourceTypes, "memory")
	if approval {
		status = "pending"
	}
	raw, _ := json.Marshal(topics)
	var person any = nil
	if personID != "" {
		person = personID
	}
	tx, err := s.store.DB.Begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `INSERT INTO memories(id,user_id,person_id,title,content_cipher,key_version,source_type,topics,status) VALUES($1,$2,$3,$4,$5,$6,'mcp',$7::jsonb,$8)`, memoryID, u.ID, person, title, cipher, version, string(raw), status); err != nil {
		return nil, err
	}
	if approval {
		if _, err = tx.Exec(r.Context(), `INSERT INTO approval_requests(id,requester_id,resource_type,resource_id,action,request_note) VALUES($1,$2,'memory',$3,'create','MCP에서 생성')`, id.New(), u.ID, memoryID); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		return nil, err
	}
	s.audit(r.Context(), u.ID, "memory.create", "memory", memoryID, r.RemoteAddr, map[string]any{"source": "mcp", "status": status})
	return map[string]any{"id": memoryID, "status": status, "approval_required": approval}, nil
}
