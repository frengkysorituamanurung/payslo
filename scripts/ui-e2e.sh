#!/usr/bin/env bash
# scripts/ui-e2e.sh — e2e Playwright vs lab serve --demo (U0 acceptance).
# Alur: ui-build -> go build (embed dist segar) -> serve --demo di :8088 ->
# tunggu / -> playwright (axe + cek request loopback) -> matikan server.
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

echo "ui-e2e: install browser bila perlu"
(cd ui && npx playwright install chromium --only-shell 2>/dev/null || npx playwright install chromium) || {
  echo "ui-e2e FAIL: playwright install" >&2; exit 1;
}

echo "ui-e2e: start lab serve --demo :8088"
./bin/labserve serve --demo --addr 127.0.0.1:8088 >/tmp/labserve-demo.log 2>&1 &
SERVE_PID=$!
cleanup() { kill "$SERVE_PID" 2>/dev/null || true; }
trap cleanup EXIT

echo "ui-e2e: tunggu / (60 dtk)"
for _ in $(seq 1 60); do
  if curl -sf --max-time 2 http://127.0.0.1:8088/ >/dev/null 2>&1; then
    break
  fi
  sleep 1
  if ! kill -0 "$SERVE_PID" 2>/dev/null; then
    echo "ui-e2e FAIL: labserve mati:" >&2
    cat /tmp/labserve-demo.log >&2
    exit 1
  fi
done
curl -sf --max-time 5 http://127.0.0.1:8088/ >/dev/null || { echo "ui-e2e FAIL: / tidak up" >&2; exit 1; }

echo "ui-e2e: playwright (serial: satu server demo dipakai bersama)"
(cd ui && PLAYWRIGHT_BASE_URL=http://127.0.0.1:8088 npx playwright test --workers=1) || {
  echo "ui-e2e FAIL: playwright" >&2; exit 1;
}

echo "ui-e2e PASS"
