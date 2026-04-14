# Bientot

Monitoring agent/serveur modulaire en Go. Push-only, auto-detection, dual-listen.

## Architecture

```
Agent (15MB)  --> push HTTP signe HMAC --> Serveur (30MB) --> Dashboard + Alerting Ntfy
  (par machine)     token unique             (central)        + Threat Intel + CVE
```

- **Agent** : binaire Go statique, amd64+arm64, auto-detecte les modules disponibles
- **Serveur** : dual-listen :3001 (dashboard via Traefik) / :3002 (agents via mesh direct)
- **Stockage** : SQLite WAL + downsampling automatique
- **Temps reel** : SSE pour le dashboard, WebSocket pour le command channel

## Quick start

```bash
# Dev local
make init              # cree data/ et .env depuis .env.example
docker compose up      # agent + serveur

# Prod (via Ansible — voir le repo zero-trust)
ansible-playbook -i inventory.ini playbooks/services.yml --tags bientot --ask-vault-pass
```

Dashboard accessible sur `http://localhost:3001`

## Modules auto-detectes

L'agent detecte automatiquement les modules disponibles sur la machine.

| Module | Detection | Metriques | Variable d'env |
|--------|-----------|-----------|----------------|
| system | `NODE_EXPORTER_URL` defini | CPU, RAM, disk, uptime | `NODE_EXPORTER_URL` |
| docker | `DOCKER_HOST` defini | containers, health, images | `DOCKER_HOST` |
| crowdsec | `CROWDSEC_URL` + LAPI accessible | decisions, bouncers, alertes | `CROWDSEC_URL`, `CROWDSEC_API_KEY` |
| adguard | `ADGUARD_URL` defini | queries, blocked, top domains | `ADGUARD_URL`, `ADGUARD_USER`, `ADGUARD_PASSWORD` |
| netbird | `NETBIRD_PEER_IP` defini | peers, latence mesh | `NETBIRD_PEER_IP`, `NETBIRD_PEER_PORT` |
| traefik | `TRAEFIK_API_URL` defini | routers, services, entrypoints | `TRAEFIK_API_URL`, `DOCKER_SOCKET` |
| backup | dossier `/status` present | age backup, taille, statut | `BACKUP_STATUS_DIR` |
| certs | `CERT_DOMAINS` defini | expiration TLS par domaine | `CERT_DOMAINS` (comma-separated) |
| git | `GIT_REPOS` defini | commits, branches, dirty state | `GIT_REPOS` (comma-separated) |

## Configuration

Tout passe par variables d'environnement (pas de fichier YAML pour l'agent).
Voir `.env.example` pour la liste complete.

### Agent (obligatoire)

```bash
BIENTOT_SERVER_URL=http://serveur:3002   # Endpoint push
BIENTOT_MACHINE_ID=sentinelle            # ID unique de la machine
BIENTOT_TOKEN=secret                     # Token HMAC (unique par machine)
```

### Serveur (obligatoire)

```bash
BIENTOT_AGENTS=machine1:token1,machine2:token2   # Tokens autorises
```

### Optionnel

```bash
PUSH_HOT=10s / PUSH_WARM=1m / PUSH_COLD=5m      # Intervalles push
DASHBOARD_ADDR=0.0.0.0:3001                       # Port dashboard
AGENT_ADDR=0.0.0.0:3002                           # Port agents
DB_PATH=/data/bientot.db                           # SQLite
LOG_LEVEL=info                                     # debug|info|warn|error
COMMAND_CHANNEL=true                               # Canal de commandes (opt-in)
NTFY_SERVER_URL / NTFY_TOPIC / NTFY_TOKEN          # Alerting Ntfy
VEILLE_URL / VEILLE_TOKEN                          # Integration veille-secu
ENRICHMENT_CONFIG / GEOIP_DB_PATH                  # Enrichissement CTI
ABUSEIPDB_API_KEY / GREYNOISE_API_KEY / CROWDSEC_CTI_KEY  # Providers
```

## Securite

- **Push-only** : aucun port expose par l'agent
- **HMAC-SHA256** + nonce UUID + timestamp anti-replay (60s)
- **Token unique par machine** : le serveur rejette si token != machine_id
- **Docker socket** via docker-socket-proxy (GET only, TCP)
- **Command channel** opt-in, desactive par defaut
- **Dual-listen** : agents jamais exposes via Traefik

## Build

```bash
make build               # agent + server
make build-agent         # agent seul (CGO_ENABLED=0)
make build-server        # server seul (CGO_ENABLED=1, SQLite)
make test                # go test -race ./...
make lint                # golangci-lint
make docker-multiarch    # images amd64 + arm64
make docker-dev          # docker compose up --build
```

## API

### Dashboard (:3001 — via Traefik)

```
GET  /health                          Healthcheck
GET  /api/status                      Status global
GET  /api/machines                    Machines connectees
GET  /api/machines/{id}/metrics       Metriques par machine
GET  /api/metrics                     Liste des metriques
GET  /api/metrics/{name}              Time-series
GET  /api/metrics/{name}/latest       Derniere valeur

GET  /api/alerts                      Toutes les alertes
GET  /api/alerts/active               Alertes actives
POST /api/alerts/{id}/ack             Acquitter une alerte

GET  /api/threats                     Resume threat intel
GET  /api/threats/attackers           Top attaquants
GET  /api/threats/patterns            Patterns d'attaque
GET  /api/threats/unblocked           IPs non bloquees
GET  /api/threats/budget              Budget API CTI

GET  /api/vulns                       Vulnerabilites
GET  /api/vulns/active                Vulns actives
GET  /api/vulns/inventory             Inventaire logiciel
GET  /api/vulns/sync                  Logs de sync
PATCH /api/vulns/{id}/dismiss         Ignorer une vuln
PATCH /api/vulns/{id}/resolve         Marquer resolue

GET  /api/services                    Services decouverts
GET  /api/events                      SSE temps reel
POST /api/commands                    Envoyer commande agent
GET  /api/commands/agents             Agents connectes
```

### Agent (:3002 — mesh direct, PAS via Traefik)

```
POST /push                            Push metriques (HMAC)
POST /scan/ingest                     Resultats scan CVE (Grype ou Trivy)
GET  /health                          Healthcheck
GET  /ws                              WebSocket command channel
```

## Stack

- **Go 1.22** (agent CGO=0, serveur CGO=1 pour SQLite)
- **SQLite WAL** + downsampling automatique
- **Frontend** : HTML/JS + HTMX + uPlot + Tailwind (embed.FS)
- **Container** : Alpine, agent ~15MB, serveur ~30MB