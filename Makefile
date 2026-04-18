.PHONY: build run-agent run-dashboard test clean

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
