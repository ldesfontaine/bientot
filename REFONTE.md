````markdown
# Bientôt v2 — Journal de refonte

> **Document de référence vivant.** Mis à jour à chaque feature/palier validé.
> Dernière mise à jour : **2026-04-18** — palier 3 validé.

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

## 📋 Paliers

| # | Nom | Statut | Résultat attendu |
|---|---|---|---|
| 0 | Squelette | ✅ VALIDÉ | `make build` + `docker-up` → logs "starting" des deux binaires |
| 1 | Agent autonome + interface Module | ✅ VALIDÉ | Module `heartbeat` détecté et collecté en boucle |
| 2 | mTLS bootstrap | ✅ VALIDÉ | Agent handshake mTLS vers echo-server, tamper cert → rejet |
| 3 | Protocole signé (protobuf + Ed25519) | ✅ VALIDÉ | PushRequest signée, tamper 1 byte → rejet |
| 4 | 1er module qui push (system) | ⬜ | Métriques CPU/RAM visibles côté dashboard de test |
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