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

## Handing a memory to another service

Orbit is the *sending* side of the in-house handoff standard
(`HANDOFF-STANDARD.md`) and sends `markdown` only. The endpoint names, request
shape and status codes are the standard's and are shared by all six services.

| Endpoint | Auth | Purpose |
|---|---|---|
| `GET /api/v1/handoff/targets` | session or `memories:read` | Services an administrator has allow-listed that accept markdown, plus the `source` origin the receiver will fetch from. Empty until configured. |
| `POST /api/v1/handoff/claims` `{"resource": "<memory id>", "format": "markdown"}` | session or `memories:read` | `201` with `claim`, `source`, `filename`, `content_type`, `bytes`, `expires_at`. Bound to one approved memory the caller owns; five minutes; single use. `404` for someone else's memory, `409 not_approved` while a memory is pending review. |
| `GET /api/v1/handoff/claims/{claim}` | none — the claim is the credential | `200 text/markdown` once. Spent, expired and unknown claims all answer `404` alike. |

The browser opens `<target origin>/handoff?source=<orbit origin>&claim=<claim>`
in a new window; the receiving service collects the document from Orbit. See
[the administrator guide](ADMIN_GUIDE.md#다른-서비스로-보내기) for the allow-list
(`PUT /api/v1/admin/settings/handoff`).
