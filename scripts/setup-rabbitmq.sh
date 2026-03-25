#!/usr/bin/env bash
# setup-rabbitmq.sh — Declare ALL Padosme RabbitMQ exchanges, queues, bindings & DLX topology
# Uses the RabbitMQ Management HTTP API (port 15672).
#
# This is the single source of truth for the entire Padosme RMQ plumbing.
# Run after every RabbitMQ restart, fresh deploy, or cluster rebuild.
#
# Usage:
#   ./scripts/setup-rabbitmq.sh [--dry-run]
#
# Environment variables (all optional, defaults match docker-compose.yml):
#   RABBITMQ_HOST        default: localhost
#   RABBITMQ_MGMT_PORT   default: 15673
#   RABBITMQ_USER        default: deploy 
#   RABBITMQ_PASSWORD    default: Kaas-Labs 
#   RABBITMQ_VHOST       default: /
#
# Last audited: 2026-03-23
# Services covered:
#   padosme-auth-service, padosme-user-profile-service, padosme-seller-service,
#   mobile-sms-service, padosme-notification-service, padosme-rating-service,
#   padosme-analytics-service, padosme-channel-service, padosme-catalogue-service,
#   padosme-wallet-service, padosme-coupon-service, padosme-subscription-service,
#   padosme-discovery-service, ledgers-cloud-connect-service, padosme-indexing-service

set -euo pipefail

# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------
HOST="${RABBITMQ_HOST:-localhost}"
PORT="${RABBITMQ_MGMT_PORT:-15673}"
USER="${RABBITMQ_USER:-deploy}"
PASS="${RABBITMQ_PASSWORD:-Kaas-Labs}"
VHOST="${RABBITMQ_VHOST:-/}"
DRY_RUN=false

for arg in "$@"; do
  case $arg in
    --dry-run) DRY_RUN=true ;;
    *) echo "Unknown argument: $arg"; exit 1 ;;
  esac
done

BASE_URL="http://${HOST}:${PORT}/api"
VHOST_ENC="${VHOST//\//%2F}"

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

# PUT /api/exchanges/{vhost}/{name}
# Args: name [type]   (type defaults to "topic")
declare_exchange() {
  local name="$1"
  local type="${2:-topic}"
  local body
  body=$(printf '{"type":"%s","durable":true,"auto_delete":false,"internal":false}' "$type")

  if $DRY_RUN; then
    dry "Exchange  ${BOLD}${name}${RESET}  (${type}, durable)"
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
    200|201|204) ok "Exchange  ${BOLD}${name}${RESET}  (${type}, durable)" ;;
    *) err "Exchange  ${name}  -> HTTP ${http_code}"; ERRORS=$((ERRORS + 1)) ;;
  esac
}

# PUT /api/queues/{vhost}/{name}
# Args: name [json_arguments]
#   json_arguments is raw JSON for x-dead-letter-exchange, x-message-ttl, etc.
declare_queue() {
  local name="$1"
  local args="${2:-}"
  local body

  if [[ -n "$args" ]]; then
    body=$(printf '{"durable":true,"auto_delete":false,"exclusive":false,"arguments":%s}' "$args")
  else
    body='{"durable":true,"auto_delete":false,"exclusive":false}'
  fi

  local label="${BOLD}${name}${RESET}  (durable)"
  [[ -n "$args" ]] && label="${BOLD}${name}${RESET}  (durable, args)"

  if $DRY_RUN; then
    dry "Queue     ${label}"
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
    200|201|204) ok "Queue     ${label}" ;;
    *) err "Queue     ${name}  -> HTTP ${http_code}"; ERRORS=$((ERRORS + 1)) ;;
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
    dry "Binding   ${BOLD}${exchange}${RESET} -> ${routing_key} -> ${BOLD}${queue}${RESET}"
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
    200|201|204) ok "Binding   ${BOLD}${exchange}${RESET} -> ${routing_key} -> ${BOLD}${queue}${RESET}" ;;
    *) err "Binding   ${exchange} -> ${routing_key} -> ${queue}  -> HTTP ${http_code}"; ERRORS=$((ERRORS + 1)) ;;
  esac
}

