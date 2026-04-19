COMPOSE := docker compose -f deploy/compose.dev.yml --project-directory .

.PHONY: help build run-agent run-dashboard test clean docker-build docker-up docker-down docker-logs bootstrap-ca test-dashboard proto proto-lint proto-breaking

.DEFAULT_GOAL := help

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

build: ## Build both binaries into bin/
	go build -ldflags="-s -w" -o bin/bientot-agent ./cmd/agent
	go build -ldflags="-s -w" -o bin/bientot-dashboard ./cmd/dashboard

run-agent: build ## Build and run the agent locally
	./bin/bientot-agent

run-dashboard: build ## Build and run the dashboard locally
	./bin/bientot-dashboard

test: ## Run all tests with race detector
	go test -race ./...

clean: ## Remove build artifacts
	rm -rf bin/

docker-build: ## Build all docker images
	$(COMPOSE) build

docker-up: ## Start the dev stack in the background
	$(COMPOSE) up --build -d

docker-down: ## Stop the dev stack
	$(COMPOSE) down

docker-logs: ## Tail logs from the dev stack
	$(COMPOSE) logs -f

bootstrap-ca: ## Generate CA + agent/dashboard certs (requires step-ca up)
	./scripts/bootstrap-ca.sh

test-dashboard: ## Hit the dashboard /ping endpoint with the agent-vps cert
	@curl --cacert deploy/certs/agent-vps/ca-bundle.crt \
	      --cert deploy/certs/agent-vps/client.crt \
	      --key deploy/certs/agent-vps/client.key \
	      https://localhost:8443/ping \
	      || echo "DASHBOARD TEST FAILED"

proto: ## Regenerate protobuf stubs
	rm -rf api/v1/gen
	PATH="$(shell go env GOPATH)/bin:$(PATH)" buf generate

proto-lint: ## Lint proto files
	buf lint

proto-breaking: ## Check proto for breaking changes vs main
	buf breaking --against '.git#branch=main'
