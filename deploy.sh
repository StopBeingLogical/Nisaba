#!/usr/bin/env bash
# deploy.sh — rebuild and restart the NISABA container
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "==> Stopping existing container (if any)..."
sudo docker rm -f nisaba 2>/dev/null || true

echo "==> Rebuilding and restarting NISABA..."
sudo docker compose up --build -d

echo "==> Done. Container logs:"
sudo docker compose logs --tail=20