# ---------------------------------------------------------------------------
# Wait for RabbitMQ
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

# =========================================================================
#                            M A I N
# =========================================================================
echo ""
echo -e "${BOLD}Padosme RabbitMQ Full Topology Setup${RESET}"
echo    "================================================================="
echo    "  Host   : ${HOST}:${PORT}"
echo    "  VHost  : ${VHOST}"
echo    "  User   : ${USER}"
$DRY_RUN && echo -e "  Mode   : ${YELLOW}DRY RUN -- no changes will be made${RESET}"
echo    "================================================================="
echo ""

if ! $DRY_RUN; then
  wait_for_rabbitmq
fi

# =========================================================================
# 1. EXCHANGES — Business Domain (topic, durable)
# =========================================================================
#
# Event Flow Map:
#   auth-service            -> otp.events      -> mobile-sms-service
#   auth-service            -> user.events     -> user-profile-service, rating-service
#   user-profile-service    -> profile.events  -> seller-service
#   seller-service          -> padosme.events  -> notification-service, indexing-service
#   seller-service          -> location.events -> indexing-service
#   rating-service          -> rating.events   -> analytics-service, indexing-service
#   catalogue-service       -> catalog.events  -> rating-service, analytics-service, indexing-service
#   channel-service         -> channel.events  -> analytics-service
#   wallet-service          -> wallet.events   -> analytics-service
#   coupon-service          -> coupon.events   -> notification-service, wallet-service, analytics-service
#   subscription-service    -> subscription.events -> coupon-service, wallet-service, analytics-service
#   analytics-service       -> analytics.events -> (dashboards / downstream)
#   discovery-service       -> discovery.events -> analytics-service
#   seller-service          -> seller.events   -> rating-service, analytics-service, channel-service
#   padosme.events (seller) -> coupon-service (seller.verified)
#   ledgers-cloud-connect   -> ledgers.exchange -> ledgers internal
# =========================================================================

section "1. EXCHANGES -- Business Domain"

declare_exchange "otp.events"
declare_exchange "user.events"
declare_exchange "profile.events"
declare_exchange "padosme.events"
declare_exchange "catalog.events"
declare_exchange "seller.events"
declare_exchange "rating.events"
declare_exchange "channel.events"
declare_exchange "wallet.events"
declare_exchange "coupon.events"
declare_exchange "analytics.events"
declare_exchange "discovery.events"
declare_exchange "subscription.events"
declare_exchange "location.events"

# Channel-service expects these specific exchange names
declare_exchange "padosme.channel"
declare_exchange "padosme.seller"
declare_exchange "padosme.auth"

# Ledgers service
declare_exchange "ledgers.exchange"

# =========================================================================
# 2. EXCHANGES — Dead Letter / Retry (various types)
# =========================================================================

section "2. EXCHANGES -- Dead Letter / Retry"

# user-profile-service DLX (fanout — all nacked msgs go to one DLQ)
declare_exchange "profile-service.dlx"     "fanout"

# notification-service retry + DLQ (direct — routing-key based)
declare_exchange "padosme.retry"           "direct"
declare_exchange "padosme.dlq"             "direct"

# channel-service DLX (topic — wildcard binding)
declare_exchange "padosme.dlx"             "topic"

# rating-service DLX (fanout)
declare_exchange "rating-service.dlx"      "fanout"

# analytics-service DLQ (direct)
declare_exchange "analytics.dlq"           "direct"

# seller-service DLX (fanout — for nacked messages)
declare_exchange "seller-service.dlx"      "fanout"

# sms-service DLX (fanout)
declare_exchange "sms-service.dlx"         "fanout"

