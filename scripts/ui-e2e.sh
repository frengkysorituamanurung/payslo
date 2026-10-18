#!/usr/bin/env bash
# scripts/ui-e2e.sh — e2e Playwright vs `lab serve --demo`.
# Alur: ui-build -> go build (embed dist segar) -> install browser -> playwright.
# Server demo (dua instance: :8088 fungsional, :8089 visual) DIKELOLA Playwright
# lewat `webServer` di ui/playwright.config.ts, sehingga selalu ada instance
# segar untuk uji visual (state demo bisa berubah akibat mutasi spec lain).
#
# Non-zero bila ada langkah gagal.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

need() { command -v "$1" >/dev/null 2>&1 || { echo "ui-e2e: missing tool: $1" >&2; exit 1; }; }
need go
need npm

echo "ui-e2e: ui-build"
make ui-build >/dev/null || { echo "ui-e2e FAIL: ui-build" >&2; exit 1; }

echo "ui-e2e: go build labserve"
(cd cmd/lab && go build -o ../../bin/labserve .) || { echo "ui-e2e FAIL: go build" >&2; exit 1; }

echo "ui-e2e: install browser bila perlu (chromium, firefox, webkit)"
(cd ui && npx playwright install chromium firefox webkit) || {
  echo "ui-e2e FAIL: playwright install" >&2; exit 1;
}

echo "ui-e2e: playwright (serial: server demo dipakai bersama, worker=1)"
(cd ui && npx playwright test --workers=1) || {
  echo "ui-e2e FAIL: playwright" >&2; exit 1;
}

echo "ui-e2e PASS"
