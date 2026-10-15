#!/usr/bin/env bash
# scripts/e2e-basic.sh — FASE 1 basic end-to-end:
# 1 payment via gateway -> wait callback -> final SUCCESS/FAILED ->
# merchant-sink received exactly 1 webhook. Also checks the Envoy path.
set -euo pipefail

GATEWAY_URL="${GATEWAY_URL:-http://127.0.0.1:3000}"
SINK_URL="${SINK_URL:-http://127.0.0.1:5000}"
ENVOY_URL="${ENVOY_URL:-http://127.0.0.1:8080}"
BCA_URL="${BCA_URL:-http://127.0.0.1:4001}"
MANDIRI_URL="${MANDIRI_URL:-http://127.0.0.1:4002}"
EWALLET_URL="${EWALLET_URL:-http://127.0.0.1:4003}"

need() { command -v "$1" >/dev/null 2>&1 || { echo "e2e: missing required tool: $1" >&2; exit 1; }; }
need curl
need python3

echo "e2e: resetting bank profiles to normal + merchant mode 200"
for b in "$BCA_URL" "$MANDIRI_URL" "$EWALLET_URL"; do
  curl -sf -X POST "$b/admin/reset" >/dev/null
done
curl -sf -X PUT "$SINK_URL/admin/mode" -H 'Content-Type: application/json' -d '{"status":200}' >/dev/null

ORDER="e2e-$(date +%s)-$RANDOM"
echo "e2e: creating VA payment order=$ORDER"
CREATE_RESP=$(curl -sf -X POST "$GATEWAY_URL/v1/payments" \
  -H 'Content-Type: application/json' \
  -d "{\"merchant_id\":\"merchant-e2e\",\"order_id\":\"$ORDER\",\"amount\":50000,\"channel\":\"VA\"}")
echo "e2e: create -> $CREATE_RESP"
TXN_ID=$(python3 -c "import json,sys; print(json.load(sys.stdin)['id'])" <<< "$CREATE_RESP")

echo "e2e: polling status (up to 60s)"
STATUS=""
for _ in $(seq 1 60); do
  STATUS=$(curl -sf "$GATEWAY_URL/v1/payments/$TXN_ID" | python3 -c "import json,sys; print(json.load(sys.stdin)['status'])")
  echo "e2e: status=$STATUS"
  if [ "$STATUS" = "SUCCESS" ] || [ "$STATUS" = "FAILED" ]; then
    break
  fi
  sleep 1
done
if [ "$STATUS" != "SUCCESS" ] && [ "$STATUS" != "FAILED" ]; then
  echo "e2e FAIL: no final status within 60s (last=$STATUS)" >&2
  exit 1
fi

echo "e2e: checking merchant-sink received exactly 1 webhook for $TXN_ID"
COUNT=$(curl -sf "$SINK_URL/state" | python3 -c "
import json,sys
state = json.load(sys.stdin)
hits = [e for e in state['events'] if e['transaction_id'] == '$TXN_ID']
print(len(hits))
")
echo "e2e: webhook count for txn=$COUNT"
if [ "$COUNT" != "1" ]; then
  echo "e2e FAIL: expected exactly 1 webhook, got $COUNT" >&2
  exit 1
fi

echo "e2e: checking Envoy path :8080"
ENVOY_STATUS=$(curl -sf "$ENVOY_URL/v1/payments/$TXN_ID" | python3 -c "import json,sys; print(json.load(sys.stdin)['status'])")
echo "e2e: envoy status=$ENVOY_STATUS"
if [ "$ENVOY_STATUS" != "$STATUS" ]; then
  echo "e2e FAIL: envoy path disagrees ($ENVOY_STATUS != $STATUS)" >&2
  exit 1
fi

echo "e2e PASS: order=$ORDER id=$TXN_ID status=$STATUS webhooks=1 envoy=ok"