# coupon-service DLX (fanout — nacked consumer msgs)
declare_exchange "coupon-service.dlx"      "fanout"

# wallet-service DLX (fanout)
declare_exchange "wallet-service.dlx"      "fanout"

# subscription-service DLX (fanout)
declare_exchange "subscription-service.dlx" "fanout"

# indexing-service DLX (fanout — all 4 consumer DLQs)
declare_exchange "indexing-service.dlx"    "fanout"

# ledgers DLX
declare_exchange "ledgers.exchange.dlx"    "topic"

# =========================================================================
# 3. QUEUES — Main Service Queues (with DLX arguments where applicable)
# =========================================================================

section "3. QUEUES -- Main Service Queues"

# --- mobile-sms-service ---
# Consumes otp.requested from otp.events
declare_queue "otp.sent" \
  '{"x-dead-letter-exchange":"sms-service.dlx"}'

# --- user-profile-service ---
# Consumes user.created from user.events
declare_queue "profile-service.user.created" \
  '{"x-dead-letter-exchange":"profile-service.dlx"}'

# --- seller-service ---
# Consumes profile.updated from profile.events
declare_queue "seller-service.profile.updated" \
  '{"x-dead-letter-exchange":"seller-service.dlx"}'

# --- notification-service ---
# Main queue: consumes notification.*, device.token.*, notification.retry from padosme.events
# DLX -> retry exchange (for exponential backoff retries)
declare_queue "notification-service.notifications" \
  '{"x-dead-letter-exchange":"padosme.retry"}'

# Retry queue: messages wait here with per-message TTL then route back to padosme.events
declare_queue "notification-service.notifications.retry" \
  '{"x-dead-letter-exchange":"padosme.events","x-dead-letter-routing-key":"notification.retry"}'

# --- rating-service ---
# Consumes seller.deleted, user.deleted, catalogue.deleted from multiple exchanges
declare_queue "rating-service.events" \
  '{"x-dead-letter-exchange":"rating-service.dlx"}'

# --- analytics-service ---
# Mega-consumer: ingests events from 10+ upstream exchanges
declare_queue "analytics.ingest" \
  '{"x-dead-letter-exchange":"analytics.dlq","x-dead-letter-routing-key":"analytics.ingest.dlq"}'

# --- channel-service ---
# Consumes seller.created, seller.deleted from padosme.seller; user.deleted from padosme.auth
declare_queue "padosme-channel-service" \
  '{"x-dead-letter-exchange":"padosme.dlx"}'

# --- padosme-indexing-service ---
# 4 independent consumer queues, one per upstream exchange domain, each with DLX
declare_queue "indexing.seller.events" \
  '{"x-dead-letter-exchange":"indexing-service.dlx"}'

declare_queue "indexing.location.events" \
  '{"x-dead-letter-exchange":"indexing-service.dlx"}'

declare_queue "indexing.catalog.events" \
  '{"x-dead-letter-exchange":"indexing-service.dlx"}'

declare_queue "indexing.rating.events" \
  '{"x-dead-letter-exchange":"indexing-service.dlx"}'

# --- discovery-service ---
# Internal queues using default exchange (direct queue-name routing)
declare_queue "search.requested"
declare_queue "search.result"

# --- coupon-service (consumers) ---
# Consumes subscription.created from subscription.events (close audit trail)
declare_queue "coupon.subscription-events" \
  '{"x-dead-letter-exchange":"coupon-service.dlx"}'

# Consumes seller.verified from padosme.events (cache seller for validation)
declare_queue "coupon.seller-events" \
  '{"x-dead-letter-exchange":"coupon-service.dlx"}'

# --- notification-service (coupon domain) ---
# Consumes coupon events from coupon.events
declare_queue "notification.coupon-events" \
  '{"x-dead-letter-exchange":"padosme.retry"}'

