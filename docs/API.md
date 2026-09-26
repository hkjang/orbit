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
