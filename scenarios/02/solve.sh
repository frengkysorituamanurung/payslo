#!/usr/bin/env bash
# Solver otomatis skenario 02.
set -euo pipefail
LAB_BIN="${LAB_BIN:-lab}"

"$LAB_BIN" scenario start 02 --force
# Beri waktu: gauge pending (>5 mnt) butuh penumpukan.
sleep 420
# Mitigasi: kembalikan timing normal kedua bank.
"$LAB_BIN" chaos reset
# Jawaban diagnosis.
"$LAB_BIN" answer diagnosis "mandiri late-callback callback_delay 300 detik sehingga pending stuck; bca ambiguous respons 7 detik melebihi timeout gateway"
"$LAB_BIN" answer fix "reset kedua bank ke normal via chaos reset, aman karena hanya mengembalikan timing dan callback in-flight tetap mendarat"
# Tunggu drain + trafik segar.
sleep 180
"$LAB_BIN" verify 02
