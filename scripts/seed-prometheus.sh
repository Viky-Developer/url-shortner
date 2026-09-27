#!/usr/bin/env bash
# Generates synthetic traffic against the running URL shortener so the
# Prometheus and Grafana dashboards show meaningful data.
#
# Usage:
#   ./scripts/seed-prometheus.sh
#
# The server must already be running (make dev or make run). The default
# admin user is used to authenticate. Destinations are real, public, reliable
# hosts so the destination-health check passes on creation.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

# Honour overrides from .env when present.
if [ -f "$PROJECT_DIR/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "$PROJECT_DIR/.env"
  set +a
fi

HOST="${SERVER_HOST:-localhost}"
PORT="${SERVER_PORT:-8085}"
API="http://${HOST}:${PORT}/api/v1"
METRICS_URL="http://${HOST}:${PORT}/metrics"

EMAIL="${DEFAULT_USER_EMAIL:-default@urlshortner.local}"
PASSWORD="${DEFAULT_USER_PASSWORD:-default123}"

URLS_COUNT="${PROMETHEUS_SEED_URLS:-8}"
CLICKS_PER_URL="${PROMETHEUS_SEED_CLICKS:-25}"

DESTINATIONS=(
  "https://example.com"
  "https://github.com"
  "https://go.dev"
  "https://en.wikipedia.org"
  "https://www.gnu.org"
)

fail() {
  echo "error: $*" >&2
  exit 1
}

echo "==> Seeding Prometheus metrics against $API"

curl -sS -m 10 "$API/health" -o /dev/null || fail "server not reachable at $METRICS_URL — is it running? (make dev)"
echo "    server is up: $METRICS_URL"

login_body="$(mktemp)"
login_code="$(curl -sS -m 10 -o "$login_body" -w '%{http_code}' -X POST "$API/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")"
if [ "$login_code" != "200" ]; then
  cat "$login_body" >&2
  rm -f "$login_body"
  fail "login failed (HTTP $login_code) with $EMAIL — check DEFAULT_USER_* env vars and that the server is running"
fi
TOKEN="$(python3 -c 'import json,sys;print(json.load(sys.stdin)["data"][0]["token"]["accessToken"])' < "$login_body")"
rm -f "$login_body"
[ -n "$TOKEN" ] || fail "login succeeded but no access token returned"

AUTH=(-H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json')

# 1. Create short URLs.
echo "==> Creating $URLS_COUNT short URLs"
for i in $(seq 1 "$URLS_COUNT"); do
  idx=$(((i - 1) % ${#DESTINATIONS[@]}))
  dest="${DESTINATIONS[$idx]}"
  code="prom$(printf '%03d' "$i")"
  status="$(curl -sS -m 20 -o /dev/null -w '%{http_code}' -X POST "$API/shorten" \
    "${AUTH[@]}" \
    -d "{\"originalURL\":\"$dest\",\"customCode\":\"$code\",\"title\":\"Prometheus seed $i\"}")"
  if [ "$status" = "201" ] || [ "$status" = "200" ]; then
    printf '    created %-6s -> %s\n' "$code" "$dest"
  else
    printf '    create %s -> HTTP %s (skipped; may already exist)\n' "$code" "$status"
  fi
done

# 2. Serve redirects so the redirects-served counter grows.
echo "==> Serving $CLICKS_PER_URL redirects per URL ($((URLS_COUNT * CLICKS_PER_URL)) total)"
total=0
for i in $(seq 1 "$URLS_COUNT"); do
  code="prom$(printf '%03d' "$i")"
  for _ in $(seq 1 "$CLICKS_PER_URL"); do
    curl -sS -m 15 -o /dev/null "$API/$code"
    total=$((total + 1))
  done
done
echo "    served $total redirects"

# 3. A little request diversity for the HTTP status breakdown panel.
echo "==> Generating health checks and 404s"
for _ in $(seq 1 5); do
  curl -sS -m 5 -o /dev/null "$API/health"
done
for _ in $(seq 1 8); do
  curl -sS -m 5 -o /dev/null "$API/does-not-exist"
done

# 4. Point of truth: the current cumulative counters.
echo "==> Checking cumulative counters at $METRICS_URL"
curl -sS -m 15 "$METRICS_URL" | grep -E '^http_requests_total\{[^}]*route="[^"]+"[^}]*\}\s' | head -8
curl -sS -m 15 "$METRICS_URL" | grep -E '^(shortener_urls_created_total|shortener_redirects_served_total)' | head -2

cat <<EOF

==> Done. View the data:
    App metrics : $METRICS_URL
    Prometheus  : http://localhost:9090   (query e.g. rate(http_requests_total[5m]))
    Grafana     : http://localhost:3000   (dashboard "URL Shortener Monitoring", admin/admin)
EOF