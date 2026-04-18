COMPOSE := docker compose -f deploy/compose.dev.yml --project-directory .

.PHONY: build run-agent run-dashboard test clean docker-build docker-up docker-down docker-logs bootstrap-ca test-echo

build:
	go build -ldflags="-s -w" -o bin/bientot-agent ./cmd/agent
	go build -ldflags="-s -w" -o bin/bientot-dashboard ./cmd/dashboard
	go build -ldflags="-s -w" -o bin/bientot-echo-server ./cmd/echo-server

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

bootstrap-ca:
	./scripts/bootstrap-ca.sh

test-echo:
	@curl --cacert deploy/certs/agent-vps/ca-bundle.crt \
	      --cert deploy/certs/agent-vps/client.crt \
	      --key deploy/certs/agent-vps/client.key \
	      https://localhost:8443/ping \
	      || echo "ECHO TEST FAILED"
