# 관리자 가이드

서비스 관리 콘솔(`/admin`)에서 다루는 설정 가운데 동작을 알고 써야 하는 것을
적는다. 화면 구성은 [사용자 가이드 §2.9](guide.md#29-서비스-관리-콘솔-admin),
배포와 환경 변수는 [오프라인 배포](OFFLINE.md)를 본다.

## MCP SSO(OAuth) — 개인 키 없이 Keycloak 토큰으로 `/mcp` 열기

`/mcp` 는 기본적으로 개인 API 키(`orb_…`)로만 들어온다. **Keycloak SSO** 탭의
"MCP SSO(OAuth) 연결" 카드를 켜면 Keycloak 이 발급한 **액세스 토큰**으로도 들어올 수
있다. MCP 인가 규격(2025-06-18 이후)은 OAuth 2.1 이라, 클라이언트(Claude·Cursor 등)에
`/mcp` 주소 하나만 주면 클라이언트가 스스로 Keycloak 로그인 창을 띄우고 토큰을 받아
온다. 키 체계는 그대로 남는다 — 폐쇄망·자동화 스크립트는 여전히 키를 쓴다.

Orbit 은 여기서 **리소스 서버**다. 로그인·토큰 발급·클라이언트 등록은 전부 Keycloak
이 하고, Orbit 은 (1) 인증 서버가 어디인지 알리는 메타데이터를 내고, (2) `/mcp` 의 401 에
그 주소를 붙이고, (3) 내민 토큰이 이 서버를 위한 것인지 검사한다. `/authorize`·`/token`
같은 끝점은 만들지 않는다.

### 설정 표

| 키 | 기본값 | 뜻 |
| --- | --- | --- |
| `mcp.oauth.enabled` | **꺼짐** | `/mcp` 에서 SSO 액세스 토큰을 받는다. 새로 설치한 곳은 켜기 전까지 아무것도 달라지지 않는다 |
| `mcp.oauth.resource` | 빈 값 | 리소스 식별자(RFC 8707). 비면 **일반** 탭의 서비스 공개 URL + `/mcp`. 클라이언트가 실제로 접속하는 공개 HTTPS 주소여야 한다 — 프록시 뒤의 `http://127.0.0.1:8080/mcp` 가 아니다 |
| `mcp.oauth.audience` | 빈 값 | 공백 구분 허용 대상. 토큰의 `aud` 또는 `azp` 가 이 가운데 하나여야 한다(아래 대상 검사) |
| `mcp.oauth.scopes` | `people:read memories:read` | SSO 토큰 주체에게 주는 권한 범위의 **천장**. 토큰이 무엇을 주장하든 이 범위를 넘지 않는다. 켜려면 하나 이상 골라야 한다 |
| (재사용) `oidc.issuer_url` · `oidc.client_id` | Keycloak SSO 설정 | 새로 만들지 않는다. issuer 가 인증 서버이고, 웹 로그인 클라이언트 ID 는 허용 대상에 자동으로 포함된다 |

켜지는 조건은 셋이 다 있을 때다: `oidc.issuer_url` 이 있고, 리소스 식별자를 만들 수
있고(`mcp.oauth.resource` 또는 서비스 공개 URL), 스위치가 켜져 있다. 저장 시점에
이 조건을 검사해 부족하면 400 으로 거부한다. 저장된 뒤 다른 탭에서 issuer 나 공개 URL
을 지우면 켜 두어도 꺼진 것처럼 동작한다: 메타데이터는 404 이고 로그에 `mcp oauth
enabled but unusable` 과 `reason` 이, 토큰은 거부되고 로그에 `mcp oauth token refused`
와 `reason="sso tokens unavailable: …"` 이 남는다. 카드에도 경고가 뜬다.

값은 `settings` 표의 `mcp:oauth` 행(JSON) 하나에 저장된다. 마이그레이션 009 가 기본
행(꺼짐)을 넣으므로 이미 돌고 있는 설치도 업그레이드만 하면 된다.

### 토큰을 어떻게 믿는가

`Authorization: Bearer <값>` 하나에서 가른다. `orb_` 로 시작하면 키(전과 동일),
아니고 JWT 모양(점 두 개)이면 토큰 검사, 둘 다 아니면 전과 같은 "로그인이 필요합니다".
토큰은 **`/mcp` 에서만** 받는다. REST·관리 API 는 지금처럼 키와 세션만 받는다.

검사 항목: 서명(Keycloak JWKS, RS/ES/PS 계열만 — `HS*`·`none` 거부) · `iss`
(`oidc.issuer_url` 과 같아야) · `exp` · `nbf` · `typ`(`ID`·`Refresh` 거부 — ID 토큰은
로그인 증거지 API 자격이 아니다) · `cnf`(있으면 거부 — 검증할 수 없는 소지자 증명) ·
`sub`(비면 거부) · 대상.

**대상 검사.** 다른 앱에 로그인해 받은 토큰이 Orbit 의 `/mcp` 를 열어서는 안 된다.
어느 하나는 맞아야 한다:

- `aud` 에 리소스 식별자(`https://<공개 주소>/mcp`)가 있다 — Keycloak 에 Audience 매퍼를
  둔 정식 경로
- `aud` 또는 `azp` 가 `mcp.oauth.audience`(또는 웹 로그인 `oidc.client_id`)에 있다 —
  매퍼 없이 쓰는 호환 경로. 실제 Keycloak 26 은 `aud` 에 `account` 만 싣고 클라이언트
  ID 는 `azp` 에 담으므로, MCP 클라이언트 ID 를 여기 적으면 된다

**계정은 만들지 않는다.** 토큰의 `sub`(웹 로그인이 묶어 둔 `oidc_subject`), 없으면
확인된 이메일(`email_verified=true`)로 **이미 등록된 활성** 계정을 찾는다 — 웹 로그인이
계정을 찾는 규칙과 같고 등록하는 절반만 없다. 없으면 "먼저 웹으로 한 번 로그인하세요",
비활성이면 거부. 토큰의 role claim 은 보지 않는다.

**권한은 키보다 넓지 않다.** SSO 주체는 그 사용자가 키로 들어왔을 때와 같은 범위
검사를 받는다. 범위는 `mcp.oauth.scopes` 가 정하며, 토큰의 `scope` 에 Orbit 의 범위
어휘(`people:read` 등)가 실려 오면 그 교집합만 준다. 교집합이 비면 빈 권한으로 통과시키지
않고 거부한다. `mcp:use` 는 문 자체라 언제나 포함된다.

### Keycloak 쪽 할 일

1. MCP 클라이언트용 **공개(public) 클라이언트**를 만든다. Standard Flow 켬, PKCE
   `S256`, Direct Access Grants·Implicit·Service accounts 끔. 웹 로그인 클라이언트와
   **다른** 클라이언트다.
2. Valid Redirect URIs 에 쓰는 MCP 클라이언트의 콜백을 정확히 적는다(Claude 는
   `https://claude.ai/api/mcp/auth_callback`, 로컬 클라이언트는
   `http://127.0.0.1:*/callback` 류). `*` 하나로 다 여는 것은 금지.
3. 정식 경로: 그 클라이언트(또는 전용 client scope)에 **Audience 매퍼** — Mapper type
   `Audience`, Included Custom Audience = 리소스 식별자(카드의 "MCP URL"), Add to access
   token 켬, Add to ID token 끔. 호환 경로: 매퍼 없이 Orbit 의 `mcp.oauth.audience` 에
   클라이언트 ID 를 적는다.
4. 액세스 토큰 수명은 짧게(5분 안팎). Orbit 은 introspection 을 하지 않으므로
   Keycloak 에서 로그아웃해도 이미 발급된 토큰은 만료까지 산다.

### curl 로 확인하기

```bash
# 1) 메타데이터 — 인증 없이 맨 JSON. 꺼져 있으면 404.
curl -s https://orbit.example.com/.well-known/oauth-protected-resource/mcp
# {"authorization_servers":["https://keycloak.example.com/realms/corp"],
#  "bearer_methods_supported":["header"],"resource":"https://orbit.example.com/mcp",
#  "resource_name":"Orbit MCP","scopes_supported":["people:read","memories:read"]}

# 2) 401 이 길을 가리킨다 — /mcp 에서만. REST 401 에는 이 헤더가 없다.
curl -si -X POST https://orbit.example.com/mcp -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | grep -i www-authenticate
# WWW-Authenticate: Bearer realm="Orbit", resource_metadata="https://orbit.example.com/.well-known/oauth-protected-resource/mcp"

# 3) 토큰으로 tools/list
curl -s -X POST https://orbit.example.com/mcp -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'
```

### 거부 메시지별 조치

401 본문의 `message` 가 아래 가운데 하나다. 같은 순간 서버 로그에 `mcp oauth token
refused` 와 `reason`·`error`(서명·발급자·만료 가운데 무엇인지)가 남는다.

| 메시지 | 원인 | 조치 |
| --- | --- | --- |
| 이 서버는 SSO 액세스 토큰을 받지 않습니다 | 꺼져 있거나 켜는 조건이 부족 | 카드의 스위치와 경고 확인. 로그의 `reason` 이 `sso tokens unavailable: …` 로 부족한 조건을 말한다 |
| Keycloak 발급자 정보를 읽지 못해 … | Orbit 서버에서 issuer 의 `/.well-known/openid-configuration` 에 닿지 못함 | 서버 → Keycloak 네트워크·TLS 확인 |
| SSO 액세스 토큰이 유효하지 않습니다(서명·발급자·만료) | 서명 키 불일치, `iss` 가 `oidc.issuer_url` 과 다름, 만료 | 로그의 `error` 로 셋 중 무엇인지 본다. issuer 는 realm 주소와 슬래시까지 같아야 한다 |
| 종류(typ=ID)가 액세스 토큰이 아닙니다 | 클라이언트가 ID 토큰을 보냄 | 액세스 토큰을 보내도록 클라이언트 설정 |
| 소지자 증명(cnf)이 묶여 있어 … | DPoP·mTLS 바인딩 토큰 | 해당 클라이언트에서 바인딩 끄기 |
| 이 서버를 위해 발급된 것이 아닙니다(aud=[account], azp="claude-mcp") | 대상 불일치 | 메시지의 `azp` 값을 `mcp.oauth.audience` 에 더하거나, Keycloak 클라이언트에 Audience 매퍼로 메시지 끝의 리소스 식별자를 넣는다 |
| 관리자가 SSO 주체에게 주는 범위가 비어 있습니다 / 토큰의 범위가 허용 범위와 겹치지 않습니다 | 천장이 비었거나 교집합이 빔 | 카드에서 범위를 고르거나, Keycloak 의 scope 매핑을 조정 |
| 이 SSO 계정은 Orbit 에 등록되지 않았습니다 | `sub`·확인된 이메일로 찾은 계정이 없음 | 그 사용자가 웹으로 한 번 로그인한다(자동 등록이 꺼져 있으면 관리자가 먼저 등록) |
| 이 Orbit 계정은 비활성 상태입니다 | 사용자 탭에서 비활성 | 관리자가 다시 활성화 |

### 한계

- Keycloak 로그아웃은 이미 발급된 토큰을 끊지 못한다(introspection 없음). 토큰 수명을
  짧게 둔다.
- Orbit 이 인증 서버 노릇을 하지 않으므로 동적 클라이언트 등록(RFC 7591)이 필요한
  클라이언트는 Keycloak 의 Client registration 정책으로 해결한다.