# --- wallet-service (coupon domain) ---
# Consumes coupon.redeemed from coupon.events (salesman commission)
declare_queue "wallet.coupon-events" \
  '{"x-dead-letter-exchange":"wallet-service.dlx"}'

# --- subscription-service (coupon domain) ---
# Consumes coupon.validated from coupon.events (pre-apply discount info)
declare_queue "subscription.coupon-events" \
  '{"x-dead-letter-exchange":"subscription-service.dlx"}'

# --- ledgers-cloud-connect-service ---
# All queues with TTL + DLX
declare_queue "ledgers.requests" \
  '{"x-message-ttl":300000,"x-dead-letter-exchange":"ledgers.exchange.dlx","x-dead-letter-routing-key":"dead.requests"}'

declare_queue "ledgers.responses" \
  '{"x-message-ttl":300000,"x-dead-letter-exchange":"ledgers.exchange.dlx","x-dead-letter-routing-key":"dead.responses"}'

declare_queue "ledgers.events" \
  '{"x-message-ttl":600000,"x-dead-letter-exchange":"ledgers.exchange.dlx","x-dead-letter-routing-key":"dead.events"}'

declare_queue "ledgers.subscription.payments" \
  '{"x-message-ttl":600000,"x-dead-letter-exchange":"ledgers.exchange.dlx","x-dead-letter-routing-key":"dead.subscription"}'

# =========================================================================
# 4. QUEUES — Dead Letter Queues
# =========================================================================

section "4. QUEUES -- Dead Letter Queues"

# sms-service DLQ
declare_queue "sms-service.dead-letter"

# profile-service DLQ
declare_queue "profile-service.dead-letter"

# seller-service DLQ
declare_queue "seller-service.dead-letter"

# notification-service DLQ (exhausted retries land here)
declare_queue "notification-service.notifications.dlq"

# rating-service DLQ
declare_queue "rating-service.dlq"

# analytics-service DLQ
declare_queue "analytics.ingest.dlq"

# channel-service DLQ
declare_queue "padosme-channel-service.dlq"

# coupon-service DLQ
declare_queue "coupon-service.dead-letter"

# wallet-service DLQ
declare_queue "wallet-service.dead-letter"

# subscription-service DLQ
declare_queue "subscription-service.dead-letter"

# indexing-service DLQs (one per consumer queue)
declare_queue "indexing.dlq.seller"
declare_queue "indexing.dlq.location"
declare_queue "indexing.dlq.catalog"
declare_queue "indexing.dlq.rating"

# ledgers DLQs (one per queue type)
declare_queue "dead.requests"
declare_queue "dead.responses"
declare_queue "dead.events"
declare_queue "dead.subscription"

# =========================================================================
# 5. BINDINGS — Main Queue Bindings
# =========================================================================

section "5. BINDINGS -- Main Queue Bindings"

# ---- otp.events -> otp.sent (mobile-sms-service) ----
declare_binding "otp.events" "otp.sent" "otp.requested"

# ---- user.events -> profile-service.user.created (user-profile-service) ----
declare_binding "user.events" "profile-service.user.created" "user.created"

# ---- profile.events -> seller-service.profile.updated (seller-service) ----
declare_binding "profile.events" "seller-service.profile.updated" "profile.updated"

# ---- padosme.events -> notification-service (notification-service) ----
declare_binding "padosme.events" "notification-service.notifications" "notification.*"
declare_binding "padosme.events" "notification-service.notifications" "notification.retry"
declare_binding "padosme.events" "notification-service.notifications" "device.token.*"

# ---- rating-service.events: binds to 3 upstream exchanges ----
declare_binding "seller.events"  "rating-service.events" "seller.deleted"
declare_binding "user.events"    "rating-service.events" "user.deleted"
declare_binding "catalog.events" "rating-service.events" "catalogue.deleted"

