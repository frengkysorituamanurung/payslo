#!/usr/bin/env bash
# Solver otomatis skenario 05.
set -euo pipefail
LAB_BIN="${LAB_BIN:-lab}"

"$LAB_BIN" scenario start 05 --force
# Beri waktu: kegagalan webhook tercatat.
sleep 60
# Mitigasi: kembalikan merchant ke 200.
"$LAB_BIN" chaos reset
# Satu pembayaran uji agar ada DELIVERED segar.
curl -sf -X POST 127.0.0.1:3000/v1/payments -H 'Content-Type: application/json' \
  -d "{\"merchant_id\":\"m-solver\",\"order_id\":\"s05-$(date +%s)-$$\",\"amount\":20000,\"channel\":\"EWALLET\"}" > /dev/null
sleep 20
"$LAB_BIN" answer diagnosis "webhook hilang di merchant-sink yang me-return 500, terbukti dari deliveries FAILED dan log retry gateway"
"$LAB_BIN" answer fix "kembalikan mode merchant ke 200 lalu kirim pembayaran uji hingga DELIVERED mengalir"
"$LAB_BIN" verify 05
