#!/usr/bin/env bash
# Bootstrap tvcentras origin server. Idempotent — safe to re-run.
# Expects: WGMESH_SECRET exported in environment.
# Files expected in /tmp: compose.yml, Caddyfile
set -euo pipefail

DEPLOY_DIR="/opt/tvcentras"

echo "=== tvcentras origin setup ==="

# ── Docker ──
if ! command -v docker &>/dev/null; then
    echo "Installing Docker..."
    curl -fsSL https://get.docker.com | sh
    systemctl enable docker
    systemctl start docker
    echo "Docker: $(docker --version)"
else
    echo "Docker: $(docker --version)"
fi

# ── Deploy directory ──
mkdir -p "$DEPLOY_DIR"

# ── Copy files ──
cp /tmp/compose.yml  "$DEPLOY_DIR/compose.yml"
cp /tmp/Caddyfile    "$DEPLOY_DIR/Caddyfile"

# ── Write .env ──
cat > "$DEPLOY_DIR/.env" <<EOF
WGMESH_SECRET=${WGMESH_SECRET:-}
EOF
chmod 600 "$DEPLOY_DIR/.env"

# ── Pull images ──
echo "Pulling images..."
docker compose \
    -f "$DEPLOY_DIR/compose.yml" \
    --project-directory "$DEPLOY_DIR" \
    --env-file "$DEPLOY_DIR/.env" \
    pull --quiet 2>/dev/null || true

# ── Start stack ──
echo "Starting tvcentras stack..."
docker compose \
    -f "$DEPLOY_DIR/compose.yml" \
    --project-directory "$DEPLOY_DIR" \
    --env-file "$DEPLOY_DIR/.env" \
    up -d

echo ""
echo "=== setup complete ==="
docker compose -f "$DEPLOY_DIR/compose.yml" --project-directory "$DEPLOY_DIR" ps