# ---- analytics.ingest: binds to all upstream exchanges ----
# auth/user events
declare_binding "user.events"         "analytics.ingest" "user.created"
# seller events
declare_binding "seller.events"       "analytics.ingest" "seller.created"
declare_binding "seller.events"       "analytics.ingest" "seller.verified"
# catalog events
declare_binding "catalog.events"      "analytics.ingest" "catalogue.created"
declare_binding "catalog.events"      "analytics.ingest" "catalogue.updated"
# channel events
declare_binding "channel.events"      "analytics.ingest" "channel.post.created"
# rating events
declare_binding "rating.events"       "analytics.ingest" "review.created"
# discovery events
declare_binding "discovery.events"    "analytics.ingest" "search.performed"
declare_binding "discovery.events"    "analytics.ingest" "search.result.clicked"
# wallet events
declare_binding "wallet.events"       "analytics.ingest" "wallet.debited"
declare_binding "wallet.events"       "analytics.ingest" "wallet.credited"
# coupon events
declare_binding "coupon.events"       "analytics.ingest" "coupon.redeemed"
# subscription events
declare_binding "subscription.events" "analytics.ingest" "subscription.created"
declare_binding "subscription.events" "analytics.ingest" "subscription.renewed"

# ---- coupon-service (consumers): bind to upstream exchanges ----
declare_binding "subscription.events" "coupon.subscription-events" "subscription.created"
declare_binding "padosme.events"      "coupon.seller-events"       "seller.verified"

# ---- notification-service: coupon domain events ----
declare_binding "coupon.events" "notification.coupon-events" "coupon.created"
declare_binding "coupon.events" "notification.coupon-events" "coupon.redeemed"
declare_binding "coupon.events" "notification.coupon-events" "campaign.limit_reached"

# ---- wallet-service: coupon domain events ----
declare_binding "coupon.events" "wallet.coupon-events" "coupon.redeemed"

# ---- subscription-service: coupon domain events ----
declare_binding "coupon.events" "subscription.coupon-events" "coupon.validated"

# ---- analytics.ingest: additional coupon domain events ----
declare_binding "coupon.events" "analytics.ingest" "coupon.created"
declare_binding "coupon.events" "analytics.ingest" "coupon.validated"
declare_binding "coupon.events" "analytics.ingest" "coupon.expired"
declare_binding "coupon.events" "analytics.ingest" "campaign.limit_reached"

# ---- padosme-channel-service: binds to seller + auth exchanges ----
declare_binding "padosme.seller" "padosme-channel-service" "seller.created"
declare_binding "padosme.seller" "padosme-channel-service" "seller.deleted"
declare_binding "padosme.auth"   "padosme-channel-service" "user.deleted"

# ---- padosme-indexing-service ----
# padosme.events: seller lifecycle events
declare_binding "padosme.events" "indexing.seller.events" "seller.verified"
declare_binding "padosme.events" "indexing.seller.events" "seller.suspended"
declare_binding "padosme.events" "indexing.seller.events" "seller.reactivated"
declare_binding "padosme.events" "indexing.seller.events" "seller.profile.updated"

# location.events: geo and presence updates
declare_binding "location.events" "indexing.location.events" "seller.location.updated"
declare_binding "location.events" "indexing.location.events" "seller.presence.changed"

# catalog.events: item and catalogue updates
declare_binding "catalog.events" "indexing.catalog.events" "catalog.updated"
declare_binding "catalog.events" "indexing.catalog.events" "item.created"
declare_binding "catalog.events" "indexing.catalog.events" "item.updated"
declare_binding "catalog.events" "indexing.catalog.events" "item.deleted"

# rating.events: rating updates
declare_binding "rating.events" "indexing.rating.events" "seller.rating.updated"

# ---- ledgers-cloud-connect-service ----
declare_binding "ledgers.exchange" "ledgers.requests"              "request.*"
declare_binding "ledgers.exchange" "ledgers.responses"             "response.*"
declare_binding "ledgers.exchange" "ledgers.events"                "event.#"
declare_binding "ledgers.exchange" "ledgers.subscription.payments" "event.subscription.payment.*"

