# Dev Setup — Bientôt v2

## Quick Start
```bash
make init && make docker-dev
open http://localhost:3001
```

## Services dev
| Service | Port | Rôle |
|---------|------|------|
| bientot-server | 3001 | Dashboard web |
| bientot-server | 3002 | Endpoint agents |
| bientot-agent | — | Push vers :3002 |

## Config dev
```yaml
# config/agent.dev.yml
server:
  url: "http://localhost:3002"
  token: "${BIENTOT_TOKEN_LOCAL}"
machine:
  name: "dev-local"
push:
  hot: 10s
  warm: 1m
  cold: 5m
command_channel:
  enabled: false
```

```yaml
# config/server.dev.yml
server:
  dashboard_port: 3001
  agent_port: 3002
  db_path: "./data/bientot.db"
agents:
  - machine: "dev-local"
    token: "${BIENTOT_TOKEN_LOCAL}"
enrichment:
  enabled: false
veille:
  enabled: false
```

## Commandes
```bash
make docker-dev / docker-dev-up / docker-dev-stop
make build-agent / build-server
GOARCH=arm64 make build-agent
make test && make lint
```

## Tester d'autres modules
```bash
docker run -d --name crowdsec crowdsecurity/crowdsec
docker run -d traefik:v3.0 --accesslog
```

Note : en dev, l'agent accède au Docker socket directement (auto-détecté).
En prod, il passe par docker-socket-proxy (voir `docs/architecture-security.md`).
