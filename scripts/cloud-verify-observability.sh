#!/usr/bin/env bash
set -euo pipefail

AUTH_SERVICE_NAME="${AUTH_SERVICE_NAME:-padosme-auth-service}"
STRICT_AUTH_CHECK="${STRICT_AUTH_CHECK:-false}"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

warn() {
  echo -e "${YELLOW}[warn] $*${NC}"
}

ok() {
  echo -e "${GREEN}[ok] $*${NC}"
}

fail() {
  echo -e "${RED}[fail] $*${NC}"
  exit 1
}

check_http() {
  local name="$1"
  local url="$2"
  if curl -fsS "$url" >/dev/null; then
    ok "$name"
  else
    fail "$name (url: $url)"
  fi
}

check_http "Prometheus healthy" "http://localhost:9090/-/healthy"
check_http "Grafana healthy" "http://localhost:3000/api/health"
check_http "Jaeger UI reachable" "http://localhost:16686"
check_http "OTel collector metrics" "http://localhost:8888/metrics"
check_http "OTel collector Prometheus exporter metrics" "http://localhost:8889/metrics"

if curl -s "http://localhost:9090/api/v1/targets" | grep -q '"health":"up"'; then
  ok "Prometheus has healthy targets"
else
  fail "Prometheus targets are not healthy"
fi

query='sum(rate(padosme_http_requests_total{service_name="'"$AUTH_SERVICE_NAME"'"}[5m]))'
auth_metrics_payload="$(curl -sG "http://localhost:9090/api/v1/query" --data-urlencode "query=${query}")"

if echo "$auth_metrics_payload" | grep -q '"status":"success"' && ! echo "$auth_metrics_payload" | grep -q '"result":\[\]'; then
  ok "Auth service metrics visible in Prometheus (${AUTH_SERVICE_NAME})"
else
  if [ "$STRICT_AUTH_CHECK" = "true" ]; then
    fail "Auth service metrics missing in Prometheus (${AUTH_SERVICE_NAME})"
  else
    warn "Auth service metrics not visible yet in Prometheus (${AUTH_SERVICE_NAME}). Generate traffic and re-run check."
  fi
fi

if curl -s "http://localhost:16686/api/services" | grep -q "$AUTH_SERVICE_NAME"; then
  ok "Auth service traces visible in Jaeger (${AUTH_SERVICE_NAME})"
else
  if [ "$STRICT_AUTH_CHECK" = "true" ]; then
    fail "Auth service traces missing in Jaeger (${AUTH_SERVICE_NAME})"
  else
    warn "Auth service traces not visible yet in Jaeger (${AUTH_SERVICE_NAME}). Generate traffic and re-run check."
  fi
fi

echo "Observability verification finished"
