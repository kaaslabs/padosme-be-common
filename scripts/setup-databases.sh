#!/usr/bin/env bash
# setup-databases.sh — Create ALL Padosme databases and run ALL migrations
#
# This is the single source of truth for the entire Padosme PostgreSQL plumbing.
# Run after a fresh Postgres deploy, data directory wipe, or cluster rebuild.
#
# What it does:
#   1. Creates all 11 PostgreSQL databases (idempotent — skips if exists)
#   2. Runs each service's SQL migrations in dependency order
#   3. Tracks applied migrations to avoid re-running (idempotent)
#
# Usage:
#   ./scripts/setup-databases.sh [--dry-run] [--skip-migrations] [--force-migrations]
#
# Environment variables (all optional):
#   PGHOST        default: localhost
#   PGPORT        default: 5434
#   PGUSER        default: padosme
#   PGPASSWORD    default: kaaslabs123@dev
#
# Production (/opt/padosme):
#   Each service's migrations/ folder is expected at:
#     /opt/padosme/<service-name>/migrations/
#
# Last audited: 2026-03-19
# Databases: 11 PostgreSQL + 1 MongoDB (catalogue — not managed here)
#
# NOTE: padosme-catalogue-service uses MongoDB (catalog_db), not PostgreSQL.
#       mobile-sms-service has no database (pure RMQ consumer).
#       MongoDB and its collections are NOT managed by this script.

set -euo pipefail

# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------
PGHOST="${PGHOST:-localhost}"
PGPORT="${PGPORT:-5434}"
PGUSER="${PGUSER:-padosme}"
PGPASSWORD="${PGPASSWORD:-kaaslabs123@dev}"
export PGPASSWORD

DRY_RUN=false
SKIP_MIGRATIONS=false
FORCE_MIGRATIONS=false

# Base path where services are deployed
# In production: /opt/padosme
# In development: auto-detected from script location
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -d "/opt/padosme/auth-service" ]]; then
  BASE_PATH="/opt/padosme"
else
  # Dev mode: go up from padosme-be-common/scripts/ to the monorepo root
  BASE_PATH="$(cd "${SCRIPT_DIR}/../.." && pwd)"
fi

for arg in "$@"; do
  case $arg in
    --dry-run)           DRY_RUN=true ;;
    --skip-migrations)   SKIP_MIGRATIONS=true ;;
    --force-migrations)  FORCE_MIGRATIONS=true ;;
    *) echo "Unknown argument: $arg"; exit 1 ;;
  esac
done

ERRORS=0

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'
DIM='\033[2m'

info()    { echo -e "${CYAN}[INFO]${RESET}  $*"; }
ok()      { echo -e "${GREEN}[OK]${RESET}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${RESET}  $*"; }
dry()     { echo -e "${YELLOW}[DRY]${RESET}   $*"; }
err()     { echo -e "${RED}[ERROR]${RESET} $*" >&2; }
section() { echo ""; echo -e "${BOLD}$*${RESET}"; }

# ---------------------------------------------------------------------------
# Database operations
# ---------------------------------------------------------------------------

# Create database if it doesn't exist
create_database() {
  local db_name="$1"

  if $DRY_RUN; then
    dry "Database  ${BOLD}${db_name}${RESET}"
    return
  fi

  # Check if database exists
  local exists
  exists=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d postgres -tAc \
    "SELECT 1 FROM pg_database WHERE datname='${db_name}'" 2>/dev/null || echo "")

  if [[ "$exists" == "1" ]]; then
    ok "Database  ${BOLD}${db_name}${RESET}  (already exists)"
  else
    if psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d postgres -c \
      "CREATE DATABASE ${db_name};" >/dev/null 2>&1; then
      ok "Database  ${BOLD}${db_name}${RESET}  (created)"
    else
      err "Database  ${db_name}  -- CREATE failed"
      ERRORS=$((ERRORS + 1))
    fi
  fi
}

