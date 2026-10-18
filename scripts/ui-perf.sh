#!/usr/bin/env bash
# scripts/ui-perf.sh — ukur performa UI (U8, docs/UI-SPEC.md PERFORMA).
#   1. build UI + pemeriksaan anggaran bundle (check-bundle.mjs),
#   2. ukur FCP/DCL lokal + verifikasi code splitting (perf.spec.ts),
#   3. (opsional) soak SSE: set PERF_SOAK_SECONDS=3600 untuk uji memori 1 jam.
#
# Server demo dikelola Playwright (webServer di ui/playwright.config.ts).
#
# Pakai: bash scripts/ui-perf.sh
# Env:
#   PERF_SOAK_SECONDS  > 0 untuk menjalankan soak SSE (default 0 = dilewati).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

need() { command -v "$1" >/dev/null 2>&1 || { echo "ui-perf: missing tool: $1" >&2; exit 1; }; }
need go
need npm

echo "ui-perf: ui-build"
make ui-build >/dev/null || { echo "ui-perf FAIL: ui-build" >&2; exit 1; }

echo "ui-perf: anggaran bundle"
(cd ui && node scripts/check-bundle.mjs) || { echo "ui-perf FAIL: anggaran bundle" >&2; exit 1; }

echo "ui-perf: go build labserve"
(cd cmd/lab && go build -o ../../bin/labserve .) || { echo "ui-perf FAIL: go build" >&2; exit 1; }

echo "ui-perf: playwright perf (PERF_SOAK_SECONDS=${PERF_SOAK_SECONDS:-0})"
(cd ui && PERF_SOAK_SECONDS="${PERF_SOAK_SECONDS:-0}" \
  npx playwright test perf.spec.ts --project=chromium --workers=1) \
  || { echo "ui-perf FAIL: playwright perf" >&2; exit 1; }

echo "ui-perf PASS"
