#!/usr/bin/env bash
# ============================================================================
# setup-rabbitmq.sh — Declare the complete RabbitMQ topology for Padosme
#
# Usage:
#   ./setup-rabbitmq.sh                          # defaults: localhost:15672 guest/guest /
#   ./setup-rabbitmq.sh -H rmq.padosme.app -P 15672 -u admin -p secret -V /
#
# Requires: rabbitmqadmin (ships with RabbitMQ management plugin)
#   brew install rabbitmq   OR   apt install rabbitmq-server
#   rabbitmq-plugins enable rabbitmq_management
# ============================================================================
set -euo pipefail

# ── Defaults ────────────────────────────────────────────────────────────────
HOST="${RABBITMQ_HOST:-localhost}"
PORT="${RABBITMQ_MGMT_PORT:-15672}"
USER="${RABBITMQ_USER:-guest}"
PASS="${RABBITMQ_PASSWORD:-guest}"
VHOST="${RABBITMQ_VHOST:-/}"

# ── CLI override ────────────────────────────────────────────────────────────
while getopts "H:P:u:p:V:" opt; do
  case "$opt" in
    H) HOST="$OPTARG" ;;
    P) PORT="$OPTARG" ;;
    u) USER="$OPTARG" ;;
    p) PASS="$OPTARG" ;;
    V) VHOST="$OPTARG" ;;
    *) echo "Usage: $0 [-H host] [-P mgmt_port] [-u user] [-p pass] [-V vhost]"; exit 1 ;;
  esac
done

ADM="rabbitmqadmin --host=$HOST --port=$PORT --username=$USER --password=$PASS --vhost=$VHOST"

echo "==> Padosme RabbitMQ topology setup"
echo "    Host: $HOST:$PORT  VHost: $VHOST  User: $USER"
echo ""

# ============================================================================
# 1. EXCHANGES — all topic type, durable
# ============================================================================
echo "--- Declaring exchanges ---"

EXCHANGES=(
  "padosme.events"
  "location.events"
  "catalog.events"
  "rating.events"
  "config.events"
  "coupon.events"
  "channel.events"
  "wallet.events"
  "payment.events"
  "analytics.events"
  "profile.events"
  "zoho.exchange"
)

for ex in "${EXCHANGES[@]}"; do
  echo "  exchange: $ex (topic)"
  $ADM declare exchange name="$ex" type=topic durable=true
done

# ── Dead-Letter Exchanges (direct type) ────────────────────────────────────
DLX_EXCHANGES=(
  "seller-service.dlx"
  "indexing-service.dlx"
  "rating-service.dlx"
  "wallet-service.dlx"
  "wallet-service.promo.dlx"
  "wallet-service.channel.dlx"
  "dictionary-service.dlx"
  "coupon-service.dlx"
)

for dlx in "${DLX_EXCHANGES[@]}"; do
  echo "  exchange: $dlx (direct, DLX)"
  $ADM declare exchange name="$dlx" type=direct durable=true
done

echo ""

# ============================================================================
# 2. HELPER — declare DLQ, bind to DLX, declare main queue with DLX args
# ============================================================================
declare_queue_with_dlq() {
  local QUEUE="$1"
  local DLQ="$2"
  local DLX="$3"
  local DLQ_RK="$4"

  echo "  dlq: $DLQ -> $DLX (rk=$DLQ_RK)"
  $ADM declare queue name="$DLQ" durable=true
  $ADM declare binding source="$DLX" destination="$DLQ" routing_key="$DLQ_RK"

  echo "  queue: $QUEUE (dlx=$DLX)"
  $ADM declare queue name="$QUEUE" durable=true \
    arguments="{\"x-dead-letter-exchange\":\"$DLX\",\"x-dead-letter-routing-key\":\"$DLQ_RK\"}"
}

bind_queue() {
  local QUEUE="$1"
  local EXCHANGE="$2"
  local RK="$3"
  echo "    bind: $EXCHANGE -> $QUEUE (rk=$RK)"
  $ADM declare binding source="$EXCHANGE" destination="$QUEUE" routing_key="$RK"
}

# ============================================================================
# 3. QUEUES & BINDINGS — per service
# ============================================================================

# ── padosme-seller-service ──────────────────────────────────────────────────
echo "--- seller-service ---"
declare_queue_with_dlq \
  "seller-service.profile.updated" \
  "seller-service.dead-letter" \
  "seller-service.dlx" \
  "dlq.seller"

bind_queue "seller-service.profile.updated" "profile.events" "profile.updated"

# ── padosme-indexing-service ────────────────────────────────────────────────
echo "--- indexing-service ---"

# Seller events queue
declare_queue_with_dlq "indexing.seller.events" "indexing.dlq.seller" "indexing-service.dlx" "dlq.seller"
for rk in seller.verified seller.suspended seller.reactivated seller.profile.updated \
          seller.outlet.created seller.outlet.updated seller.outlet.deactivated; do
  bind_queue "indexing.seller.events" "padosme.events" "$rk"
done

# Location events queue
declare_queue_with_dlq "indexing.location.events" "indexing.dlq.location" "indexing-service.dlx" "dlq.location"
bind_queue "indexing.location.events" "location.events" "seller.location.updated"
bind_queue "indexing.location.events" "location.events" "seller.presence.changed"

