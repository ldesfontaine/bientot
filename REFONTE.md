````markdown
# Bientôt v2 — Journal de refonte

> **Document de référence vivant.** Mis à jour à chaque feature/palier validé.
> Dernière mise à jour : **2026-04-18** — palier 0 redécoupé en features.

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
| 0 | Squelette | 🟡 EN COURS | `make build` + `docker-up` → logs "starting" des deux binaires |
| 1 | Agent autonome + interface Module | ⬜ | Module `heartbeat` détecté et collecté en boucle |
| 2 | mTLS bootstrap | ⬜ | Agent handshake mTLS vers echo-server de test, tamper cert → rejet |
| 3 | Protocole signé (protobuf + Ed25519) | ⬜ | PushRequest signée, tamper 1 byte → rejet au serveur |
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

#### Feature 0.3 — Les deux binaires qui tournent en Docker ⬜
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

*(Chaque feature validée ajoute une entrée ici avec la date et un résumé d'une ligne.)*

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