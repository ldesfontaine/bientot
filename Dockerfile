# ── Builder ──
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .

# ── Agent: binaire pur, pas de CGO ──
FROM builder AS build-agent
ARG TARGETARCH
RUN CGO_ENABLED=0 GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /bientot-agent ./cmd/agent

# ── Server: SQLite (CGO) ──
FROM builder AS build-server
ARG TARGETARCH
RUN CGO_ENABLED=1 GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /bientot-server ./cmd/server

# ── Image agent (~15MB) ──
FROM alpine:3.21 AS agent
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 999 docker \
    && addgroup -S bientot && adduser -S bientot -G bientot \
    && adduser bientot docker
COPY --from=build-agent /bientot-agent /usr/local/bin/
USER bientot
HEALTHCHECK --interval=60s --timeout=5s --retries=3 CMD kill -0 1 || exit 1
ENTRYPOINT ["bientot-agent"]

# ── Image server (~30MB) ──
FROM alpine:3.21 AS server
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S bientot && adduser -S bientot -G bientot \
    && mkdir -p /data && chown bientot:bientot /data
COPY --from=build-server /bientot-server /usr/local/bin/
COPY --from=build-server /app/config /app/config
USER bientot
EXPOSE 3001 3002
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD wget -q --spider http://localhost:3001/ || exit 1
ENTRYPOINT ["bientot-server"]
