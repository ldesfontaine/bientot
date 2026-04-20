# Local deployment example

Ce dossier simule un déploiement réel de Bientôt sur plusieurs machines,
en local via Docker. On utilise les images publiées sur GHCR (pas de build
depuis source) — même chemin que ce que tu feras sur ton VPS ou ton Pi.

## Topologie simulée

```
  examples/server/compose.yml            examples/agent/compose.yml
  ┌─────────────────────────┐            ┌──────────────────────────┐
  │ step-ca  (CA interne)   │            │ bientot-agent (local-test-a) │
  │ dashboard               │◄─ mTLS ───►│ node-exporter                │
  │   :8443 exposé sur host │   +sign    └──────────────────────────┘
  └─────────────────────────┘            ┌──────────────────────────┐
                                         │ bientot-agent (local-test-b) │
                                         │ node-exporter                │
                                         └──────────────────────────┘
```

Les agents tapent `https://host.docker.internal:8443` pour atteindre le
dashboard. Cela reproduit fidèlement le cas prod où agent et dashboard
sont sur deux machines distinctes (sur IP publique, réseau NetBird, ou
autre) — seule la config `dashboard.url` et les SANs du cert serveur
changeront.

## Ordre des opérations

```bash
# 1. Préparer .env (password CA, UID hôte)
cp examples/server/.env.example examples/server/.env
# Éditer examples/server/.env → STEP_CA_PASSWORD=$(openssl rand -base64 32)

# 2. Démarrer UNIQUEMENT step-ca (le dashboard a besoin de certs avant de démarrer)
docker compose -f examples/server/compose.yml --project-directory examples/server up -d step-ca

# 3. Générer les certs + clés Ed25519 pour dashboard + agents
./examples/generate-test-certs.sh

# 4. Démarrer le dashboard
docker compose -f examples/server/compose.yml --project-directory examples/server up -d dashboard

# 5. Démarrer agent local-test-a (premier terminal ou background)
MACHINE_ID=local-test-a docker compose -f examples/agent/compose.yml --project-directory examples/agent -p agent-a up

# 6. Démarrer agent local-test-b (deuxième terminal)
MACHINE_ID=local-test-b docker compose -f examples/agent/compose.yml --project-directory examples/agent -p agent-b up
```

Observer côté dashboard : `docker logs bientot-dashboard-example -f` doit
montrer des `push accepted` avec les machine_id `local-test-a` et `local-test-b`.

## Arrêter + nettoyer

```bash
docker compose -f examples/agent/compose.yml --project-directory examples/agent -p agent-a down
docker compose -f examples/agent/compose.yml --project-directory examples/agent -p agent-b down
docker compose -f examples/server/compose.yml --project-directory examples/server down -v
rm -rf examples/test-certs/*
```

## Règle importante : un seul dashboard local à la fois

Le dashboard expose `:8443` sur l'hôte. Si `deploy/compose.dev.yml` tourne
déjà, il occupe ce port et le dashboard example refusera de démarrer.
Arrête le dev compose avant :

```bash
docker compose -f deploy/compose.dev.yml down
```

## Adapter pour prod

Pour passer de ce test local à un vrai déploiement sur VPS/Pi :

1. Dans `agent.yaml` : remplacer `https://host.docker.internal:8443` par
   l'URL réelle de ton dashboard (ex: `https://bientot-dashboard:8443` si
   via NetBird, ou `https://dashboard.tondomaine.com:8443` si via DNS
   public).
2. Régénérer le cert serveur du dashboard avec le bon SAN (ex:
   `bientot-dashboard` pour le mesh, `dashboard.tondomaine.com` pour DNS
   public — cf. `scripts/bootstrap-ca.sh`).
3. Copier le contenu de `examples/agent/` sur chaque machine agent, avec
   son cert client et sa clé Ed25519 dédiés.
