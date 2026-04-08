#!/usr/bin/env bash
# ============================================================================
# setup-database.sh — Create all Padosme PostgreSQL databases and run migrations
#
# Usage:
#   ./setup-database.sh                              # defaults: localhost:5432 postgres/postgres
#   ./setup-database.sh -H db.padosme.app -P 5432 -u admin -p secret
#   ./setup-database.sh --databases-only             # skip migrations
#   ./setup-database.sh --migrate-only               # skip database creation
#
# Requires: psql (PostgreSQL client)
# ============================================================================
set -euo pipefail

# ── Defaults ────────────────────────────────────────────────────────────────
HOST="${DB_HOST:-localhost}"
PORT="${DB_PORT:-5432}"
USER="${DB_USER:-postgres}"
PASS="${DB_PASSWORD:-postgres}"
SSLMODE="${DB_SSLMODE:-disable}"
DATABASES_ONLY=false
MIGRATE_ONLY=false

# Base path where all service repos live (sibling of padosme-be-common)
REPOS_DIR="${REPOS_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}"

# ── CLI override ────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    -H) HOST="$2"; shift 2 ;;
    -P) PORT="$2"; shift 2 ;;
    -u) USER="$2"; shift 2 ;;
    -p) PASS="$2"; shift 2 ;;
    -S) SSLMODE="$2"; shift 2 ;;
    --databases-only) DATABASES_ONLY=true; shift ;;
    --migrate-only)   MIGRATE_ONLY=true; shift ;;
    *) echo "Usage: $0 [-H host] [-P port] [-u user] [-p pass] [-S sslmode] [--databases-only|--migrate-only]"; exit 1 ;;
  esac
done

export PGPASSWORD="$PASS"
PSQL="psql -h $HOST -p $PORT -U $USER"

echo "==> Padosme PostgreSQL setup"
echo "    Host: $HOST:$PORT  User: $USER  SSLMode: $SSLMODE"
echo "    Repos: $REPOS_DIR"
echo ""

# ============================================================================
# Database-to-service mapping
# ============================================================================
declare -A DB_SERVICE_MAP=(
  ["padosme_auth"]="padosme-auth-service"
  ["padosme_sellers"]="padosme-seller-service"
  ["padosme_profiles"]="padosme-user-profile-service"
  ["padosme_location"]="padosme-location-service"
  ["padosme_indexing"]="padosme-indexing-service"
  ["padosme_catalogue"]="padosme-catalogue-service"
  ["padosme_coupon"]="padosme-coupon-service"
  ["padosme_config"]="padosme-config-service"
  ["padosme_dictionary"]="padosme-dictionary-service"
  ["padosme_notifications"]="padosme-notification-service"
  ["padosme_ledgers"]="ledgers-cloud-connect-service"
  ["analytics_db"]="padosme-analytics-service"
  ["rating_db"]="padosme-rating-service"
  ["wallet_service"]="padosme-wallet-service"
  ["subscription_service"]="padosme-subscription-service"
  ["payment_service"]="padosme-payment-service"
  ["channel_service"]="padosme-channel-service"
  ["discovery"]="padosme-discovery-service"
)

# ============================================================================
# 1. CREATE DATABASES
# ============================================================================
create_databases() {
  echo "--- Creating databases ---"
  for db in "${!DB_SERVICE_MAP[@]}"; do
    exists=$($PSQL -tAc "SELECT 1 FROM pg_database WHERE datname='$db'" postgres 2>/dev/null || echo "")
    if [ "$exists" = "1" ]; then
      echo "  [skip] $db (already exists)"
    else
      echo "  [create] $db"
      $PSQL -c "CREATE DATABASE $db OWNER $USER;" postgres
    fi
  done

  # Enable common extensions in all databases
  echo ""
  echo "--- Enabling extensions ---"
  for db in "${!DB_SERVICE_MAP[@]}"; do
    echo "  $db: uuid-ossp"
    $PSQL -d "$db" -c 'CREATE EXTENSION IF NOT EXISTS "uuid-ossp";' 2>/dev/null || true
  done
  echo ""
}

# ============================================================================
# 2. RUN MIGRATIONS
# ============================================================================
run_migrations() {
  echo "--- Running migrations ---"

  for db in "${!DB_SERVICE_MAP[@]}"; do
    service="${DB_SERVICE_MAP[$db]}"
    migrations_dir="$REPOS_DIR/$service/migrations"

    if [ ! -d "$migrations_dir" ]; then
      echo "  [skip] $db ($service — no migrations/ directory)"
      continue
    fi

    # Count SQL files
    sql_count=$(find "$migrations_dir" -maxdepth 1 -name "*.sql" -o -name "*.up.sql" | wc -l | tr -d ' ')
    if [ "$sql_count" = "0" ]; then
      echo "  [skip] $db ($service — no .sql files)"
      continue
    fi

    echo "  [migrate] $db ($service — $sql_count files)"

    # Run .sql files in sorted order (schema first, then numbered migrations)
    # Exclude .down.sql files — only run .up.sql and plain .sql
    find "$migrations_dir" -maxdepth 1 \( -name "*.sql" -o -name "*.up.sql" \) \
      ! -name "*.down.sql" \
      | sort \
      | while read -r f; do
          fname=$(basename "$f")
          echo "    -> $fname"
          $PSQL -d "$db" -f "$f" 2>&1 | grep -v "^$" | sed 's/^/       /' || true
        done
  done
  echo ""
}

# ============================================================================
# 3. EXECUTE
# ============================================================================
if [ "$MIGRATE_ONLY" = false ]; then
  create_databases
fi

if [ "$DATABASES_ONLY" = false ]; then
  run_migrations
fi

echo "==> PostgreSQL setup complete."
echo "    Databases: ${#DB_SERVICE_MAP[@]}"
echo "    Run '$PSQL -l' to verify."
