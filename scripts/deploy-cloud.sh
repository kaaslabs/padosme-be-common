#!/usr/bin/env bash
set -euo pipefail

REF="${1:-main}"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

compose() {
  if docker compose version >/dev/null 2>&1; then
    docker compose -f docker-compose.yml -f docker-compose.prod.yml "$@"
  else
    docker-compose -f docker-compose.yml -f docker-compose.prod.yml "$@"
  fi
}

cd "$PROJECT_ROOT"

echo "[deploy] Repo: $PROJECT_ROOT"
echo "[deploy] Ref: $REF"

if [ -d .git ]; then
  git fetch --all --tags --prune

  if git rev-parse --verify "$REF" >/dev/null 2>&1; then
    git checkout "$REF"
  elif git ls-remote --heads origin "$REF" | grep -q "$REF"; then
    git checkout -B "$REF" "origin/$REF"
  else
    echo "[deploy] Ref '$REF' not found as local branch/commit or remote branch"
    exit 1
  fi

  if git show-ref --verify --quiet "refs/heads/$REF"; then
    git pull --ff-only origin "$REF"
  fi
fi

echo "[deploy] Pulling images"
compose pull

echo "[deploy] Starting/updating services"
compose up -d --remove-orphans

echo "[deploy] Running health checks"
./scripts/cloud-verify-observability.sh

echo "[deploy] Deployment complete"
compose ps
