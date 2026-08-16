# Orbit — Personal Relationship Universe

<p align="center">
  <img src="docs/images/09_orbit_canvas_populated.png" alt="Orbit Universe Canvas" width="900" style="border-radius: 12px; box-shadow: 0 10px 30px rgba(0,0,0,0.5);">
</p>

<p align="center">
  <strong>"Your relationships have gravity."</strong><br>
  사람들을 단순한 CRM 점수로 다루지 않고, 나와 그 사람 사이의 중력(Gravity), 거리, 시간의 흐름을 하나의 우주로 시각화하는 프라이빗 관계 관리 플랫폼.
</p>

<p align="center">
  <a href="https://github.com/hkjang/orbit"><img src="https://img.shields.io/badge/License-Apache--2.0-blue.svg" alt="License"></a>
  <a href="https://golang.org"><img src="https://img.shields.io/badge/Go-1.26+-00ADD8.svg?logo=go" alt="Go"></a>
  <a href="https://react.dev"><img src="https://img.shields.io/badge/React-19.x-61DAFB.svg?logo=react" alt="React"></a>
  <a href="https://www.postgresql.org"><img src="https://img.shields.io/badge/PostgreSQL-16+-4169E1.svg?logo=postgresql" alt="PostgreSQL"></a>
  <a href="docs/index.html"><img src="https://img.shields.io/badge/Promo%20Page-GitHub%20Pages-purple.svg" alt="GitHub Pages"></a>
</p>

---

## 🌟 핵심 특징 (Key Highlights)

- **🌌 Orbit Canvas with Collision Relaxation**: HTML5 Canvas 기반의 60fps 고성능 독립 렌더러. 관계 중요도와 교류 빈도에 따라 실시간 궤도와 중력 물리 시뮬레이션 적용.
- **📖 Relationship Stories & Memories**: 언제 처음 만났는지, 어떤 이야기를 나누었는지 나만의 맥락을 기록하고 타임라인으로 보존.
- **🔐 Per-User AES-256-GCM Envelope Encryption**: 모든 교류 기록과 소중한 기억은 사용자별 고유 데이터 키로 암호화되며, 무중단 키 회전(Key Rotation) 지원.
- **🤖 AI Relationship Co-Pilot & Rediscover**: 과거 기억 맥락을 참조하여 소원해진 인연과의 따뜻한 후속 교류 전략 제안 (OpenAI Responses SSE 스트리밍 및 최대 256K 토큰 지원).
- **🔌 Streamable HTTP MCP Endpoint**: AI 에이전트 도구와 안전하게 나의 관계 컨텍스트를 연동할 수 있는 `/mcp` 표준 엔드포인트 제공.
- **🛡️ Enterprise Governance & Keycloak OIDC**: 외부 이해관계자 등록 시 팀장 승인/반려 워크플로우 및 Keycloak SSO Discovery/PKCE 연동.

---

## 📸 주요 화면 갤러리 (Feature Showcase)

| 01. 로그인 & OIDC SSO | 02. 나의 우주 (Orbit Canvas) |
| :---: | :---: |
| <img src="docs/images/01_login_page.png" width="440"> | <img src="docs/images/09_orbit_canvas_populated.png" width="440"> |
| **03. 인물 등록 (CRU Create)** | **04. 관계 목록 (People Grid)** |
| <img src="docs/images/03_person_create_modal.png" width="440"> | <img src="docs/images/04_people_list_grid.png" width="440"> |
| **05. 인물 상세 & 타임라인** | **06. 교류 기록 추가 (Interaction)** |
| <img src="docs/images/05_person_detail.png" width="440"> | <img src="docs/images/06_interaction_create_modal.png" width="440"> |
| **07. 기억 보관소 (Memories)** | **08. 서비스 관리 콘솔 (Admin)** |
| <img src="docs/images/10_memories_rediscover.png" width="440"> | <img src="docs/images/14_admin_service_console.png" width="440"> |

---

## 🚀 빠른 시작 가이드 (Quick Start)

### 1. PostgreSQL DB 기동
```bash
docker run -d --name orbit-db \
  -p 5432:5432 \
  -e POSTGRES_USER=orbit \
  -e POSTGRES_PASSWORD=orbit \
  -e POSTGRES_DB=orbit \
  postgres:16-alpine
```

### 2. 필수 환경변수 설정 (4종)
```bash
export DATABASE_URL='postgresql://orbit:orbit@127.0.0.1:5432/orbit?sslmode=disable'
export BOOTSTRAP_ADMIN='admin@example.internal'
export BOOTSTRAP_ADMIN_PASSWORD='a-long-bootstrap-password'
export ENCRYPTION_KEY="$(openssl rand -base64 32)"
```

### 3. Orbit 빌드 및 실행
```bash
# 프론트엔드 빌드 후 Go 단일 바이너리 컴파일
cd web && npm ci && npm run build && cd ..
mkdir -p internal/webui/dist && cp -a web/dist/. internal/webui/dist/
go build -o orbit ./cmd/orbit

# 서비스 실행 (Port 8080)
./orbit
```

브라우저에서 `http://localhost:8080`으로 접속하여 Orbit 우주를 시작하세요.

---

## 📚 상세 문서 (Documentation)

- 📖 [사용자 가이드 (User Guide)](docs/guide.md)
- 📝 [관계 수명주기 실무 매뉴얼 (CRU Manual)](docs/cru-manual.md)
- 🌐 [인터랙티브 웹 홍보 페이지 (GitHub Landing Page)](docs/index.html)
- 🏛️ [시스템 아키텍처 (Architecture)](docs/ARCHITECTURE.md)
- 🔌 [REST & Streamable MCP API 명세 (API)](docs/API.md)
- 📦 [오프라인 에어갭 배포 (Offline Operations)](docs/OFFLINE.md)

---

## 📄 라이선스 (License)

Orbit은 Apache License 2.0 하에 배포됩니다.