# Catalog events queue
declare_queue_with_dlq "indexing.catalog.events" "indexing.dlq.catalog" "indexing-service.dlx" "dlq.catalog"
for rk in catalogue.updated item.created item.updated item.deleted; do
  bind_queue "indexing.catalog.events" "catalog.events" "$rk"
done

# Rating events queue
declare_queue_with_dlq "indexing.rating.events" "indexing.dlq.rating" "indexing-service.dlx" "dlq.rating"
bind_queue "indexing.rating.events" "rating.events" "seller.rating.updated"

# ── padosme-analytics-service ───────────────────────────────────────────────
echo "--- analytics-service ---"
$ADM declare queue name="analytics-service-queue" durable=true
bind_queue "analytics-service-queue" "padosme.events" "seller.created"
bind_queue "analytics-service-queue" "padosme.events" "seller.verified"
bind_queue "analytics-service-queue" "catalog.events" "catalogue.created"
bind_queue "analytics-service-queue" "catalog.events" "catalogue.updated"
bind_queue "analytics-service-queue" "rating.events"  "review.created"
bind_queue "analytics-service-queue" "coupon.events"  "coupon.redeemed"
bind_queue "analytics-service-queue" "wallet.events"  "wallet.credited"

# ── padosme-wallet-service ──────────────────────────────────────────────────
echo "--- wallet-service ---"
declare_queue_with_dlq "wallet-service.payment.success" "wallet-service.dead-letter" "wallet-service.dlx" "dlq.wallet.payment"
bind_queue "wallet-service.payment.success" "payment.events" "payment.success"

declare_queue_with_dlq "wallet-service.promo.viewed" "wallet-service.promo.dead-letter" "wallet-service.promo.dlx" "dlq.wallet.promo"
bind_queue "wallet-service.promo.viewed" "analytics.events" "promo.viewed"

declare_queue_with_dlq "wallet-service.channel.post.created" "wallet-service.channel.dead-letter" "wallet-service.channel.dlx" "dlq.wallet.channel"
bind_queue "wallet-service.channel.post.created" "channel.events" "channel.post.created"

# ── padosme-rating-service ──────────────────────────────────────────────────
echo "--- rating-service ---"
declare_queue_with_dlq "rating-service.events" "rating-service.dead-letter" "rating-service.dlx" "dlq.rating"
bind_queue "rating-service.events" "catalog.events" "item.created"
bind_queue "rating-service.events" "catalog.events" "item.updated"

# ── padosme-coupon-service ──────────────────────────────────────────────────
echo "--- coupon-service ---"
declare_queue_with_dlq "coupon-service.events" "coupon-service.dead-letter" "coupon-service.dlx" "dlq.coupon"
bind_queue "coupon-service.events" "padosme.events" "seller.verified"

# ── padosme-channel-service ─────────────────────────────────────────────────
echo "--- channel-service ---"
$ADM declare queue name="padosme-channel-service" durable=true
bind_queue "padosme-channel-service" "padosme.events"  "seller.verified"
bind_queue "padosme-channel-service" "catalog.events"  "item.created"
bind_queue "padosme-channel-service" "rating.events"   "review.created"

# ── padosme-dictionary-service ──────────────────────────────────────────────
echo "--- dictionary-service ---"
declare_queue_with_dlq "dictionary-service.events" "dictionary-service.dead-letter" "dictionary-service.dlx" "dlq.dictionary"
bind_queue "dictionary-service.events" "catalog.events" "item.created"
bind_queue "dictionary-service.events" "catalog.events" "item.updated"

# ── padosme-subscription-service ────────────────────────────────────────────
echo "--- subscription-service ---"
$ADM declare queue name="subscription-service.events" durable=true
bind_queue "subscription-service.events" "payment.events" "payment.success"

# ── padosme-notification-service ────────────────────────────────────────────
echo "--- notification-service ---"
$ADM declare queue name="notification-service.events" durable=true
bind_queue "notification-service.events" "padosme.events"  "seller.verified"
bind_queue "notification-service.events" "coupon.events"   "coupon.created"
bind_queue "notification-service.events" "coupon.events"   "coupon.redeemed"
bind_queue "notification-service.events" "wallet.events"   "wallet.credited"
bind_queue "notification-service.events" "channel.events"  "channel.post.created"
bind_queue "notification-service.events" "payment.events"  "payment.success"

# ── ledgers-cloud-connect-service ───────────────────────────────────────────
echo "--- ledgers-cloud-connect ---"
$ADM declare queue name="ledgers-cloud-connect.events" durable=true
bind_queue "ledgers-cloud-connect.events" "zoho.exchange" "zoho.bill.#"
bind_queue "ledgers-cloud-connect.events" "zoho.exchange" "zoho.invoice.#"
bind_queue "ledgers-cloud-connect.events" "zoho.exchange" "zoho.subscription.#"
bind_queue "ledgers-cloud-connect.events" "zoho.exchange" "zoho.bank_transaction.#"

echo ""
echo "==> RabbitMQ topology setup complete."
echo "    Exchanges: ${#EXCHANGES[@]} topic + ${#DLX_EXCHANGES[@]} DLX"
echo "    Run 'rabbitmqadmin list queues' to verify."
