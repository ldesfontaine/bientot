#!/usr/bin/env bash
# examples/generate-test-certs.sh
# Génère la chaîne de certs + clés Ed25519 pour le déploiement local d'examples/.
# Variante de scripts/bootstrap-ca.sh scopée sur examples/ (step-ca dédié,
# certs dédiés, SANs adaptés au test local via host.docker.internal).
set -euo pipefail

cd "$(dirname "$0")/.."

# ─── Configuration ──────────────────────────────────────────────
CERTS_DIR="./examples/test-certs"
COMPOSE="docker compose -f examples/server/compose.yml --project-directory examples/server"
PROVISIONER="bientot"

SERVER_CERT_TTL="24h"
CLIENT_CERT_TTL="24h"

# Agents à provisionner en local. Ajouter "local-test-c" ici pour simuler
# un 3e agent — le pattern est générique.
AGENTS=("local-test-a" "local-test-b")

# ─── 1. Vérifier que la CA tourne ───────────────────────────────
if ! $COMPOSE ps step-ca | grep -q "Up\|running"; then
    echo "ERROR: step-ca container is not running." >&2
    echo "Run: $COMPOSE up -d step-ca" >&2
    exit 1
fi

# ─── 2. Lire le password provisioner depuis examples/server/.env ────
ENV_FILE="examples/server/.env"
if [ ! -f "$ENV_FILE" ]; then
    echo "ERROR: $ENV_FILE not found. Copy $ENV_FILE.example and fill it." >&2
    exit 1
fi

STEP_PASSWORD=$(grep "^STEP_CA_PASSWORD=" "$ENV_FILE" | cut -d= -f2- | tr -d '\n')
if [ -z "$STEP_PASSWORD" ]; then
    echo "ERROR: STEP_CA_PASSWORD is empty in $ENV_FILE." >&2
    exit 1
fi

# ─── 3. Nettoyer + recréer l'arborescence cible ─────────────────
rm -rf "$CERTS_DIR"
mkdir -p "$CERTS_DIR/ca" "$CERTS_DIR/dashboard" "$CERTS_DIR/dashboard/agent-keys"
for agent in "${AGENTS[@]}"; do
    mkdir -p "$CERTS_DIR/$agent"
done

# ─── 4. Récupérer le root CA + intermediate ─────────────────────
echo ">>> Exporting root + intermediate CA certificates"
$COMPOSE exec -T step-ca cat /home/step/certs/root_ca.crt > "$CERTS_DIR/ca/root.crt"
$COMPOSE exec -T step-ca cat /home/step/certs/intermediate_ca.crt > "$CERTS_DIR/ca/intermediate.crt"
cat "$CERTS_DIR/ca/root.crt" "$CERTS_DIR/ca/intermediate.crt" > "$CERTS_DIR/ca/bundle.crt"

# ─── 5. Émettre le cert serveur du dashboard ────────────────────
# SANs couvrent les hostnames que l'agent et les outils utilisent en local.
# En prod, régénérer avec le SAN du vrai hostname (mesh ou DNS public).
# Password passé via stdin (printf → docker exec -T → /dev/stdin) pour éviter
# l'injection shell quand le password contient ', $, `, espaces, etc.
echo ">>> Generating server cert for dashboard"
printf '%s' "$STEP_PASSWORD" | $COMPOSE exec -T step-ca \
    sh -c "step ca certificate dashboard \
        /tmp/server.crt /tmp/server.key \
        --provisioner $PROVISIONER \
        --provisioner-password-file /dev/stdin \
        --san dashboard \
        --san localhost \
        --san host.docker.internal \
        --not-after $SERVER_CERT_TTL \
        --force"

$COMPOSE exec -T step-ca cat /tmp/server.crt > "$CERTS_DIR/dashboard/server.crt"
$COMPOSE exec -T step-ca cat /tmp/server.key > "$CERTS_DIR/dashboard/server.key"
$COMPOSE exec -T step-ca rm /tmp/server.crt /tmp/server.key

cp "$CERTS_DIR/ca/bundle.crt" "$CERTS_DIR/dashboard/ca-bundle.crt"
chmod 600 "$CERTS_DIR/dashboard/server.key"

# ─── 6. Émettre les certs clients + clés Ed25519 des agents ─────
for agent in "${AGENTS[@]}"; do
    echo ">>> Generating client cert for $agent"

    printf '%s' "$STEP_PASSWORD" | $COMPOSE exec -T step-ca \
        sh -c "step ca certificate $agent \
            /tmp/client.crt /tmp/client.key \
            --provisioner $PROVISIONER \
            --provisioner-password-file /dev/stdin \
            --not-after $CLIENT_CERT_TTL \
            --force"

    $COMPOSE exec -T step-ca cat /tmp/client.crt > "$CERTS_DIR/$agent/client.crt"
    $COMPOSE exec -T step-ca cat /tmp/client.key > "$CERTS_DIR/$agent/client.key"
    $COMPOSE exec -T step-ca rm /tmp/client.crt /tmp/client.key

    cp "$CERTS_DIR/ca/bundle.crt" "$CERTS_DIR/$agent/ca-bundle.crt"
    chmod 600 "$CERTS_DIR/$agent/client.key"

    echo ">>> Generating Ed25519 signing key for $agent"

    $COMPOSE exec -T step-ca \
        step crypto keypair --kty OKP --crv Ed25519 --force \
            --insecure --no-password \
            /tmp/signing.pub /tmp/signing.key

    $COMPOSE exec -T step-ca cat /tmp/signing.key > "$CERTS_DIR/$agent/signing.key"
    $COMPOSE exec -T step-ca cat /tmp/signing.pub > "$CERTS_DIR/$agent/signing.pub"
    $COMPOSE exec -T step-ca sh -c "rm -f /tmp/signing.key /tmp/signing.pub"

    cp "$CERTS_DIR/$agent/signing.pub" "$CERTS_DIR/dashboard/agent-keys/$agent.pub"

    chmod 600 "$CERTS_DIR/$agent/signing.key"
    chmod 644 "$CERTS_DIR/$agent/signing.pub"
    chmod 644 "$CERTS_DIR/dashboard/agent-keys/$agent.pub"
done

# ─── 7. Préserver .gitkeep (le rm -rf du step 3 l'a supprimé) ───
touch "$CERTS_DIR/.gitkeep"

# ─── 8. Récap ───────────────────────────────────────────────────
echo ""
echo "=== Certificates generated ==="
find "$CERTS_DIR" -type f ! -name '.gitkeep' | sort
echo ""
echo "=== Fingerprints & subjects ==="
openssl x509 -in "$CERTS_DIR/ca/root.crt" -noout -fingerprint -sha256 | sed 's/^/  root CA: /'
echo "--- dashboard ---"
openssl x509 -in "$CERTS_DIR/dashboard/server.crt" -noout -subject -issuer -dates
for agent in "${AGENTS[@]}"; do
    echo "--- $agent ---"
    openssl x509 -in "$CERTS_DIR/$agent/client.crt" -noout -subject -issuer -dates
done
