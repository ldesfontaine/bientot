FROM golang:1.25-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o /out/bientot-agent ./cmd/agent
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o /out/bientot-dashboard ./cmd/dashboard


FROM alpine:3.20 AS agent
RUN adduser -D -u 10001 bientot
COPY --from=builder /out/bientot-agent /usr/local/bin/
USER bientot
ENTRYPOINT ["/usr/local/bin/bientot-agent"]


FROM alpine:3.20 AS dashboard
RUN adduser -D -u 10001 bientot
COPY --from=builder /out/bientot-dashboard /usr/local/bin/
USER bientot
ENTRYPOINT ["/usr/local/bin/bientot-dashboard"]
