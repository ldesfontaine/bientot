# Bientot

Dashboard de monitoring et alerting modulaire. Se branche sur n'importe quelle infra.

## Quick Start

```bash
# Clone et init
git clone <repo>
cd bientot
make init

# Editer la config
vim .env
vim config/targets.yml
vim config/alerts.yml

# Lancer
make docker-up
```

Dashboard accessible sur `http://localhost:3001`

## Architecture

```
Sources (configurable)     Storage              Sorties
----------------------     -------              -------
Prometheus endpoints  -->            --> Dashboard web
Docker health         -->  SQLite    --> Alerting (Ntfy, webhook)
ZFS status            -->            --> API REST
JSON files            -->
```

**Modulaire** : ajoute un collector ou notifier en config, pas en code.

## Configuration

### targets.yml

Definit les sources de metriques :

```yaml
collectors:
  prometheus:
    - name: mon-serveur
      url: http://localhost:9100/metrics
      interval: 30s
  docker:
    enabled: true
    socket: /var/run/docker.sock
  zfs:
    enabled: true
    pools: [tank]
```

### alerts.yml

Definit les regles d'alerte :

```yaml
alerts:
  - name: disk_critical
    expr: disk_used_percent > 90
    severity: critical
    message: "Disk {{ .Value }}%"

notifiers:
  - type: ntfy
    url: ${NTFY_URL}
    topic: ${NTFY_TOPIC}
```

### .env

Secrets et config runtime :

```bash
NTFY_URL=https://ntfy.example.com
NTFY_TOPIC=alerts
```

## API

```
GET  /health          - Healthcheck
GET  /api/status      - Status global
GET  /api/metrics     - Liste des metriques
GET  /api/metrics/:n  - Query time-series
GET  /api/alerts      - Alertes actives
POST /api/alerts/:id  - Acknowledge alert
```

## Dev

```bash
make build    # Compile
make run      # Lance localement
make test     # Tests
make docker   # Build image
```

## Stack

- **Backend** : Go 1.22
- **Storage** : SQLite + downsampling automatique
- **Frontend** : HTML/JS + HTMX + uPlot + Tailwind
- **Container** : Alpine ~20MB
