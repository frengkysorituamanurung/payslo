#!/usr/bin/env bash
# Solver otomatis skenario 06.
set -euo pipefail
LAB_BIN="${LAB_BIN:-lab}"

"$LAB_BIN" scenario start 06 --force
# Beri waktu: spike 5xx tercatat.
sleep 120
# Rollback ke versi baik.
"$LAB_BIN" deploy gateway --version good
"$LAB_BIN" answer diagnosis "regresi deploy gateway versi bad: 20 persen 500 cepat berimpit dengan anotasi deploy di grafana"
"$LAB_BIN" answer fix "rollback lab deploy gateway ke versi good lalu verifikasi rasio 5xx turun"
# Tunggu jendela [2m] bersih.
sleep 150
"$LAB_BIN" verify 06