# Ensure the migration tracking table exists in a database
ensure_migration_table() {
  local db_name="$1"

  psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$db_name" -q <<'SQL' 2>/dev/null
CREATE TABLE IF NOT EXISTS _applied_migrations (
  filename    TEXT PRIMARY KEY,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
SQL
}

# Check if a migration has already been applied
is_migration_applied() {
  local db_name="$1"
  local filename="$2"

  local result
  result=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$db_name" -tAc \
    "SELECT 1 FROM _applied_migrations WHERE filename='${filename}'" 2>/dev/null || echo "")

  [[ "$result" == "1" ]]
}

# Run a single SQL migration file against a database
run_migration() {
  local db_name="$1"
  local sql_file="$2"
  local filename
  filename=$(basename "$sql_file")

  if $DRY_RUN; then
    dry "  Migrate  ${BOLD}${filename}${RESET}  -> ${db_name}"
    return
  fi

  # Skip if already applied (unless --force-migrations)
  if ! $FORCE_MIGRATIONS && is_migration_applied "$db_name" "$filename"; then
    ok "  Migrate  ${BOLD}${filename}${RESET}  -> ${db_name}  (already applied)"
    return
  fi

  if psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$db_name" \
    -v ON_ERROR_STOP=1 -f "$sql_file" >/dev/null 2>&1; then
    # Record the migration
    psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$db_name" -q -c \
      "INSERT INTO _applied_migrations (filename) VALUES ('${filename}') ON CONFLICT DO NOTHING;" 2>/dev/null
    ok "  Migrate  ${BOLD}${filename}${RESET}  -> ${db_name}"
  else
    err "  Migrate  ${filename}  -> ${db_name}  -- FAILED"
    ERRORS=$((ERRORS + 1))
  fi
}

# Run all migrations for a service (sorted by filename for correct ordering)
run_service_migrations() {
  local db_name="$1"
  local service_dir="$2"    # directory name under BASE_PATH
  local migrations_dir

  # Resolve the migrations directory
  # Production layout:  /opt/padosme/auth-service/migrations/
  # Dev layout:         /home/.../kaaslabs/padosme-auth-service/migrations/
  if [[ -d "${BASE_PATH}/${service_dir}/migrations" ]]; then
    migrations_dir="${BASE_PATH}/${service_dir}/migrations"
  else
    warn "  No migrations dir found for ${service_dir} (looked in ${BASE_PATH}/${service_dir}/migrations)"
    return
  fi

  # Find .sql files (exclude .down.sql rollback files), sorted by name
  local sql_files
  sql_files=$(find "$migrations_dir" -maxdepth 1 -name "*.sql" ! -name "*.down.sql" | sort)

  if [[ -z "$sql_files" ]]; then
    warn "  No SQL files found in ${migrations_dir}"
    return
  fi

  if ! $DRY_RUN; then
    ensure_migration_table "$db_name"
  fi

  while IFS= read -r sql_file; do
    run_migration "$db_name" "$sql_file"
  done <<< "$sql_files"
}

# ---------------------------------------------------------------------------
# Wait for PostgreSQL
# ---------------------------------------------------------------------------
wait_for_postgres() {
  local max_attempts=30
  local attempt=0

  info "Waiting for PostgreSQL at ${PGHOST}:${PGPORT} ..."
  until psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d postgres -c "SELECT 1" >/dev/null 2>&1; do
    attempt=$((attempt + 1))
    if [[ $attempt -ge $max_attempts ]]; then
      err "PostgreSQL not reachable after ${max_attempts} attempts."
      err "  Host: ${PGHOST}:${PGPORT}"
      err "  User: ${PGUSER}"
      exit 1
    fi
    echo -n "."
    sleep 2
  done
  echo ""
  ok "PostgreSQL is ready"
}

# =========================================================================
#                            M A I N
# =========================================================================
echo ""
echo -e "${BOLD}Padosme PostgreSQL Full Database Setup${RESET}"
echo    "================================================================="
echo    "  Host      : ${PGHOST}:${PGPORT}"
echo    "  User      : ${PGUSER}"
echo    "  Base path : ${BASE_PATH}"
$DRY_RUN && echo -e "  Mode      : ${YELLOW}DRY RUN -- no changes will be made${RESET}"
$SKIP_MIGRATIONS && echo -e "  Migrations: ${YELLOW}SKIPPED${RESET}"
$FORCE_MIGRATIONS && echo -e "  Migrations: ${YELLOW}FORCE -- re-running all${RESET}"
echo    "================================================================="
echo ""

if ! $DRY_RUN; then
  wait_for_postgres
fi

# =========================================================================
# 1. CREATE DATABASES
# =========================================================================
# All 11 PostgreSQL databases in the Padosme ecosystem.
#
# Service-to-database mapping:
#   padosme-auth-service          -> padosme_auth
#   padosme-user-profile-service  -> padosme_profiles
#   padosme-seller-service        -> padosme_sellers
#   padosme-notification-service  -> padosme_notifications
#   padosme-rating-service        -> rating_db
#   padosme-analytics-service     -> analytics_db
#   padosme-channel-service       -> channel_service
#   padosme-wallet-service        -> wallet_service
#   padosme-subscription-service  -> subscription_service
#   padosme-payment-service       -> payment_service
#   padosme-coupon-service        -> padosme_coupon
#   padosme-discovery-service     -> discovery
#   ledgers-cloud-connect-service -> padosme_ledgers
#
# NOT managed here:
#   padosme-catalogue-service     -> MongoDB: catalog_db
#   mobile-sms-service            -> no database
# =========================================================================

section "1. CREATE DATABASES"

# Core services (original 3 from init-scripts)
create_database "padosme_auth"
create_database "padosme_profiles"
create_database "padosme_sellers"

# Messaging / notification
create_database "padosme_notifications"

# Marketplace
create_database "rating_db"
create_database "channel_service"
create_database "padosme_coupon"

# Financial
create_database "wallet_service"
create_database "subscription_service"
create_database "payment_service"

# Analytics / discovery
create_database "analytics_db"
create_database "discovery"

# External integrations
create_database "padosme_ledgers"

# =========================================================================
# 2. RUN MIGRATIONS (in dependency order)
# =========================================================================
# Order matters:
#   Layer 0: auth-service (no upstream dependency)
#   Layer 1: user-profile-service (depends on auth events)
#   Layer 2: seller-service (depends on profile events)
#   Layer 3: everything else (depends on seller/profile/auth)
#
# Directory name mapping:
#   Production (/opt/padosme):  auth-service, user-profile-service, etc.
#   Dev (monorepo):             padosme-auth-service, padosme-user-profile-service, etc.
# =========================================================================

if $SKIP_MIGRATIONS; then
  section "2. MIGRATIONS -- SKIPPED"
else
  # Detect directory naming convention (prod vs dev)
  if [[ -d "${BASE_PATH}/auth-service" ]]; then
    # Production layout: /opt/padosme/auth-service/
    AUTH_DIR="auth-service"
    PROFILE_DIR="user-profile-service"
    SELLER_DIR="seller-service"
    NOTIFICATION_DIR="padosme-notification-service"
    RATING_DIR="padosme-rating-service"
    CHANNEL_DIR="padosme-channel-service"
    COUPON_DIR="coupon-service"
    WALLET_DIR="wallet-service"
    SUBSCRIPTION_DIR="subscription-service"
    PAYMENT_DIR="payment-service"
    ANALYTICS_DIR="padosme-analytics-service"
    DISCOVERY_DIR="padosme-discovery-service"
    LEDGERS_DIR="ledgers-cloud-connect-service"
  else
    # Dev layout: /home/.../kaaslabs/padosme-auth-service/
    AUTH_DIR="padosme-auth-service"
    PROFILE_DIR="padosme-user-profile-service"
    SELLER_DIR="padosme-seller-service"
    NOTIFICATION_DIR="padosme-notification-service"
    RATING_DIR="padosme-rating-service"
    CHANNEL_DIR="padosme-channel-service"
    COUPON_DIR="padosme-coupon-service"
    WALLET_DIR="padosme-wallet-service"
    SUBSCRIPTION_DIR="padosme-subscription-service"
    PAYMENT_DIR="padosme-payment-service"
    ANALYTICS_DIR="padosme-analytics-service"
    DISCOVERY_DIR="padosme-discovery-service"
    LEDGERS_DIR="ledgers-cloud-connect-service"
  fi

  # --- Layer 0: Auth (no dependencies) ---
  section "2a. MIGRATIONS -- Layer 0: Auth"
  run_service_migrations "padosme_auth"          "$AUTH_DIR"

  # --- Layer 1: Profile (depends on auth events) ---
  section "2b. MIGRATIONS -- Layer 1: User Profile"
  run_service_migrations "padosme_profiles"       "$PROFILE_DIR"

  # --- Layer 2: Seller (depends on profile events) ---
  section "2c. MIGRATIONS -- Layer 2: Seller"
  run_service_migrations "padosme_sellers"        "$SELLER_DIR"

  # --- Layer 3: All other services (no inter-dependencies) ---
  section "2d. MIGRATIONS -- Layer 3: Remaining Services"

  info "Notification service"
  run_service_migrations "padosme_notifications"  "$NOTIFICATION_DIR"

  info "Rating service"
  run_service_migrations "rating_db"              "$RATING_DIR"

  info "Channel service"
  run_service_migrations "channel_service"        "$CHANNEL_DIR"

  info "Coupon service"
  run_service_migrations "padosme_coupon"          "$COUPON_DIR"

  info "Wallet service"
  run_service_migrations "wallet_service"          "$WALLET_DIR"

  info "Subscription service"
  run_service_migrations "subscription_service"    "$SUBSCRIPTION_DIR"

  info "Payment service"
  run_service_migrations "payment_service"         "$PAYMENT_DIR"

  info "Analytics service"
  run_service_migrations "analytics_db"            "$ANALYTICS_DIR"

  info "Discovery service"
  run_service_migrations "discovery"               "$DISCOVERY_DIR"

  info "Ledgers service"
  run_service_migrations "padosme_ledgers"         "$LEDGERS_DIR"
fi

# =========================================================================
# Summary
# =========================================================================
echo ""
echo "================================================================="
if [[ $ERRORS -gt 0 ]]; then
  err "${ERRORS} error(s) occurred. Check output above."
  echo ""
  exit 1
fi

if $DRY_RUN; then
  warn "Dry run complete -- nothing was created"
else
  ok "Setup complete -- all databases created and migrations applied"
fi

echo ""
echo -e "${DIM}Database summary:${RESET}"
echo    "  13 PostgreSQL databases (11 service DBs + postgres + template)"
echo    "  12 services with SQL migrations"
echo    "   1 service uses MongoDB (catalogue-service -> catalog_db)"
echo    "   1 service has no database (mobile-sms-service)"
echo ""
echo -e "${DIM}Production paths:${RESET}"
echo    "  Databases: psql -h ${PGHOST} -p ${PGPORT} -U ${PGUSER}"
echo    "  Migrations: /opt/padosme/<service>/migrations/*.sql"
echo    "  Tracking: SELECT * FROM _applied_migrations; (per database)"
echo ""
