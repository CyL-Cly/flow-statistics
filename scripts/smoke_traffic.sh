#!/bin/bash
set -euo pipefail
TOKEN="${TRAFFIC_TOKEN:?set TRAFFIC_TOKEN}"
BASE="${TRAFFIC_BASE:-http://127.0.0.1:50000}"
ROUTER_ID="${TRAFFIC_ROUTER_ID:-main-router-01}"
case "$ROUTER_ID" in
  *[!A-Za-z0-9_-]*) echo "invalid ROUTER_ID: $ROUTER_ID" >&2; exit 1 ;;
esac
BODY="{\"router_id\":\"${ROUTER_ID}\",\"timestamp\":0,\"interval_sec\":5,\"devices\":[{\"mac\":\"AA:BB:CC:DD:EE:FF\",\"ip\":\"192.168.50.10\",\"name\":\"test-phone\",\"iface\":\"phy0-ap0\",\"rx_bytes\":5242880,\"tx_bytes\":1048576}]}"

echo "=== local report ==="
curl -sS -i -X POST "${BASE}/api/v1/traffic/report" \
  -H "Content-Type: application/json" \
  -H "X-Device-Token: $TOKEN" \
  -d "$BODY" | head -20

echo
echo "=== local stats ==="
curl -sS -i "${BASE}/api/v1/traffic/stats?router_id=${ROUTER_ID}" | head -20

echo
echo "=== bad token report ==="
curl -sS -o /dev/null -w "http_code=%{http_code}\n" -X POST "${BASE}/api/v1/traffic/report" \
  -H "Content-Type: application/json" \
  -H "X-Device-Token: wrong" \
  -d "$BODY"

echo SMOKE_DONE
