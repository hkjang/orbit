# syntax=docker/dockerfile:1.7
FROM node:24-alpine AS web-builder
WORKDIR /src/web
# 이미지 빌드는 npm 캐시가 늘 비어 있어 레지스트리 장애를 그대로 맞는다.
# 릴리즈(태그 푸시 → docker build)가 여기서 죽지 않도록 재시도 래퍼로 설치한다.
# 실행 비트에 기대지 않으려고 sh 로 부른다(Windows 체크아웃에서 모드가 죽는다).
COPY scripts/npm-install.sh /usr/local/bin/npm-install.sh
COPY web/package.json web/package-lock.json ./
RUN sh /usr/local/bin/npm-install.sh
COPY web/ ./
RUN npm run build

FROM golang:1.26.6-alpine AS go-builder
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILT_AT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-builder /src/web/dist ./internal/webui/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.builtAt=${BUILT_AT}" \
    -o /out/orbit ./cmd/orbit

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.title="Orbit" \
      org.opencontainers.image.description="Private Personal Relationship Universe" \
      org.opencontainers.image.source="https://github.com/hkjang/orbit"
COPY --from=go-builder /out/orbit /orbit
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/orbit"]
