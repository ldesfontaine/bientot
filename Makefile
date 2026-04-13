.PHONY: build-agent build-server build run test clean docker docker-run fmt lint init deps \
       build-agent-arm64 build-agent-amd64 build-all-arch docker-multiarch

GOARCH ?= $(shell go env GOARCH)

# ── Build ──
build-agent:
	CGO_ENABLED=0 GOARCH=$(GOARCH) go build -ldflags="-s -w" -o bientot-agent ./cmd/agent

build-server:
	CGO_ENABLED=1 GOARCH=$(GOARCH) go build -ldflags="-s -w" -o bientot-server ./cmd/server

build: build-agent build-server

# ── Multi-arch ──
build-agent-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bientot-agent-amd64 ./cmd/agent

build-agent-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bientot-agent-arm64 ./cmd/agent

build-all-arch: build-agent-amd64 build-agent-arm64

# ── Run ──
run-agent: build-agent
	./bientot-agent

run-server: build-server
	./bientot-server

# ── Test ──
test:
	go test -race ./...

# ── Clean ──
clean:
	rm -f bientot-agent bientot-server bientot-agent-amd64 bientot-agent-arm64
	rm -rf data/

# ── Format & Lint ──
fmt:
	go fmt ./...

lint:
	golangci-lint run

# ── Docker ──
docker-agent:
	docker build --target agent -t bientot-agent .

docker-server:
	docker build --target server -t bientot-server .

docker-multiarch:
	docker buildx build --platform linux/amd64,linux/arm64 --target agent -t bientot-agent .
	docker buildx build --platform linux/amd64,linux/arm64 --target server -t bientot-server .

docker-dev:
	docker compose up --build

docker-dev-up:
	docker compose up --build -d

docker-dev-stop:
	docker compose down

# ── Init ──
init:
	mkdir -p data
	cp .env.example .env

# ── Deps ──
deps:
	go mod download
	go mod tidy
