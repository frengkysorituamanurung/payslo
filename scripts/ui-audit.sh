#!/usr/bin/env bash
# scripts/ui-audit.sh — audit dependensi + lisensi + build UI + Go statis (U8).
#
# Menjalankan:
#   1. npm audit (gagal pada high/critical),
#   2. pemeriksaan lisensi npm (ui/scripts/check-licenses.mjs),
#   3. build UI + pemeriksaan anggaran bundle (ui/scripts/check-bundle.mjs),
#   4. Go: gofmt -l + go vet pada cmd/lab.
#
# Pakai: bash scripts/ui-audit.sh
# Env:
#   SKIP_AUDIT_NETWORK=1  lewati `npm audit` (mis. lingkungan tanpa jaringan),
#                         langkah lain tetap dijalankan.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

need() { command -v "$1" >/dev/null 2>&1 || { echo "ui-audit: missing tool: $1" >&2; exit 1; }; }
need node
need npm
need go

step() { echo "ui-audit: $1"; }

if [ "${SKIP_AUDIT_NETWORK:-0}" = "1" ]; then
  step "[1/4] npm audit dilewati (SKIP_AUDIT_NETWORK=1)"
else
  step "[1/4] npm audit (high/critical)"
  (cd ui && npm audit --audit-level=high)
fi

step "[2/4] lisensi dependensi npm"
(cd ui && node scripts/check-licenses.mjs)

step "[3/4] build UI + anggaran bundle"
(cd ui && npm run build >/dev/null && node scripts/check-bundle.mjs)

step "[4/4] Go: gofmt + go vet"
unformatted="$(gofmt -l cmd/lab)"
if [ -n "$unformatted" ]; then
  echo "ui-audit FAIL: gofmt perlu dijalankan pada:" >&2
  echo "$unformatted" >&2
  exit 1
fi
(cd cmd/lab && go vet ./...)

echo "ui-audit PASS"
