#!/usr/bin/env bash
# Solver otomatis skenario 04.
set -euo pipefail
LAB_BIN="${LAB_BIN:-lab}"

"$LAB_BIN" scenario start 04 --force
# Beri waktu: request mulai menggantung dan 5xx tercatat.
sleep 60
# Mitigasi: bebaskan lock.
"$LAB_BIN" chaos reset
"$LAB_BIN" answer diagnosis "AccessExclusiveLock di tabel transactions menahan semua query gateway, terbukti dari pg_locks dan pg_stat_activity"
"$LAB_BIN" answer fix "terminate backend pemegang lock via pg_terminate_backend; cegah dengan statement_timeout dan lock_timeout"
# Tunggu trafik segar pasca-pemulihan (jendela [2m] harus bersih).
sleep 210
"$LAB_BIN" verify 04
