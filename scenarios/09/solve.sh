#!/usr/bin/env bash
# Solver game day: seed tetap -> skenario tetap -> delegasikan ke solvernya.
set -euo pipefail
LAB_BIN="${LAB_BIN:-lab}"
SEED="${SEED:-7}"

PICK=$("$LAB_BIN" scenario random --seed "$SEED" 2>&1 | grep -oE 'random pick \(seed [0-9]+\): [0-9]+' | grep -oE '[0-9]+$' || true)
if [ -z "${PICK:-}" ]; then
  # random sudah start bila belum ada sesi aktif; baca sesi aktif.
  PICK=$(ls payslo-workspace/lab-sessions/*.json 2>/dev/null | head -n 1 | grep -oE '[0-9]+' || true)
fi
echo "game day pick: $PICK (seed $SEED)"
"$LAB_BIN" scenario stop "$PICK" > /dev/null 2>&1 || true
LAB_BIN="$LAB_BIN" bash "scenarios/$PICK/solve.sh"
