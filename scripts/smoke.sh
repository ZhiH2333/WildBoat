#!/usr/bin/env sh
set -eu

base_url="${SMOKE_BASE_URL:-http://127.0.0.1:8080}"

echo "GET ${base_url}/healthz"
curl -fsS "${base_url}/healthz" | grep -q '"status":"ok"'

echo "GET ${base_url}/readyz"
curl -fsS "${base_url}/readyz" | grep -q '"status":"ready"'

echo "smoke ok"
