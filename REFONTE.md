````markdown
# Bientôt v2 — Journal de refonte

> **Document de référence vivant.** Mis à jour à chaque feature/palier validé.
> Dernière mise à jour : **2026-04-20** — sub-palier 5.1 complet (API JSON read-only). 4 endpoints : `/api/health`, `/api/agents`, `/api/agents/{id}/metrics`, `/api/agents/{id}/metric-points`. Prêt pour 5.2 (templates HTML + HTMX).

---

## 🎯 Vision

Refonte complète de [Bientôt](https://github.com/ldesfontaine/bientot) sur la branche `refonte`, en partant de zéro en gardant l'historique git.

Objectifs :
- **Sécurité niveau production** : mTLS + JWT court + signature Ed25519. Aucune confiance dans le réseau sous-jacent.
- **Extensible** : N agents sans reconfig serveur, modules agent ajoutables sans toucher au core.
- **Packageable Docker** : agent et dashboard = deux images distinctes, déployables séparément.
- **Prêt pour intégrations futures** :
  - `veille-secu` (matching CVE via `software_inventory` remonté par l'agent)
  - CTI (enrichissement IPs attaquantes via `raw_events` remontés par l'agent)

## 🏗️ Architecture cible

```
┌──────────────────── CA INTERNE (step-ca) ────────────────────┐
│  Émet les certificats X.509 de tous les nœuds                 │
│  Rotation auto 24h via ACME                                   │
└─────────────┬──────────────────────────────┬──────────────────┘
              │ émet cert                    │ émet cert
              ▼                              ▼
    ┌──────────────────┐            ┌──────────────────┐
    │   Agent (×N)     │◄── mTLS ───►│    Dashboard     │
    │                  │  HTTP/2    │                   │
    │ cert client      │  + JWT     │ cert serveur      │
    │ + sign Ed25519   │  + sign    │ + vérif Ed25519   │
    └──────────────────┘            └────────┬─────────┘
                                             ▼
                                       ┌───────────┐
                                       │  SQLite   │
                                       │ + corrél. │
                                       └───────────┘
```

## 📐 Décisions d'architecture

### Stack technique
| Choix | Justification |
|---|---|
| **Go 1.24** | Binaire statique, stdlib riche (crypto/tls, crypto/ed25519, net/http) |
| **Monorepo** | Un seul contract protobuf, pas de dérive de version agent/serveur |
| **Protobuf + Connect-go** | Contract strict, versionnable, debuggable au curl |
| **step-ca** | CA automatisée, rotation cert auto, SCEP/ACME natif |
| **mTLS** | Auth transport obligatoire, pas de MITM possible |
| **JWT 5 min** | Identité applicative post-handshake, perms granulaires |
| **Ed25519** | Signature applicative, plus simple que HMAC, non-répudiable |
| **SQLite WAL** | Suffit pour N≤20 agents, pas d'ops, downsampling maison |
| **HTMX + uPlot + Tailwind** | Front léger, embed.FS, zéro runtime JS côté serveur |
| **Docker multi-target** | Un Dockerfile, deux images (agent, dashboard) |

### Sécurité en 3 couches empilées

1. **Transport** : mTLS obligatoire (cert client + cert serveur signés par CA interne)
2. **Identité** : JWT court-vécu (5 min) émis après handshake mTLS, avec permissions granulaires
3. **Intégrité message** : signature Ed25519 sur `{machine_id, timestamp, nonce, body}` canoniquement encodé

### Principes directeurs
- **Pas de code "sécurité à faire plus tard"** : mTLS dès le palier 2
- **Contract-first** : le `.proto` est la source de vérité, agent et dashboard génèrent leur code
- **Tests dès la première feature qui compte** : pas de "on verra"
- **Secrets jamais en env var** : tmpfs via Docker secrets, mode 0400
- **Defense-in-depth sur le dashboard** : auth native même si derrière reverse-proxy

## 🗺️ Arborescence du repo

```
bientot/
├── api/v1/                  # protobuf (contract unique)
│   └── gen/                 # code Go généré
├── cmd/
│   ├── agent/               # entrypoint binaire agent
│   └── dashboard/           # entrypoint binaire dashboard
├── internal/
│   ├── shared/              # crypto, mtls, proto helpers (partagé)
│   ├── agent/               # logique agent
│   ├── modules/             # modules collecteurs (agent uniquement)
│   └── dashboard/           # logique dashboard (storage, corrélation, API)
├── web/                     # front HTMX (embed.FS)
├── deploy/
│   ├── compose.dev.yml
│   ├── compose.prod.yml
│   └── certs/               # certs générés (gitignored)
├── scripts/
│   ├── bootstrap-ca.sh
│   └── gen-proto.sh
├── docs/                    # docs techniques (01-ARCHI, 02-SECU, etc.)
├── .github/workflows/       # CI/CD
├── Dockerfile               # multi-stage, multi-target
├── Makefile                 # interface unique dev
├── buf.yaml / buf.gen.yaml  # config Buf protobuf
└── REFONTE.md               # ce fichier
```

**Règle de dépendance stricte** : `internal/agent/` et `internal/dashboard/` ne s'importent JAMAIS l'un l'autre. Ils communiquent uniquement via le contract `api/v1/gen/`.

## 🧩 Architecture des modules

Les modules sont chargés via un pattern registry + config YAML :

- Chaque module implémente `modules.Module` et expose `Factory(config) (Module, error)`
- Chaque module s'auto-enregistre via `init()` dans son package
- Le main importe `_ "internal/modules/registry"` pour déclencher les inits
- `modules.Build(configs)` assemble les modules selon `deploy/agent.yaml`

Ajouter un nouveau module = écrire le code + 1 ligne dans `internal/modules/registry/registry.go`.
Déployer différemment sur une machine = éditer `agent.yaml`, pas de rebuild.

## 📋 Paliers

| # | Nom | Statut | Résultat attendu |
|---|---|---|---|
| 0 | Squelette | ✅ VALIDÉ | `make build` + `docker-up` → logs "starting" des deux binaires |
| 1 | Agent autonome + interface Module | ✅ VALIDÉ | Module `heartbeat` détecté et collecté en boucle |
| 2 | mTLS bootstrap | ✅ VALIDÉ | Agent handshake mTLS vers echo-server, tamper cert → rejet |
| 3 | Protocole signé (protobuf + Ed25519) | ✅ VALIDÉ | PushRequest signée, tamper 1 byte → rejet |
| 4 | 1er module qui push (system) | ✅ VALIDÉ | Métriques CPU/RAM visibles côté dashboard de test |
| 5 | Tous les modules + software_inventory | ⬜ | 8 modules actifs, inventaire logiciel rempli |
| 6 | Agent production-ready | ⬜ | Healthcheck basé last-push, backoff, rotation cert auto |
| 7 | Dashboard — stockage + pipeline | ⬜ | SQLite + pipeline corrélation 6 stages |
| 8 | Dashboard — front HTMX | ⬜ | UI de monitoring de base (machines, métriques, alertes) |
| 9 | Intégration veille-secu (CVE) | ⬜ | Matching software_inventory × CVE depuis veille-secu |
| 10 | Intégration CTI (IPs) | ⬜ | Enrichissement GeoIP + blocklists + AbuseIPDB |

Légende : ⬜ TODO — 🟡 EN COURS — ✅ VALIDÉ

## 🔨 Palier en cours : Palier 0 — Squelette

### Objectif
Créer le repo vide, poser les fondations Go + Docker, obtenir deux binaires qui démarrent, loggent leur ligne de start, répondent proprement à `SIGTERM`.

### Features (approche par unité logique)

Chaque feature = un résultat observable + un commit atomique.

#### Feature 0.1 — Bootstrap du module Go 🟡
**Fichiers** : `.gitignore`, `go.mod`, `README.md`
**Résultat** : le repo a une identité Go, il ne commitera plus de saloperies, GitHub affiche un README.
**Commit** : `chore(refonte): bootstrap go module and readme`

- [x] `.gitignore`
- [x] `go.mod` (via `go mod init github.com/ldesfontaine/bientot`)
- [ ] `README.md` (minimal, "WIP refonte")

#### Feature 0.2 — Les deux binaires qui tournent en local ✅
**Fichiers** : `cmd/agent/main.go`, `cmd/dashboard/main.go`, `Makefile`
**Résultat** :
```bash
make build
./bin/bientot-agent       # log "agent starting", Ctrl+C → "agent stopped"
./bin/bientot-dashboard   # idem
```
**Commit** : `feat(refonte): add agent and dashboard binaries with signal handling`

#### Feature 0.3 — Les deux binaires qui tournent en Docker ✅
**Fichiers** : `Dockerfile`, `deploy/compose.dev.yml`
**Résultat** :
```bash
make docker-up
docker compose -f deploy/compose.dev.yml logs
# les deux containers loggent leur ligne de start
make docker-down
```
**Commit** : `feat(refonte): add docker packaging for agent and dashboard`

### Critère de validation global du palier 0
```bash
make build                        # produit bin/bientot-agent et bin/bientot-dashboard
make run-agent                    # log "agent starting", Ctrl+C → log "agent stopped"
make docker-up                    # deux containers démarrent, leurs logs apparaissent
docker compose -f deploy/compose.dev.yml logs
make docker-down
```

Si ces 4 commandes passent sans erreur → ✅ palier 0 validé.

## 📖 Journal d'avancement

- **2026-04-18 (matin)** — Démarrage. Branche `refonte` créée. Roadmap initialisée. Décisions d'architecture figées.
- **2026-04-18 (après-midi)** — Workflow clarifié : travail **feature-par-feature** (pas fichier-par-fichier). Palier 0 redécoupé en features 0.1, 0.2, 0.3.
- **2026-04-18 (soir)** — Feature 0.1 en cours : `.gitignore` ✅ et `go.mod` ✅ commités. Reste `README.md`.
- **2026-04-18 (soir)** — Feature 0.2 ✅ : agent et dashboard démarrent, loggent en JSON, gèrent SIGINT/SIGTERM proprement.
- **2026-04-18 (soir)** — Feature 0.3 ✅ : Dockerfile multi-stage/multi-target, compose dev, user non-root UID 10001. Palier 0 validé.
- **2026-04-18 (nuit)** — 🎉 **Palier 1 VALIDÉ**. Interface Module posée (prête pour CVE+CTI), module heartbeat testé, boucle agent multi-goroutine fonctionnelle. Premier test unitaire du projet en place. 12 collectes régulières validées à 3s d'interval (ramené à 30s).
- **2026-04-18 (nuit)** — Feature 2.1 ✅ : step-ca containerisé, isolé sur réseau dédié, password admin piloté par `.env`. Piège Docker Compose attrapé : l'interpolation `${VAR}` lit `.env` dans le project-dir (dossier du compose par défaut), pas via `env_file:`. Fix : `--project-directory .` + fail-fast `${STEP_CA_PASSWORD:?...}`. Validation contractuelle via `step ca provisioner list`.
- **2026-04-18 (nuit)** — Feature 2.2 ✅ : `scripts/bootstrap-ca.sh` idempotent, génère root+intermediate+leafs (dashboard server, agent-vps client) avec SAN `dashboard,localhost`. Chaîne vérifiée par `openssl verify`. TTL 24h (renouvellement auto au palier 6).
- **2026-04-18 (nuit)** — Feature 2.3 ✅ : echo-server mTLS fonctionnel. Handshake `RequireAndVerifyClientCert` + TLS 1.3 validé sur 3 scénarios (cert légitime, aucun cert, cert d'une autre CA). Fix perms UID via `user: ${HOST_UID:-1000}` pour bind-mount certs.
- **2026-04-18 (nuit)** — Feature 2.4 ✅ : helper `internal/shared/mtls/` avec `ClientConfig` + 3 tests (success, cert manquant, CA invalide). Enforce TLS 1.3 + ServerName obligatoire (anti-MITM).
- **2026-04-18 (nuit)** — Feature 2.5 ✅ : agent parle mTLS à echo-server bout-en-bout. Package `internal/agent/client/` avec retry implicite sur tick. Refactor `mtls.ServerConfig` utilisé par echo-server et tests. Cert serveur gagne SAN `echo-server`. Résilience testée : echo down → warn → retry → recovery.
- **2026-04-18 (nuit)** — 🎉 **Palier 2 VALIDÉ** — mTLS bout-en-bout. CA step-ca + bootstrap idempotent + echo-server mTLS + client agent avec tests de régression sécurité (no cert / wrong CA). Écart Go 1.21+ géré via `GetClientCertificate`. 6 features, ~300 lignes prod + ~200 lignes tests.
- **2026-04-18 (nuit)** — Feature 3.1 ✅ : buf + protoc-gen-go configurés, contrat protobuf `PushRequest` v1 posé (`api/v1/ingest.proto`). Makefile étend `PATH` avec `$GOPATH/bin` pour que `buf generate` trouve le plugin sans modifier le shell rc. `PACKAGE_DIRECTORY_MATCH` exclu explicitement (mono-produit, pas besoin du nesting `bientot/v1/`). Round-trip marshal/unmarshal validé.
- **2026-04-18 (nuit)** — Feature 3.2 ✅ : package `internal/shared/crypto/` avec Sign/Verify Ed25519 sur PushRequest. Canonical encoding via `proto.MarshalOptions{Deterministic: true}`, signature cleared pattern pour éviter le chicken-and-egg. 5 tests couvrent roundtrip, tamper, wrong key, determinism, invalid key.
- **2026-04-18 (nuit)** — Feature 3.3 ✅ : package `internal/shared/keys/` + extension du bootstrap script. Chaque agent a sa paire Ed25519 (`signing.key`/`signing.pub`), clé publique copiée côté dashboard dans `agent-keys/<machine_id>.pub`. Loader fail-fast sur clé corrompue. 5 tests (roundtrip priv/pub, scan dir + filtrage `.pub`, fichier manquant, PEM malformé).
- **2026-04-18 (nuit)** — Feature 3.4 ✅ : endpoint `/v1/push` sur echo-server. Pipeline de vérif en 11 étapes (version, skew 60s, cross-check TLS CN ↔ payload machine_id, lookup clé publique, Ed25519 verify, nonce). Cache nonce TTL 5min, éviction 1min. 3 pushes consécutifs validés.
- **2026-04-18 (nuit)** — Fix d'ordre : vérif signature avant nonce cache (sinon un attaquant mTLS-valide mais sans signing key pouvait polluer le cache — DoS pré-authentification).
- **2026-04-18 (nuit)** — Feature 3.5 ✅ : agent passe de `Ping` à `Push` signé. Pipeline bout-en-bout Collect → ToProto → Sign Ed25519 → POST mTLS `/v1/push` → Verify + Accept. Décision design : une seule `pushLoop` globale (30s) qui collecte tous les modules actifs et push en batch — simplification volontaire du palier 3, le per-module scheduling arrive au palier 5/6. `Ping` conservé pour healthcheck/debug (`make test-echo` OK). Conversion `modules.Data → bientotv1.ModuleData` isolée dans `convert.go` pour découpler les modules du protobuf. 5 scénarios validés : push initial, ticks 30s, echo-server down (warn + agent up), recovery, /ping toujours fonctionnel.
- **2026-04-18 (nuit)** — Feature 3.6 ✅ : 7 tests de non-régression sécurité sur `/v1/push` (happy path + 6 rejets). Chaque invariant du handler couvert par un test Go automatisable en CI.
- **2026-04-18 (nuit)** — 🎉 **Palier 3 VALIDÉ**. Protocole push signé Ed25519 bout-en-bout, contract protobuf versionné, 0 dette de sécurité identifiée (ordre signature/nonce corrigé en cours de palier).
- **2026-04-19** — Refactor architecture : migration vers registry + config YAML. 4 commits atomiques (config loader, Factory pattern, registry, migration main). Ajout/toggle/désactivation de modules désormais possible via `agent.yaml` sans rebuild. Feature 4.1 définitivement validée (le travail existait avant, pas commité ; réincorporé proprement dans cet historique).
- **2026-04-19** — 🎉 **Premier tag release publié : v0.1.0**. Workflow GitHub Actions déclenché, GoReleaser a publié :
    - Release GitHub avec 4 archives multi-arch + checksums
    - Images Docker multi-arch sur GHCR (`ghcr.io/ldesfontaine/bientot-agent:0.1.0` et `bientot-dashboard:0.1.0`)
    - Changelog auto-généré depuis les Conventional Commits

    Déploiement désormais reproductible : `docker pull ghcr.io/ldesfontaine/bientot-agent:0.1.0` tire le binaire correct pour l'archi de la machine cible.
- **2026-04-19** — Feature 6.4 ✅ : workflow CI avec 3 jobs parallèles (test race, golangci-lint 11 linters, buf lint + buf breaking sur PR). SHAs pinnés pour sécu supply chain. Concurrency group pour auto-cancel des runs obsolètes. Premier run a remonté 3 findings (2 gosec G115 + 1 errorlint), tous résolus — G115 supprimés localement avec rationale (bornage par `maxPayloadSize = 1 MB`), errorlint fixé via `errors.Is(err, http.ErrServerClosed)` cohérent avec `server.go:92`.
- **2026-04-19** — Feature 4.2/4.3/4.4 ✅ rétrospectivement : parseur Prometheus text format (`internal/shared/promparse/`, 157+159 lignes), extraction 14 métriques système dans `internal/modules/system/extract.go`, tests unitaires module system (197 lignes). Commits `2a74903`, `e0cd589`, `98fe0d5`. Travail déjà en place avant cette entrée, journalisé a posteriori pour tracer le palier 4.
- **2026-04-19** — Feature 4.5 ✅ : revue de conformité + fix + validation e2e. (1) `Detect()` du module system faisait un `http.Get` live — il disabled le module au boot si node_exporter pas encore up. Corrigé en validation syntaxique pure (`url.Parse` + check scheme http/https + host non-vide). Runtime reachability relève de `Collect()` et retry au tick suivant. (2) Ajout `load_average_5m` + `load_average_15m` dans `extract.go` (plan d'origine, triviaux). (3) Tests Detect refactorisés : les 3 tests de reachability (`_Success`, `_Non200`, `_ServerDown`) remplacés par `_ValidURL` (URL unroutable OK) + `_InvalidURL` (no scheme, wrong scheme, missing host). Compteur bumped 14→16 dans `TestExtract_AllMetrics` et `TestModule_Collect_Real`. (4) Validation pipeline bout-en-bout : stack `examples/` tourne depuis ~1h, `bientot-dashboard-example` logue `push accepted modules=2 metrics=15` toutes les 30s sur deux agents (local-test-a, local-test-b). E2E post-fix (metrics=17) différé car port 8443 occupé par le stack examples.
- **2026-04-19** — 🎉 **Palier 4 VALIDÉ**. Module system scrape node_exporter, extrait 16 métriques normalisées (mémoire + swap + load 1/5/15 + CPU counters par mode + filesystem root + uptime + cpu_cores), pipeline bout-en-bout fonctionnel sur deux agents simultanés. 1er module réel en prod, pipeline protobuf signé du palier 3 validé avec un vrai payload (15+ métriques avec labels).
- **2026-04-20** — Feature 5.0.1 ✅ : fondation SQLite. Package `internal/dashboard/storage/` avec `Open`/`Close`/`Ping`, schéma embarqué via `go:embed` (4 tables : `pushes`, `metrics`, `module_state`, `agents`). WAL mode + `foreign_keys=ON` + `MaxOpenConns=1` (single-writer SQLite). Driver pure Go via `modernc.org/sqlite` (pas de CGO). 8 tests unitaires (open/close, idempotence, schema applied, pragmas, invalid path). Bump Go 1.24 → 1.25 requis par `modernc.org/sqlite@v1.49.1`, propagé au `Dockerfile` (CI suit via `go-version-file: go.mod`). Commits `a6faa53` (bump) + `cc27d5b` (storage).
- **2026-04-20** — Feature 5.0.2 ✅ : `Storage.SavePush(ctx, req)` transactionnel. Insère push (raw protobuf canonique via `proto.MarshalOptions{Deterministic: true}`) + métriques en batch (prepared statement réutilisé) + upsert agent (`first_seen_at` figé, `last_push_at` mis à jour). Rollback complet sur erreur partielle. Labels stockés en JSON, `NULL` si vide. 12 tests : insertion, comptage, sémantique upsert, isolation entre agents, rollback sur duplicate nonce, edge cases (nil, modules vides), roundtrip raw_payload.
- **2026-04-20** — Feature 5.0.3 ✅ : 3 méthodes de lecture sur `Storage`. (1) `ListAgents(ctx)` → tous les agents triés par `machine_id`. (2) `GetLatestMetrics(ctx, machineID)` → dernière valeur connue par nom de métrique (auto-jointure avec `MAX(timestamp_ns) GROUP BY name`, dédoublonnage `HAVING m.id = MAX(m.id)`). (3) `GetMetricPoints(ctx, machineID, name, start, end)` → série temporelle ordonnée croissante sur intervalle semi-ouvert `[start, end)`. Toutes retournent slice/map vide (pas nil) pour API JSON-friendly. Types publics : `Agent`, `Metric`, `MetricPoint`. 13 tests.
- **2026-04-20** — Feature 5.0.4 ✅ : intégration handler `/v1/push` ↔ storage. `Server` gagne un `*storage.Storage` injecté ; `handlePush` appelle `SavePush` après toutes les validations (version, signature, machine_id, nonce) et avant la réponse 200 ; erreur SQL → 500 (l'agent retentera au prochain tick, le cache nonce reste protecteur). `cmd/dashboard` ouvre la DB via `BIENTOT_DB_PATH` (défaut `/data/dashboard.db`), close au shutdown. Volume Docker nommé `dashboard-data` mounté sur `/data`. Fix Dockerfile : `RUN mkdir -p /data && chown 1000:1000 /data` car le mountpoint d'un volume vide est créé en root (incompatible avec `user: 1000:1000` du compose dev). Tests handler refactorés : injection d'un `Storage` temporaire via `t.TempDir()` dans `testSetup`. Validation E2E : pushes persistent à travers `docker-down/up`, `first_seen_at` préservé, `last_push_at` mis à jour.
- **2026-04-20** — 🎉 **Sub-palier 5.0 VALIDÉ**. Pipeline data bout-en-bout fonctionnel : agent → push signé → mTLS + signature + nonce + skew → `SavePush` transactionnel → SQLite (pushes + metrics + agents) → requêtable via `ListAgents` / `GetLatestMetrics` / `GetMetricPoints`. 4 features (5.0.1 → 5.0.4), ~150 lignes prod + ~700 lignes tests, 33 tests storage + tests handler verts.
- **2026-04-20** — Feature 5.1.1 ✅ : deuxième serveur HTTP (API JSON clair sur `:8080`) à côté du serveur mTLS (`:8443`). Nouveau package `internal/dashboard/api/` (`server.go`, `router.go`, `json.go`, `health.go`). `http.ServeMux` Go 1.22+ (verbe HTTP dans le pattern → 405 auto). Middleware `withLogging` (method/path/status/duration). Helpers `writeJSON`/`writeError` avec format erreur unifié `{"error":"..."}` pour 4XX et 5XX. Endpoint `GET /api/health` (liveness uniquement). Seuil offline configurable via `OFFLINE_THRESHOLD_SECONDS` (default 120s) porté par `api.Config`. `cmd/dashboard` orchestre les 2 serveurs en parallèle via `errgroup.WithContext` (fail coordonné). Compose : expose `8080`, `DASHBOARD_WEB_ADDR`, `OFFLINE_THRESHOLD_SECONDS`. 3 tests (200/405/404), E2E validé avec curl.
- **2026-04-20** — Feature 5.1.2 ✅ : endpoint `GET /api/agents`. DTO `agentDTO` (camelCase, timestamps en ms Unix) découplé de `storage.Agent`. Statut `online`/`offline` calculé via `toDTO(agent, now, threshold)` — fonction libre, testable sans HTTP. Inégalité stricte `now.Sub(lastPush) > threshold` pour éviter les flip-flops pile à la frontière. Slice pré-alloué `make([]agentDTO, 0, N)` garantit `[]` (pas `null`) même si vide. 9 nouveaux tests (empty array, single online, sorted, content-type, toDTO online/offline/boundary/ms, offline via injection de threshold négatif). Total package `api` : 12 PASS. E2E validé cycle complet : online → stop 130s → offline → start → online.
- **Dette légère palier 6** — Injecter une `Clock` interface dans `api.Server` pour tester proprement la logique temps-dépendante (statut offline, graph range) sans recourir à l'astuce du threshold négatif.
- **2026-04-20** — Feature 5.1.3 ✅ : endpoint `GET /api/agents/{id}/metrics`. Ajout `Storage.AgentExists(ctx, id)` (key lookup, réutilisable en 5.1.4). Handler : `PathValue` → `AgentExists` (404 sinon) → `GetLatestMetrics` → `sort.Slice` par `name` (ordre déterministe) → JSON. DTO `metricDTO` (name, value, module, labels, timestamp ms). Labels jamais `null` — forcé à `{}` dans `toMetricDTO`. Helper `timestampNsToMillis` partagé. 10 tests API (404, sans métriques, tri, dernière valeur, labels jamais null au niveau wire via `json.RawMessage`, ms, 3 unit `toMetricDTO`, ordering déterministe sur 10 itérations) + 3 tests storage (`AgentExists` NotFound/Found/CaseSensitive). Total : 36 storage + 22 api. E2E : 17 métriques triées, 404 sur agent inconnu.
- **2026-04-20** — Feature 5.1.4 ✅ : endpoint `GET /api/agents/{id}/metric-points` (séries temporelles pour graphes uPlot). Envelope `{name, machineId, start, end, points}` avec `points: [{t, v}]` compact (~50% gain de payload vs verbose). `parseTimeRange` extrait en fonction libre — précédence `absolute (start+end ms) > relative (range=1h) > default 24h`. Validation stricte : `start`-or-`end` partiel rejeté, `start ≥ end` rejeté, durée négative rejetée, parser limité aux unités `time.ParseDuration` (`h`, `m`, `s` — pas `d`/`w`, dette palier 5/6). Garde-fou `maxPointsPerResponse=10000` pour MVP avant downsampling. 14 tests (4 validation HTTP, 6 happy-path incluant absolute mode et default range, 4 unit `parseTimeRange` dont 6 sous-cas d'erreurs). Route wired (1 ligne). E2E : 113 points/1h sur data réelle, DB count == API count (cohérence pipeline complète).
- **2026-04-20** — 🎉 **Sub-palier 5.1 VALIDÉ**. API JSON read-only complète : 4 endpoints (`/api/health`, `/api/agents`, `/api/agents/{id}/metrics`, `/api/agents/{id}/metric-points`), tous derrière le serveur HTTP `:8080` séparé du mTLS `:8443`. Conventions homogènes : ms timestamps, camelCase, `[]` jamais `null`, format erreur unifié `{"error":"..."}`, statut online/offline calculé via `Clock`-comparable threshold, agent inexistant → 404, deux modes range (relatif + absolu). 36 PASS api + 36 PASS storage + tests handler dashboard verts. E2E validé bout-en-bout sur stack live. Prêt pour 5.2 (templates HTML + HTMX) qui consommera ces endpoints.
- **Dette palier 5/6** — Parser de `range` API limité à `time.ParseDuration` (`h`, `m`, `s`). Étendre pour supporter `d` (days) et `w` (weeks) via parser custom dès qu'un user (humain) en aura le besoin.
- **2026-04-20** — Feature 5.2.1 ✅ : refactor api + setup web + assets. (1) `api.Server` → `api.Router` ; suppression `Run`/lifecycle ; extraction `BuildHandler()` ; `router.go` fusionné dans `server.go` ; receivers `s *Server` → `r *Router` + param HTTP `r` → `req` (anti-shadow). (2) Nouveau package `internal/dashboard/web/` à plat (mirror de `api/`) : `Router`, `Config`, `NewRouter`, `BuildHandler` avec `embed.FS` pour les assets. Endpoints `GET /{$}` (placeholder) + `GET /static/*` via `http.FileServer` sur sub-FS. (3) Assets embeddés : `htmx.min.js` (~50KB), `Geist-Variable.woff2` + `GeistMono-Variable.woff2` (~70KB chacun, magic `wOF2` validé). (4) `cmd/dashboard/main.go` : un seul `*http.Server` mount `/api/` (avant `/`) et `/` ; `errgroup` 3 goroutines (mtls + http listen + http shutdown coordonné). 4 nouveaux tests web (200, content-type, static served, 404). E2E : `/api/health` JSON, `/` text, `/static/htmx.min.js` 200/50917 bytes, `firstSeenAt` persiste à travers restart.

*(Chaque feature validée ajoute une entrée ici avec la date et un résumé d'une ligne.)*

## ⚠️ Dette technique (à traiter aux paliers indiqués)

- **Palier 6** : `Agent.Run()` ne `WaitGroup`-pas ses goroutines au shutdown → le log `"module stopped"` est perdu parce que le process exit avant que `runModule` flush son cleanup. Ajout d'un `sync.WaitGroup` prévu.
- **Palier 2 ou ultérieur** : `cmd/agent/main.go` utilise encore la goroutine manuelle de signal au lieu de `signal.NotifyContext` (idiomatique Go 1.16+). Refactor trivial à faire en passant.
- **Palier 2.3** — UID mismatch host↔container sur bind-mount : pattern `user: "${HOST_UID:-1000}:${HOST_GID:-1000}"` en dev, Docker secrets ou image dédiée avec UID 10001 natif en prod.
- **Palier 2.3** — La couche mTLS `RequireAndVerifyClientCert` garantit qu'aucune requête non-authentifiée n'atteint le code applicatif. Tout rejet se fait au handshake. Logs sécurité à brancher sur alerting au palier 7.
- **Palier 6** — La `pushLoop` agent n'a pas de backoff exponentiel ni de circuit breaker : quand le dashboard est down, l'agent spamme un warn toutes les 30s. À implémenter avec `cenkalti/backoff` ou équivalent.
- **Palier 10** — `client.ToProto` ignore silencieusement les `RawEvents.Fields` non-string (le proto est `map<string,string>` pour l'instant). Quand un module CTI enverra des fields typés (int, map, bool), ajouter un log warn ou sérialiser en JSON.
- **Palier 7** — Aucun rate limiting côté echo-server/dashboard : un client authentifié peut saturer le serveur. `chi/middleware/httprate` dès que le vrai dashboard prend le relais.
- **Palier 6** — Certs 24h nécessitent régénération manuelle en dev. Renouvellement auto via step toolkit à implémenter.
- **Palier 6** — Les tests mTLS utilisent les certs `deploy/certs/` via paths relatifs (`../../../`). Refactor en fixtures embarquées (`go:embed`) au palier 6.
- **Palier 6** — `AGENTS=("vps")` en dur dans `bootstrap-ca.sh` : extraire dans un fichier de config (`deploy/agents.yaml`) ou ajouter un `scripts/add-agent.sh <name>` qui génère juste les certs/clés d'un nouvel agent sans régénérer les existants.
- **Palier 6** — Logs stdlib "TLS handshake error EOF" pendant les tests viennent de la sonde `net.Dial` d'attente du port. Fix possible via `http.Server.ErrorLog = io.Discard` ou sonde `tls.Dial`. Bruit cosmétique, pas bloquant.
- **Post-6.2 / futur** — GoReleaser v2 affiche un deprecation warning pour `dockers` et `docker_manifests` (remplacés à terme par `dockers_v2`). Migration à faire quand GoReleaser 3.x publiera un guide officiel, pas avant. Syntaxe actuelle pleinement supportée.
- **Palier 5** — `system.cpu_temperature_celsius` : jointure `node_hwmon_sensor_label` + `node_hwmon_temp_celsius` + filtre sensors fiables (coretemp, nct*, k10temp ; exclure NVMe qui reporte 128°C quand sensor offline).
- **Palier 5** — `system.disk_*` multi-mountpoint : actuellement filtré en dur sur `mountpoint="/"`. Étendre avec whitelist `fstype` (ext4/xfs/btrfs/zfs) et blacklist mountpoints virtuels (tmpfs, overlayfs, procfs). Configuration exposée dans `agent.yaml`.
- **Palier 6** — Unifier la config agent : YAML avec overrides env. Actuellement `DASHBOARD_URL` est env-only, les configs modules sont YAML-only. Pattern cible : YAML par défaut, `BIENTOT_*` env override pour le déploiement (12-factor compatible).
- **Palier 6/7** — Stack `examples/` et `deploy/compose.dev.yml` utilisent toutes deux le port `8443` et sont mutuellement exclusives. Paramétrer le port via env var ou documenter explicitement le pattern de bascule.
- **Palier 6** — `Dockerfile` (target `dashboard`) figé sur `chown 1000:1000 /data` pour le mountpoint du volume `dashboard-data`. Cohérent avec `user: ${HOST_UID:-1000}` du compose dev mais pas portable si `HOST_UID != 1000`. Solutions à explorer en prod : init-container qui chown au démarrage, image dédiée à l'UID 10001 natif sans override, ou `chmod 1777` (sticky bit) pour autoriser tous les UIDs.

## 📝 Conventions

### Workflow de développement
- **Feature-par-feature, pas fichier-par-fichier** : on groupe les fichiers par unité logique qui apporte un résultat observable (build qui marche, binaire qui tourne, etc.).
- **Chaque feature = un commit atomique** avec message en Conventional Commits.
- **Tout est codé à la main** : zéro copier-coller de code généré. Claude explique le "pourquoi" et le "quoi", l'humain code.
- **Validation par Claude avant commit** : on colle le code dans le chat, review, ajustements, puis commit.

### Commits
[Conventional Commits](https://www.conventionalcommits.org/) avec scope :
- `feat(agent): add heartbeat module`
- `fix(dashboard): handle nil pointer in push handler`
- `docs(refonte): update roadmap after palier 0`
- `chore(ci): pin actions to SHA`

### Code Go
- `gofmt` + `goimports` obligatoires (le CI vérifiera)
- Nom de package en minuscules, sans underscore
- Erreurs : `fmt.Errorf("contexte: %w", err)` pour préserver la chaîne
- Logs : `slog` uniquement, jamais `fmt.Println`
- Contextes propagés partout (`ctx context.Context` en premier paramètre)

### Tests
- Un test par bug découvert (pas de bug qui ne génère pas un test de non-régression)
- Tests de protocole (sign/verify, nonce, mTLS) : obligatoires dès leur introduction
- Tests de modules : au minimum un golden test par module à partir du palier 5

### Sécurité
- `hmac.Equal` ou `subtle.ConstantTimeCompare` pour toute comparaison de secret
- Jamais de secret en query string
- Jamais de `panic()` dans le code principal (recovers explicites aux bornes)

## 📚 Glossaire

- **Agent** : binaire déployé par machine surveillée, collecte les métriques localement et les push au dashboard
- **Dashboard** : serveur central (front + API + storage + corrélation). Il y en a un seul.
- **Module** : unité de collecte côté agent (system, docker, traefik…). Implémente l'interface `modules.Module`.
- **Push** : envoi périodique des métriques d'un agent au dashboard
- **Software inventory** : liste des noms/versions des logiciels détectés par les modules, utilisée pour matcher les CVE
- **Raw event** : événement brut remonté par un module (ligne de log Traefik, décision CrowdSec…) utilisé par la CTI
- **Corrélation** : pipeline côté dashboard qui transforme les pushes bruts en signaux exploitables (alertes, enrichissement, matching CVE)
- **CA interne** : step-ca déployé dans l'infra, émet les certs de tous les nœuds
- **Feature** : unité logique de dev qui produit un résultat observable. Plusieurs fichiers, un commit.
- **Palier** : regroupement de features qui forme une étape majeure démontrable.

## 🔗 Ressources

- [Code ancien (branche main)](https://github.com/ldesfontaine/bientot/tree/main) — référence pour ne pas reproduire les erreurs
- [Connect-go](https://connectrpc.com/)
- [step-ca](https://smallstep.com/docs/step-ca/)
- [Buf](https://buf.build/docs/introduction)
````