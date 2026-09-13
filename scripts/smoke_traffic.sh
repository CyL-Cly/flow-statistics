#!/bin/bash
set -e
TOKEN="dd16d55a075f1429067e86be1a0b576c"
BODY='{"router_id":"main-router-01","timestamp":0,"interval_sec":5,"devices":[{"mac":"AA:BB:CC:DD:EE:FF","ip":"192.168.50.10","name":"test-phone","iface":"phy0-ap0","rx_bytes":5242880,"tx_bytes":1048576}]}'
BODY2='{"router_id":"main-router-01","timestamp":0,"interval_sec":5,"devices":[{"mac":"11:22:33:44:55:66","ip":"192.168.50.20","name":"via-nginx","iface":"br-lan","rx_bytes":1000000,"tx_bytes":200000}]}'

echo "=== local report ==="
curl -sS -i -X POST "http://127.0.0.1:50000/api/v1/traffic/report" \
  -H "Content-Type: application/json" \
  -H "X-Device-Token: $TOKEN" \
  -d "$BODY" | head -20

echo
echo "=== local stats (no gateway user -> expect 401 from app) ==="
curl -sS -i "http://127.0.0.1:50000/api/v1/traffic/stats" | head -20

echo
echo "=== public report via nginx ==="
curl -sS -i -X POST "https://dagongren.tech/api/v1/traffic/report" \
  -H "Content-Type: application/json" \
  -H "X-Device-Token: $TOKEN" \
  -d "$BODY2" | head -25

echo
echo "=== public stats no cookie ==="
curl -sS -o /tmp/stats_body.txt -w "http_code=%{http_code}\n" "https://dagongren.tech/api/v1/traffic/stats"
head -c 200 /tmp/stats_body.txt; echo

echo
echo "=== public traffic page ==="
curl -sS -o /dev/null -w "http_code=%{http_code} redirect=%{redirect_url}\n" "https://dagongren.tech/app/traffic.html"

echo
echo "=== bad token report ==="
curl -sS -o /dev/null -w "http_code=%{http_code}\n" -X POST "https://dagongren.tech/api/v1/traffic/report" \
  -H "Content-Type: application/json" \
  -H "X-Device-Token: wrong" \
  -d "$BODY"

echo SMOKE_DONE
