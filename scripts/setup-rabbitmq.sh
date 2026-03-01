#!/usr/bin/env bash
# setup-rabbitmq.sh — Declare all Padosme RabbitMQ exchanges, queues, and bindings
# Uses the RabbitMQ Management HTTP API (port 15672).
#
# Usage:
#   ./scripts/setup-rabbitmq.sh [--dry-run]
#
# Environment variables (all optional, defaults match docker-compose.yml):
#   RABBITMQ_HOST        default: localhost
#   RABBITMQ_MGMT_PORT   default: 15672
#   RABBITMQ_USER        default: cto
#   RABBITMQ_PASSWORD    default: kaaslabs123
#   RABBITMQ_VHOST       default: /

set -euo pipefail

# ---------------------------------------------------------------------------
# Config — env vars with docker-compose defaults
# ---------------------------------------------------------------------------
HOST="${RABBITMQ_HOST:-localhost}"
PORT="${RABBITMQ_MGMT_PORT:-15672}"
USER="${RABBITMQ_USER:-cto}"
PASS="${RABBITMQ_PASSWORD:-kaaslabs123}"
VHOST="${RABBITMQ_VHOST:-/}"
DRY_RUN=false

for arg in "$@"; do
  case $arg in
    --dry-run) DRY_RUN=true ;;
    *) echo "Unknown argument: $arg"; exit 1 ;;
  esac
done

BASE_URL="http://${HOST}:${PORT}/api"
# URL-encode vhost: "/" → "%2F"
VHOST_ENC="${VHOST//\//%2F}"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

info()    { echo -e "${CYAN}[INFO]${RESET}  $*"; }
ok()      { echo -e "${GREEN}[OK]${RESET}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${RESET}  $*"; }
dry()     { echo -e "${YELLOW}[DRY]${RESET}   $*"; }
err()     { echo -e "${RED}[ERROR]${RESET} $*" >&2; }

# PUT /api/exchanges/{vhost}/{name}
declare_exchange() {
  local name="$1"
  local body
  body=$(printf '{"type":"topic","durable":true,"auto_delete":false,"internal":false}')

  if $DRY_RUN; then
    dry "Exchange  ${BOLD}${name}${RESET}  (topic, durable)"
    return
  fi

  local http_code
  http_code=$(curl -s -o /dev/null -w "%{http_code}" \
    -u "${USER}:${PASS}" \
    -X PUT \
    -H "Content-Type: application/json" \
    -d "$body" \
    "${BASE_URL}/exchanges/${VHOST_ENC}/${name}")

  case $http_code in
    200|201|204) ok "Exchange  ${BOLD}${name}${RESET}  (topic, durable)" ;;
    *) err "Exchange  ${name}  → HTTP ${http_code}"; return 1 ;;
  esac
}

# PUT /api/queues/{vhost}/{name}
declare_queue() {
  local name="$1"
  local body
  body=$(printf '{"durable":true,"auto_delete":false,"exclusive":false}')

  if $DRY_RUN; then
    dry "Queue     ${BOLD}${name}${RESET}  (durable)"
    return
  fi

  local http_code
  http_code=$(curl -s -o /dev/null -w "%{http_code}" \
    -u "${USER}:${PASS}" \
    -X PUT \
    -H "Content-Type: application/json" \
    -d "$body" \
    "${BASE_URL}/queues/${VHOST_ENC}/${name}")

  case $http_code in
    200|201|204) ok "Queue     ${BOLD}${name}${RESET}  (durable)" ;;
    *) err "Queue     ${name}  → HTTP ${http_code}"; return 1 ;;
  esac
}

