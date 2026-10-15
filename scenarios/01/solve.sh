#!/usr/bin/env bash
# Solver otomatis skenario 01 (meniru trainee ideal). Butuh stack + payslo service.
# Usage: LAB_BIN=lab ./solve.sh
set -euo pipefail
LAB_BIN="${LAB_BIN:-lab}"

"$LAB_BIN" scenario start 01 --force
# Tunggu burn alert + incident terbentuk (TTL 60s -> EXPIRED, burn 5m/1m).
sleep 300
# Mitigasi: pindahkan VA dari bca yang down.
"$LAB_BIN" routing disable bca
# Jawaban diagnosis.
"$LAB_BIN" answer diagnosis "Provider bca down: success rate bca 0 di dashboard, log gateway bank charge timed out, profil bank down"
"$LAB_BIN" answer divergence "HTTP 200/2xx hanya berarti gateway merespons, bukan pembayaran sukses (P2); timeout bca me-return 503 cepat lalu pulih setelah routing dipindah"
# Tunggu trafik segar pasca-mitigasi (jendela [2m] harus bersih dari EXPIRED).
sleep 180
"$LAB_BIN" verify 01
