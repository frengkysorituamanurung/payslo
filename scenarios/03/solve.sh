#!/usr/bin/env bash
# Solver otomatis skenario 03.
set -euo pipefail
LAB_BIN="${LAB_BIN:-lab}"

"$LAB_BIN" scenario start 03 --force
# Beri waktu duplikat terjadi.
sleep 90
# Mitigasi: hentikan sumber duplikat.
"$LAB_BIN" chaos reset
# Satu pembayaran uji agar ada trafik segar (order_id unik per run).
curl -sf -X POST 127.0.0.1:3000/v1/payments -H 'Content-Type: application/json' \
  -d "{\"merchant_id\":\"m-solver\",\"order_id\":\"s03-$(date +%s)-$$\",\"amount\":20000,\"channel\":\"EWALLET\"}" > /dev/null
sleep 30
"$LAB_BIN" answer diagnosis "callback duplikat dari ewallet dup-callback: merchant menerima webhook 2x sehingga saldo ganda"
"$LAB_BIN" answer fix "reset bank ke normal untuk hentikan duplikat; permanen: idempotency di merchant dengan reset via chaos"
"$LAB_BIN" verify 03
