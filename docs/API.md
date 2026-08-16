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
