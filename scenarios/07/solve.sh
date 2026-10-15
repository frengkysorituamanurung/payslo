#!/usr/bin/env bash
# Solver otomatis skenario 07.
set -euo pipefail
LAB_BIN="${LAB_BIN:-lab}"

"$LAB_BIN" scenario start 07 --force
# Beri waktu: fast-burn flap beberapa siklus.
sleep 180
# Mitigasi akar masalah (bukan alertnya).
"$LAB_BIN" chaos reset
"$LAB_BIN" answer diagnosis "fast burn window 5m flapping sementara slow burn 1h tenang: error kecil intermiten dari ewallet flaky, bukan outage"
"$LAB_BIN" answer tuning "reset bank ke normal; tuning yang diusulkan: naikkan threshold page 14.4 ke 20 dan wajibkan slow window ikut menyala"
sleep 90
"$LAB_BIN" verify 07
