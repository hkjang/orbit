# API and MCP

For endpoints that accept API keys, create a personal key from
**Personalization → API · MCP keys**. Send it as:

```http
Authorization: Bearer orb_...
```

The live OpenAPI 3.1 document is available at `/openapi.json`. API keys are
scope-limited and can be revoked without changing the user's encryption key.
The [full data export](#full-data-export) is a session-only exception: it requires
the logged-in browser's session cookie and cannot be called with an API key.

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

## Full data export

`GET /api/v1/personal/export` exports the logged-in user's records as readable
JSON. Use the export link in **Personalization** in a logged-in browser; the
browser sends the `orbit_session` cookie. This endpoint is session-only and does
not accept Bearer API keys, regardless of their scopes.

The response uses `Content-Type: application/json; charset=utf-8` and
`Content-Disposition: attachment; filename="orbit-export-YYYY-MM-DD.json"`.
The filename contains the export date. The top-level fields are:

| Field | Meaning |
|---|---|
| `exported_at` | Export timestamp in RFC3339 format. |
| `service_version` | Orbit server version that produced the export. |
| `user` | Account metadata: `id`, `username`, `email`, `display_name`, `role`, and `created_at`. |
| `people` | People and their relationship data. |
| `interactions` | Recorded interactions, linked to people by `person_id`. |
| `memories` | Recorded memories, optionally linked to people by `person_id`. |
| `links` | Connections between people. |
| `complete` | `true` only after all four arrays have been written. |
| `failed_section` | Present on a section processing failure; one of `people`, `interactions`, `memories`, or `links`. |

**HTTP 200 alone does not mean the backup is complete.** Require both successful
JSON parsing and `complete === true`. A normal export ends with `"complete": true`.
The arrays are written in this order: `people`, `interactions`, `memories`, `links`.
If section processing fails, the response still has HTTP 200 but ends with
`"complete": false` and `failed_section`. The failed array remains partial or
empty, and later section keys are absent. Network or write failures can truncate
the response: neither valid JSON nor a failure marker is guaranteed. Treat a
parse failure or a missing or false `complete` value as an incomplete backup.

Successful export with empty arrays (fictional account):

```json
{
  "exported_at": "2026-10-08T15:00:00+09:00",
  "service_version": "0.7.12",
  "user": {
    "id": "00000000-0000-4000-8000-000000000001",
    "username": "example-user",
    "email": "example@example.com",
    "display_name": "Example User",
    "role": "user",
    "created_at": "2026-01-01T00:00:00Z"
  },
  "people": [],
  "interactions": [],
  "memories": [],
  "links": [],
  "complete": true
}
```

Failure while processing `people`, before any people were written:

```json
{
  "exported_at": "2026-10-08T15:00:00+09:00",
  "service_version": "0.7.12",
  "user": {
    "id": "00000000-0000-4000-8000-000000000001",
    "username": "example-user",
    "email": "example@example.com",
    "display_name": "Example User",
    "role": "user",
    "created_at": "2026-01-01T00:00:00Z"
  },
  "people": [],
  "complete": false,
  "failed_section": "people"
}
```

## Time Travel — `GET /orbit?at=`

`GET /api/v1/orbit` accepts an optional `at` query parameter and rebuilds the
graph as it stood at that moment. Two formats are accepted:

| Value | Meaning |
|---|---|
| `2025-06-01T09:00:00Z` | RFC3339 instant, used as given |
| `2025-06-01` | date only — read as **midnight UTC** of that day |

Anything else returns `400 validation_error`. Omitting `at` returns the current
graph.

The response carries three fields that describe the point in time:

| Field | Present | Meaning |
|---|---|---|
| `earliest_at` | always | Oldest record you can travel back to (first `people.created_at` or `interactions.occurred_at`). `null` when the user has no records at all. |
| `at` | only with `?at=` | The instant the graph was rebuilt for, echoed back. |
| `historical` | only with `?at=` | Always `true`, so a client can tell a past view from the present one. |

Every other key (`center`, `nodes`, `contexts`, `links`, `categories`,
`generated_at`) is the same in both cases. Use `earliest_at` as the lower bound
of any time slider or range picker — there is nothing to show before it.

What actually travels: closeness, momentum, `last_interaction_at` and
`memory_count` are recomputed from the interactions and memories recorded up to
`at`. Importance, categories/label and the anchored flag are set by hand and
have no change history, so the **current** values are used for those. What comes
back is the distance and drift that interactions created, not a full snapshot.

`links` follows the same rule — person-to-person links are set by hand, so the
current ones are used. They are, however, restricted to the nodes in the same
response: a link is only returned when **both** endpoints appear in `nodes`, so
`links[*].a` and `links[*].b` never point outside it and the graph in a `?at=`
response is closed on its own.
