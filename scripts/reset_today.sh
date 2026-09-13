#!/bin/bash
set -euo pipefail
TOKEN="${TRAFFIC_TOKEN:?set TRAFFIC_TOKEN}"
BASE="${TRAFFIC_BASE:-http://127.0.0.1:50000}"
ROUTER_ID="${TRAFFIC_ROUTER_ID:-main-router-01}"
curl -sS -X POST "${BASE}/api/v1/traffic/reset" \
  -H "X-Device-Token: ${TOKEN}"
echo
curl -sS -H "X-User-ID: smoke" "${BASE}/api/v1/traffic/stats?router_id=${ROUTER_ID}"
echo
