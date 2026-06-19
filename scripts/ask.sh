#!/usr/bin/env bash
# Обёртка для ручного теста: ./scripts/ask.sh "выручка за последнюю неделю" [tenant_id]
set -euo pipefail

API="${API:-http://localhost:8088}"
TEXT="${1:-выручка за сегодня}"
TENANT="${2:-mock_single}"

curl -sS -X POST "$API/ask" \
  -H 'Content-Type: application/json' \
  -H "X-Tenant-ID: $TENANT" \
  -d "$(jq -n --arg t "$TEXT" '{text:$t}')" | jq .
