#!/bin/bash
set -euo pipefail
BASE="${TRAFFIC_BASE:-http://127.0.0.1:50000}"
ROUTER_ID="${TRAFFIC_ROUTER_ID:-main-router-01}"
curl -sS -H "X-User-ID: smoke" "${BASE}/api/v1/traffic/stats?router_id=${ROUTER_ID}" | python3 -m json.tool
