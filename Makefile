.PHONY: build run test clean docker docker-run

# Variables
BINARY_NAME=bientot
DOCKER_IMAGE=bientot

# Build
build:
	CGO_ENABLED=1 go build -o $(BINARY_NAME) ./cmd/bientot

# Run locally
run: build
	./$(BINARY_NAME)

# Run with hot reload (requires air)
dev:
	air

# Test
test:
	go test -v ./...

# Clean
clean:
	rm -f $(BINARY_NAME)
	rm -rf data/

# Format
fmt:
	go fmt ./...

# Lint (requires golangci-lint)
lint:
	golangci-lint run

# Docker build
docker:
	docker build -t $(DOCKER_IMAGE) .

# Docker run
docker-run:
	docker compose up

# Docker run detached
docker-up:
	docker compose up -d

# Docker stop
docker-down:
	docker compose down

# Init database directory
init:
	mkdir -p data
	cp .env.example .env

# Download dependencies
deps:
	go mod download
	go mod tidy
