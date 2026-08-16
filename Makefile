.PHONY: dev test build web docker release-image

VERSION ?= $(shell tr -d '\n' < VERSION)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILT_AT ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -s -w -X main.version=v$(VERSION) -X main.commit=$(COMMIT) -X main.builtAt=$(BUILT_AT)

dev:
	cd web && npm run dev

test:
	go test ./...
	cd web && npm run test -- --run

web:
	cd web && npm ci && npm run build
	rm -rf internal/webui/dist
	cp -R web/dist internal/webui/dist

build: web
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o orbit ./cmd/orbit

docker:
	docker build --build-arg VERSION=v$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILT_AT=$(BUILT_AT) -t orbit:v$(VERSION) .

release-image: docker
	docker save orbit:v$(VERSION) | gzip -9 > orbit-v$(VERSION).tar.gz
