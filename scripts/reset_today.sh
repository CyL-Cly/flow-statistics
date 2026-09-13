#!/bin/bash
TOKEN="dd16d55a075f1429067e86be1a0b576c"
curl -sS -X POST "http://127.0.0.1:50000/api/v1/traffic/reset" \
  -H "X-Device-Token: ${TOKEN}"
echo
curl -sS -H "X-User-ID: smoke" "http://127.0.0.1:50000/api/v1/traffic/stats"
echo
