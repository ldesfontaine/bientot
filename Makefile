COMPOSE := docker compose -f deploy/compose.dev.yml

.PHONY: build run-agent run-dashboard test clean docker-build docker-up docker-down docker-logs

build:
	go build -ldflags="-s -w" -o bin/bientot-agent ./cmd/agent
	go build -ldflags="-s -w" -o bin/bientot-dashboard ./cmd/dashboard

run-agent: build
	./bin/bientot-agent

run-dashboard: build
	./bin/bientot-dashboard

test:
	go test -race ./...

clean:
	rm -rf bin/

docker-build:
	$(COMPOSE) build

docker-up:
	$(COMPOSE) up --build -d

docker-down:
	$(COMPOSE) down

docker-logs:
	$(COMPOSE) logs -f
