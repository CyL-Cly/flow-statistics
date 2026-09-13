#!/bin/bash
curl -sS -H "X-User-ID: smoke" "http://127.0.0.1:50000/api/v1/traffic/stats" | python3 -m json.tool
