# syntax=docker/dockerfile:1.7
FROM node:24-alpine AS web-builder
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
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
