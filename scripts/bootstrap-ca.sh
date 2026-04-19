#!/usr/bin/env bash
set -euo pipefail

# Lance le script depuis n'importe où : on se place à la racine du repo.
cd "$(dirname "$0")/.."

# ─── Configuration ──────────────────────────────────────────────
CERTS_DIR="./deploy/certs"
COMPOSE="docker compose -f deploy/compose.dev.yml --project-directory ."
PROVISIONER="bientot"

# 24h est la durée max autorisée par la policy par défaut de step-ca.
# Renouvellement auto arrive au palier 6 ; en attendant, relancer le script quotidiennement.
SERVER_CERT_TTL="24h"
CLIENT_CERT_TTL="24h"

# Agents à provisionner. Ajouter "pi" ici pour générer un cert d'agent pi.
AGENTS=("vps")

# ─── 1. Vérifier que la CA tourne ───────────────────────────────
if ! $COMPOSE ps step-ca | grep -q "Up"; then
    echo "ERROR: step-ca container is not running. Run 'make docker-up' first." >&2
    exit 1
fi

# ─── 2. Lire le password provisioner depuis .env ────────────────
if [ ! -f .env ]; then
    echo "ERROR: .env not found at repo root." >&2
    exit 1
fi

STEP_PASSWORD=$(grep "^STEP_CA_PASSWORD=" .env | cut -d= -f2- | tr -d '\n')
if [ -z "$STEP_PASSWORD" ]; then
    echo "ERROR: STEP_CA_PASSWORD is empty in .env." >&2
    exit 1
fi

# ─── 3. Nettoyer + recréer l'arborescence cible ─────────────────
rm -rf "$CERTS_DIR"
mkdir -p "$CERTS_DIR/ca" "$CERTS_DIR/dashboard" "$CERTS_DIR/dashboard/agent-keys"
for agent in "${AGENTS[@]}"; do
    mkdir -p "$CERTS_DIR/agent-$agent"
done

# ─── 4. Récupérer le root CA + intermediate ─────────────────────
# step-ca signe les leafs avec l'intermediate → clients ont besoin des deux pour vérifier.
echo ">>> Exporting root + intermediate CA certificates"
$COMPOSE exec -T step-ca cat /home/step/certs/root_ca.crt > "$CERTS_DIR/ca/root.crt"
$COMPOSE exec -T step-ca cat /home/step/certs/intermediate_ca.crt > "$CERTS_DIR/ca/intermediate.crt"
cat "$CERTS_DIR/ca/root.crt" "$CERTS_DIR/ca/intermediate.crt" > "$CERTS_DIR/ca/bundle.crt"

# ─── 5. Émettre le cert serveur du dashboard ────────────────────
echo ">>> Generating server cert for dashboard"
$COMPOSE exec -T step-ca \
    sh -c "echo '$STEP_PASSWORD' | step ca certificate dashboard \
        /tmp/server.crt /tmp/server.key \
        --provisioner $PROVISIONER \
        --provisioner-password-file /dev/stdin \
        --san dashboard --san localhost \
        --not-after $SERVER_CERT_TTL \
        --force"

$COMPOSE exec -T step-ca cat /tmp/server.crt > "$CERTS_DIR/dashboard/server.crt"
$COMPOSE exec -T step-ca cat /tmp/server.key > "$CERTS_DIR/dashboard/server.key"
$COMPOSE exec -T step-ca rm /tmp/server.crt /tmp/server.key

cp "$CERTS_DIR/ca/bundle.crt" "$CERTS_DIR/dashboard/ca-bundle.crt"
chmod 600 "$CERTS_DIR/dashboard/server.key"

# ─── 6. Émettre les certs clients des agents ────────────────────
for agent in "${AGENTS[@]}"; do
    echo ">>> Generating client cert for agent-$agent"

    $COMPOSE exec -T step-ca \
        sh -c "echo '$STEP_PASSWORD' | step ca certificate $agent \
            /tmp/client.crt /tmp/client.key \
            --provisioner $PROVISIONER \
            --provisioner-password-file /dev/stdin \
            --not-after $CLIENT_CERT_TTL \
            --force"

    $COMPOSE exec -T step-ca cat /tmp/client.crt > "$CERTS_DIR/agent-$agent/client.crt"
    $COMPOSE exec -T step-ca cat /tmp/client.key > "$CERTS_DIR/agent-$agent/client.key"
    $COMPOSE exec -T step-ca rm /tmp/client.crt /tmp/client.key

    cp "$CERTS_DIR/ca/bundle.crt" "$CERTS_DIR/agent-$agent/ca-bundle.crt"
    chmod 600 "$CERTS_DIR/agent-$agent/client.key"

    echo ">>> Generating Ed25519 signing key for agent-$agent"

    $COMPOSE exec -T step-ca \
        step crypto keypair --kty OKP --crv Ed25519 --force \
            --insecure --no-password \
            /tmp/signing.pub /tmp/signing.key

    $COMPOSE exec -T step-ca cat /tmp/signing.key > "$CERTS_DIR/agent-$agent/signing.key"
    $COMPOSE exec -T step-ca cat /tmp/signing.pub > "$CERTS_DIR/agent-$agent/signing.pub"
    $COMPOSE exec -T step-ca sh -c "rm -f /tmp/signing.key /tmp/signing.pub"

    cp "$CERTS_DIR/agent-$agent/signing.pub" "$CERTS_DIR/dashboard/agent-keys/$agent.pub"

    chmod 600 "$CERTS_DIR/agent-$agent/signing.key"
    chmod 644 "$CERTS_DIR/agent-$agent/signing.pub"
    chmod 644 "$CERTS_DIR/dashboard/agent-keys/$agent.pub"
done

# ─── 7. Récap ───────────────────────────────────────────────────
echo ""
echo "=== Certificates generated ==="
find "$CERTS_DIR" -type f | sort
echo ""
echo "=== Fingerprints & subjects ==="
openssl x509 -in "$CERTS_DIR/ca/root.crt" -noout -fingerprint -sha256 | sed 's/^/  root CA: /'
echo "--- dashboard ---"
openssl x509 -in "$CERTS_DIR/dashboard/server.crt" -noout -subject -issuer -dates
for agent in "${AGENTS[@]}"; do
    echo "--- agent-$agent ---"
    openssl x509 -in "$CERTS_DIR/agent-$agent/client.crt" -noout -subject -issuer -dates
done
echo "--- Signing keys ---"
for agent in "${AGENTS[@]}"; do
    echo "  agent-$agent: $(openssl pkey -in "$CERTS_DIR/agent-$agent/signing.key" -pubout 2>/dev/null | openssl dgst -sha256)"
done
