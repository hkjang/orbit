# API and MCP

Create a personal key from **Personalization → API · MCP keys**. Send it as:

```http
Authorization: Bearer orb_...
```

The live OpenAPI 3.1 document is available at `/openapi.json`. API keys are
scope-limited and can be revoked without changing the user's encryption key.

The MCP Streamable HTTP endpoint is `/mcp`. Orbit is a dual-era server: it
supports modern MCP `2026-07-28` per-request metadata and `server/discover`,
plus legacy `2025-11-25` and `2025-06-18` initialization. An MCP key needs `mcp:use` plus the
data scope required by each tool:

| Tool | Additional scope |
|---|---|
| `orbit_search_people` | `people:read` |
| `orbit_get_relationship` | `people:read` |
| `orbit_list_memories` | `memories:read` |
| `orbit_create_memory` | `memories:write` |

Memory creation through REST or MCP enters `pending` only when the administrator
has enabled the approval workflow. Otherwise the review process is omitted.

## 키 없이 SSO 로 연결하기

관리자가 **MCP SSO(OAuth)** 를 켠 설치에서는 개인 키를 만들지 않아도 된다. MCP
클라이언트(Claude, Cursor 등)에 `https://<서비스 주소>/mcp` **URL 하나만** 넣으면
클라이언트가 `/.well-known/oauth-protected-resource/mcp` 를 읽어 Keycloak 을 찾고,
로그인 창을 띄운 뒤 액세스 토큰을 받아 `Authorization: Bearer <토큰>` 으로 보낸다.
이미 Keycloak 에 로그인돼 있으면 창은 거의 보이지 않는다.

- 이 화면(웹)에 한 번 로그인한 계정이어야 한다. 토큰으로 계정이 만들어지지는 않는다.
- 권한은 관리자가 정한 범위(기본 `people:read memories:read`)를 따른다. 쓰기 도구가
  필요하면 키를 쓰거나 관리자에게 범위를 요청한다.
- 토큰은 `/mcp` 에서만 통한다. REST 는 여전히 키 또는 세션이다.
- 켜져 있는지는 **개인화 → API · MCP 키** 의 "MCP 연결" 카드에 표시된다.

관리자 쪽 설정과 Keycloak 클라이언트 구성은 [관리자 가이드](ADMIN_GUIDE.md#mcp-ssooauth--개인-키-없이-keycloak-토큰으로-mcp-열기)를 본다.