# =========================================================================
# 6. BINDINGS — Dead Letter Queue Bindings
# =========================================================================

section "6. BINDINGS -- Dead Letter Queue Bindings"

# sms-service DLX -> DLQ
declare_binding "sms-service.dlx"     "sms-service.dead-letter"     ""

# profile-service DLX -> DLQ (fanout, routing key ignored)
declare_binding "profile-service.dlx" "profile-service.dead-letter" ""

# seller-service DLX -> DLQ
declare_binding "seller-service.dlx"  "seller-service.dead-letter"  ""

# notification-service: retry exchange -> retry queue
declare_binding "padosme.retry" "notification-service.notifications.retry" "notification-service.notifications.retry"
# notification-service: DLQ exchange -> DLQ
declare_binding "padosme.dlq"   "notification-service.notifications.dlq"   "notification-service.notifications.dlq"

# rating-service DLX -> DLQ (fanout)
declare_binding "rating-service.dlx" "rating-service.dlq" ""

# analytics-service DLQ exchange -> DLQ
declare_binding "analytics.dlq" "analytics.ingest.dlq" "analytics.ingest.dlq"

# channel-service DLX -> DLQ (wildcard)
declare_binding "padosme.dlx" "padosme-channel-service.dlq" "#"

# coupon-service DLX -> DLQ
declare_binding "coupon-service.dlx"       "coupon-service.dead-letter"       ""

# wallet-service DLX -> DLQ
declare_binding "wallet-service.dlx"       "wallet-service.dead-letter"       ""

# subscription-service DLX -> DLQ
declare_binding "subscription-service.dlx" "subscription-service.dead-letter"  ""

# indexing-service DLX -> DLQs (fanout, routing key ignored)
declare_binding "indexing-service.dlx" "indexing.dlq.seller"   ""
declare_binding "indexing-service.dlx" "indexing.dlq.location" ""
declare_binding "indexing-service.dlx" "indexing.dlq.catalog"  ""
declare_binding "indexing-service.dlx" "indexing.dlq.rating"   ""

# ledgers DLX -> DLQs
declare_binding "ledgers.exchange.dlx" "dead.requests"     "dead.requests"
declare_binding "ledgers.exchange.dlx" "dead.responses"    "dead.responses"
declare_binding "ledgers.exchange.dlx" "dead.events"       "dead.events"
declare_binding "ledgers.exchange.dlx" "dead.subscription" "dead.subscription"

# =========================================================================
# Summary
# =========================================================================
echo ""
echo "================================================================="
if [[ $ERRORS -gt 0 ]]; then
  err "${ERRORS} error(s) occurred during setup. Check output above."
  echo ""
  exit 1
fi

if $DRY_RUN; then
  warn "Dry run complete -- nothing was created"
else
  ok "Setup complete -- all exchanges, queues, and bindings declared"
fi

echo ""
echo -e "${DIM}Topology summary:${RESET}"
echo    "  22 exchanges (15 business domain + 7 DLX/retry)"
echo    "  43 queues (24 main + 19 dead-letter)"
echo    "  72 bindings"
echo ""
echo -e "${DIM}Known gaps (require code changes, not setup changes):${RESET}"
echo    "  - analytics-service consumer binds to 'auth.events' but auth-service"
echo    "    publishes to 'user.events'. The binding above uses 'user.events'."
echo    "  - analytics-service expects 'channel.events' but channel-service"
echo    "    publishes to 'padosme.channel'. Both exchanges are declared."
echo    "  - subscription-service uses Redis Streams, not RMQ. The"
echo    "    'subscription.events' exchange is declared for future migration."
echo    "  - seller-service publishes seller.requested — no consumer declared"
echo    "    yet (pending admin workflow service)."
echo ""
