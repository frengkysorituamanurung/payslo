#!/usr/bin/env bash
# Solver otomatis skenario 08.
set -euo pipefail
LAB_BIN="${LAB_BIN:-lab}"
PAYSLO_BIN="${PAYSLO_BIN:-./target/debug/payslo}"
export DD_API_KEY="${DD_API_KEY:-lab-only-dd-api-key}"

"$LAB_BIN" scenario start 08 --force
# Apply ke kedua backend (prometheus untuk konsistensi + datadog target).
"$PAYSLO_BIN" apply payslo-workspace/slo --workspace payslo-workspace --connection prometheus-local > /dev/null
"$PAYSLO_BIN" apply payslo-workspace/slo --workspace payslo-workspace --connection datadog-mock
# Buktikan idempotensi.
"$PAYSLO_BIN" plan payslo-workspace/slo --workspace payslo-workspace --connection datadog-mock
# Simulasi edit manual lalu deteksi drift (lalu sembuhkan).
MID=$(curl -s -H "DD-API-KEY: $DD_API_KEY" '127.0.0.1:8126/api/v1/monitor?tags=managed-by:payslo' | python3 -c "import json,sys; print(json.load(sys.stdin)[0]['id'])")
curl -sf -X PUT -H "DD-API-KEY: $DD_API_KEY" -H 'Content-Type: application/json' \
  "127.0.0.1:8126/api/v1/monitor/$MID" -d '{"message":"hand edit"}' > /dev/null
"$PAYSLO_BIN" drift --workspace payslo-workspace --connection datadog-mock
"$PAYSLO_BIN" apply payslo-workspace/slo --workspace payslo-workspace --connection datadog-mock > /dev/null
"$LAB_BIN" answer mapping "window lab 1h dipetakan ke timeframe 7d di datadog dengan warning eksplisit timeframes 7d 30d 90d"
"$LAB_BIN" answer drift "perubahan manual terdeteksi via payslo drift yang membandingkan hash: backend differs from last apply"
"$LAB_BIN" verify 08
