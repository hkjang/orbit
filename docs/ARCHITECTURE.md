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

Local password login is throttled in memory: ten failures for the same
username from the same client address within 15 minutes lock that pair for
15 minutes (`429 too_many_attempts` with `Retry-After`). The key combines
username and address so a stranger cannot lock a real account by guessing its
name, and neighbours behind one proxy do not lock each other. The moment a lock
starts is written to the audit log as `auth.login_blocked`.

Key rotation is transactional: Orbit creates a new data key, re-encrypts all
current protected fields and retires the old key in one database transaction.

## Tests

`go test ./...` runs without a database. Tests that need real SQL (for
example `internal/server/timetravel_db_test.go`) skip unless
`ORBIT_TEST_DATABASE_URL` points at a PostgreSQL the test may migrate and
write to; the file header shows a one-line `docker run` that provides one.

## Web dependency install

`scripts/npm-install.sh` wraps `npm ci` in a bounded retry (3 attempts) and is
what the Dockerfile and `make web` call. npm's own `fetch-retries` cannot cover
a registry socket drop that happens while the response body is being read —
that error is thrown outside make-fetch-happen's retry wrapper — so the command
itself has to be re-run. The wrapper never falls back to `npm install`, so the
lockfile still decides the dependency tree, and a genuine failure still fails
after the attempt limit. `scripts/npm_install_test.go` pins this down by putting
a fake `npm` on `PATH`.

## Relationship visual grammar

- Planet size: long-term importance
- Distance: current interaction-derived closeness
- Brightness: current activity
- Green arc: positive momentum
- Stable angle: deterministic spatial memory
- Context color: overlapping relationship context

Numbers are internal signals and are translated to humane phrases in the UI.