# POST /api/bindings/{vhost}/e/{exchange}/q/{queue}
declare_binding() {
  local exchange="$1"
  local queue="$2"
  local routing_key="$3"
  local body
  body=$(printf '{"routing_key":"%s","arguments":{}}' "$routing_key")

  if $DRY_RUN; then
    dry "Binding   ${BOLD}${exchange}${RESET} → ${routing_key} → ${BOLD}${queue}${RESET}"
    return
  fi

  local http_code
  http_code=$(curl -s -o /dev/null -w "%{http_code}" \
    -u "${USER}:${PASS}" \
    -X POST \
    -H "Content-Type: application/json" \
    -d "$body" \
    "${BASE_URL}/bindings/${VHOST_ENC}/e/${exchange}/q/${queue}")

  case $http_code in
    200|201|204) ok "Binding   ${BOLD}${exchange}${RESET} → ${routing_key} → ${BOLD}${queue}${RESET}" ;;
    *) err "Binding   ${exchange} → ${routing_key} → ${queue}  → HTTP ${http_code}"; return 1 ;;
  esac
}

# ---------------------------------------------------------------------------
# Wait for RabbitMQ to be ready
# ---------------------------------------------------------------------------
wait_for_rabbitmq() {
  local max_attempts=30
  local attempt=0

  info "Waiting for RabbitMQ at ${HOST}:${PORT} ..."
  until curl -s -o /dev/null -u "${USER}:${PASS}" "${BASE_URL}/overview"; do
    attempt=$((attempt + 1))
    if [[ $attempt -ge $max_attempts ]]; then
      err "RabbitMQ not reachable after ${max_attempts} attempts. Is it running?"
      err "  Expected: http://${HOST}:${PORT}"
      err "  User:     ${USER}"
      exit 1
    fi
    echo -n "."
    sleep 2
  done
  echo ""
  ok "RabbitMQ is ready"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
echo ""
echo -e "${BOLD}Padosme RabbitMQ Setup${RESET}"
echo    "─────────────────────────────────────────────────────────────"
echo    "  Host   : ${HOST}:${PORT}"
echo    "  VHost  : ${VHOST}"
echo    "  User   : ${USER}"
$DRY_RUN && echo -e "  Mode   : ${YELLOW}DRY RUN — no changes will be made${RESET}"
echo    "─────────────────────────────────────────────────────────────"
echo ""

if ! $DRY_RUN; then
  wait_for_rabbitmq
fi

# ---------------------------------------------------------------------------
# Exchanges
# ---------------------------------------------------------------------------
# Flow:
#   auth-service         → otp.events    → mobile-sms-service
#   auth-service         → user.events   → user-profile-service
#   user-profile-service → profile.events → seller-service
#   seller-service       → padosme.events → (no consumer yet — seller lifecycle events)
# ---------------------------------------------------------------------------
echo -e "${BOLD}Exchanges${RESET}"

declare_exchange "otp.events"
declare_exchange "user.events"
declare_exchange "profile.events"
declare_exchange "padosme.events"

echo ""

# ---------------------------------------------------------------------------
# Queues
# ---------------------------------------------------------------------------
echo -e "${BOLD}Queues${RESET}"

declare_queue "otp.sent"
declare_queue "profile-service.user.created"
declare_queue "seller-service.profile.updated"

echo ""

# ---------------------------------------------------------------------------
# Bindings
# ---------------------------------------------------------------------------
echo -e "${BOLD}Bindings${RESET}"

# auth-service publishes otp.requested → sms-service consumes from otp.sent
declare_binding "otp.events"     "otp.sent"                        "otp.requested"

# auth-service publishes user.created → profile-service consumes
declare_binding "user.events"    "profile-service.user.created"    "user.created"

# profile-service publishes profile.updated → seller-service consumes
declare_binding "profile.events" "seller-service.profile.updated"  "profile.updated"

echo ""

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo "─────────────────────────────────────────────────────────────"
if $DRY_RUN; then
  warn "Dry run complete — nothing was created"
else
  ok  "Setup complete"
  echo ""
  warn "NOTE: 'padosme.events' has no consumer yet."
  warn "      seller-service publishes seller.requested + seller.verified to it."
  warn "      auth-service should consume seller.verified to flip user_type→seller."
fi
echo ""
