# Orbit architecture

Orbit is a modular Go monolith with an embedded React application. A single
service image serves the REST API, MCP endpoint, OIDC flow, AI stream proxy and
static UI. PostgreSQL is the only runtime dependency.

## Runtime boundaries

```text
Browser / MCP client
        │
        ▼
Orbit :8080
 ├─ local session + dynamic Keycloak OIDC
 ├─ relationship / memory domain
 ├─ user key vault + API key scopes
 ├─ approval workflow (optional)
 ├─ OpenAI Responses-compatible SSE gateway
 ├─ REST / OpenAPI
 └─ Streamable HTTP MCP
        │
        ▼
PostgreSQL
```

All mutable service settings live in the `settings` table. Client secrets and
AI API keys are encrypted with the master encryption key. Contact fields,
interaction summaries and memory content are encrypted using versioned,
per-user data encryption keys. User keys are wrapped by the master key.

Key rotation is transactional: Orbit creates a new data key, re-encrypts all
current protected fields and retires the old key in one database transaction.

## Relationship visual grammar

- Planet size: long-term importance
- Distance: current interaction-derived closeness
- Brightness: current activity
- Green arc: positive momentum
- Stable angle: deterministic spatial memory
- Context color: overlapping relationship context

Numbers are internal signals and are translated to humane phrases in the UI.

