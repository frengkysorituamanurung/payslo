#!/usr/bin/env bash
# scripts/smoke.sh — smoke test FASE 8 (final, full pipeline).
#
# Dari kondisi bersih:
#   1. up-lite (build lokal) ATAU up-prebuilt (image GHCR ber-tag versi),
#   2. tunggu semua service healthy,
#   2b. verifikasi konsol `lab serve` (health, token disuntikkan, guard mutasi),
#   3. kirim traffic (e2e-basic: pembayaran -> callback -> webhook via Envoy),
#   4. apply SLO (payslo -> Prometheus rules + datadog-mock),
#   5. picu skenario 01 dengan solver otomatis,
#   6. pastikan incident + notifikasi ntfy + pemulihan (resolved),
#   7. down.
#
# Keluar non-zero jika ADA langkah gagal. Dirancang untuk CI dengan batas waktu
# (timeout-minutes di workflow). Durasi tipikal: up+build ~8 mnt (cold) +
# solver ~8 mnt (sleep 300+180) + tunggu burn 1h reda ~55-60 mnt = ~75 mnt.
#
# Env:
#   SMOKE_MODE=lite|prebuilt   (default: lite)
#   PAYSLO_IMAGE_TAG=vX.Y.Z    (hanya untuk prebuilt; default v0.1.0)
#   PAYSLO_REGISTRY=ghcr.io/<org> (hanya untuk prebuilt)
#   SMOKE_KEEP_UP=1            (lewati down di akhir, untuk debug)
#   LAB_BIN                    (default: ./target/debug/lab, di-build bila perlu)
#   DD_API_KEY                 (default: lab-only-dd-api-key)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

MODE="${SMOKE_MODE:-lite}"
LAB_BIN="${LAB_BIN:-./target/debug/lab}"
export DD_API_KEY="${DD_API_KEY:-lab-only-dd-api-key}"
export NTFY_TOPIC="${NTFY_TOPIC:-payslo-lab}"

T0=$(date +%s)
step_t0=$T0
step() {
  local now
  now=$(date +%s)
  echo "smoke: [step $1] $2 (+$((now - step_t0))s, total +$((now - T0))s)"
  step_t0=$now
}
fail() { echo "smoke FAIL: $1" >&2; exit 1; }

# Diagnostik SLO:cetak rasio error, status alert Prometheus/Alertmanager, dan
# incident PaySLO. Dipakai saat timeout supaya kegagalan bisa ditelusuri tanpa
# harus reproduce manual.
dump_slo_diag() {
  echo "smoke: --- diagnostik SLO ---"
  for w in 1m 5m 10m 15m 1h; do
    v="$(curl -sfG --data-urlencode "query=payslo:slo_error_ratio:payment_success_rate:$w" \
      http://127.0.0.1:9090/api/v1/query 2>/dev/null \
      | python3 -c 'import json,sys
r=json.load(sys.stdin)["data"]["result"]
print(r[0]["value"][1] if r else "n/a")' 2>/dev/null || true)"
    echo "smoke: error ratio ${w} = ${v:-n/a}"
  done
  echo "smoke: alert prometheus:"
  curl -sf http://127.0.0.1:9090/api/v1/alerts 2>/dev/null \
    | python3 -c 'import json,sys
for a in json.load(sys.stdin)["data"]["alerts"]:
    print("  ", a["labels"].get("slo"), a["labels"].get("window"), a["state"])' 2>/dev/null || true
  echo "smoke: alert alertmanager:"
  curl -sf http://127.0.0.1:9093/api/v2/alerts 2>/dev/null \
    | python3 -c 'import json,sys
for a in json.load(sys.stdin):
    print("  ", a["labels"].get("slo"), a["labels"].get("window"), a["status"]["state"])' 2>/dev/null || true
  echo "smoke: incident payslo:"
  curl -sf http://127.0.0.1:8081/incidents 2>/dev/null || true
  echo
  echo "smoke: --- akhir diagnostik ---"
}
need() { command -v "$1" >/dev/null 2>&1 || fail "missing required tool: $1"; }

# Bersihkan server console (lab serve) bila start saat skrip keluar lebih awal.
SERVE_PID=""
cleanup_serve() { [ -n "$SERVE_PID" ] && kill "$SERVE_PID" 2>/dev/null || true; }
trap cleanup_serve EXIT

need docker
need curl
need python3
need cargo
docker compose version >/dev/null 2>&1 || fail "docker compose v2 tidak tersedia"

if [ "$MODE" = "prebuilt" ]; then
  COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.prebuilt.yml
    --profile traffic --profile obs --profile payslo)
else
  COMPOSE=(docker compose --profile traffic --profile obs --profile payslo)
fi

# --- 0. kondisi bersih (idempotent; aman dijalankan ulang) --------------------
step 0 "reset bersih (down -v)"
# down -v menghapus volume (DB seed + state payslo) sehingga run berikutnya
# deterministik; up berikutnya me-seed ulang otomatis via initdb + migrasi.
"${COMPOSE[@]}" --profile obs-full down -v || fail "compose down -v gagal"

# --- 1. up dari kondisi bersih ---------------------------------------------
step 1 "up (mode=$MODE)"
if [ "$MODE" = "prebuilt" ]; then
  "${COMPOSE[@]}" pull || fail "compose pull (prebuilt) gagal"
  "${COMPOSE[@]}" up -d || fail "compose up (prebuilt) gagal"
else
  "${COMPOSE[@]}" up -d --build || fail "compose up --build (lite) gagal"
fi

# --- 2. tunggu semua healthy (deadline 10 mnt) ------------------------------
step 2 "tunggu healthy (deadline 600s)"
deadline=$(( $(date +%s) + 600 ))
while :; do
  notready=$("${COMPOSE[@]}" ps --format '{{.Name}}|{{.Health}}|{{.State}}' 2>/dev/null \
    | python3 -c "
import sys
bad = [l.split('|')[0] for l in sys.stdin if l.strip() and (l.strip().split('|')[2] != 'running' or (l.strip().split('|')[1] not in ('', 'healthy')))]
print(' '.join(bad))" || true)
  http_ok=1
  curl -sf --max-time 3 http://127.0.0.1:3000/healthz >/dev/null 2>&1 || http_ok=0
  curl -sf --max-time 3 http://127.0.0.1:8081/incidents >/dev/null 2>&1 || http_ok=0
  curl -sf --max-time 3 http://127.0.0.1:8126/healthz >/dev/null 2>&1 || http_ok=0
  curl -sf --max-time 3 http://127.0.0.1:8022/ >/dev/null 2>&1 || http_ok=0
  curl -sf --max-time 3 http://127.0.0.1:9090/-/healthy >/dev/null 2>&1 || http_ok=0
  if [ -z "$notready" ] && [ "$http_ok" = "1" ]; then
    echo "smoke: semua healthy + HTTP ok"
    break
  fi
  [ "$(date +%s)" -gt "$deadline" ] && {
    echo "smoke: service belum healthy:" >&2
    "${COMPOSE[@]}" ps >&2
    fail "timeout menunggu healthy"
  }
  sleep 10
done

# --- 2b. console `lab serve` (U8): health + guard mutasi ----------------------
# Memverifikasi konsol UI (embed ui/dist) hidup, menyuntikkan token nonce, dan
# menegakkan guard mutasi: TANPA token 403, DENGAN token (bukan 403).
step "2b" "console: lab serve health + guard mutasi"
if [ ! -f cmd/lab/web/dist/index.html ]; then
  echo "smoke: UI embed belum dibuild -> make ui-build"
  make ui-build >/dev/null || fail "make ui-build gagal"
fi
(cd cmd/lab && go build -o ../../bin/labserve .) || fail "go build labserve gagal"
./bin/labserve serve --addr 127.0.0.1:8088 >/tmp/labserve-smoke.log 2>&1 &
SERVE_PID=$!
serve_deadline=$(( $(date +%s) + 60 ))
while ! curl -sf --max-time 2 http://127.0.0.1:8088/ >/dev/null 2>&1; do
  kill -0 "$SERVE_PID" 2>/dev/null || { cat /tmp/labserve-smoke.log >&2; fail "labserve mati saat start"; }
  [ "$(date +%s)" -gt "$serve_deadline" ] && { cat /tmp/labserve-smoke.log >&2; fail "labserve timeout up"; }
  sleep 1
done
PAGE="$(curl -sf --max-time 10 http://127.0.0.1:8088/)"
TOKEN="$(printf '%s' "$PAGE" | sed -n 's/.*window.__LAB_TOKEN__="\([^"]*\)".*/\1/p' | head -1)"
[ -n "$TOKEN" ] || fail "token tidak disuntikkan ke halaman console"
# SetTraffic menjalankan `docker compose up -d loadgen`; di runner CI yang
# sibuk ini bisa >10 dtk. Timeout curl tidak boleh membunuh skrip (exit 28);
# kode 000 akan gagal dengan pesan eksplisit di bawah.
code_no="$(curl -s -o /dev/null -w '%{http_code}' --max-time 60 -X PUT \
  -H "Origin: http://127.0.0.1:8088" -H "Content-Type: application/json" \
  --data '{"ratePerSec":5,"source":"loadgen"}' http://127.0.0.1:8088/api/v1/traffic || true)"
code_yes="$(curl -s -o /dev/null -w '%{http_code}' --max-time 60 -X PUT \
  -H "Origin: http://127.0.0.1:8088" -H "X-Lab-Token: $TOKEN" -H "Content-Type: application/json" \
  --data '{"ratePerSec":5,"source":"loadgen"}' http://127.0.0.1:8088/api/v1/traffic || true)"
kill "$SERVE_PID" 2>/dev/null || true; SERVE_PID=""
echo "smoke: lab serve tanpa token=$code_no, dengan token=$code_yes"
[ "$code_no" = "403" ] || fail "mutasi tanpa token harus 403 (dapat $code_no)"
[ "$code_yes" != "403" ] || fail "mutasi dengan token ditolak 403"

# --- 3. traffic: pembayaran -> callback -> webhook (via Envoy) ----------------
step 3 "traffic (e2e-basic.sh)"
bash scripts/e2e-basic.sh || fail "e2e-basic gagal"

# --- 4. apply SLO ------------------------------------------------------------
step 4 "apply SLO (payslo -> prometheus + datadog-mock)"
cargo run -q -p payslo-cli -- validate payslo-workspace/slo --workspace payslo-workspace \
  || fail "payslo validate gagal"
cargo run -q -p payslo-cli -- apply payslo-workspace/slo --workspace payslo-workspace \
  || fail "payslo apply gagal"
# Pastikan rules termuat di Prometheus (adapter me-reload via /-/reload).
rules=$(curl -sf http://127.0.0.1:9090/api/v1/rules | python3 -c "
import json,sys
d = json.load(sys.stdin)
names = [g['name'] for g in d['data']['groups']]
print(' '.join(names))
") || fail "prometheus /api/v1/rules tidak terbaca"
echo "smoke: rule groups: $rules"
case "$rules" in
  *payslo-payment-success-rate*) echo "smoke: SLO payment-success-rate termuat" ;;
  *) fail "rule group payslo-payment-success-rate tidak ada di Prometheus" ;;
esac
# Pastikan SLO mirror ada di datadog-mock.
dd_count=$(curl -sf -H "DD-API-KEY: $DD_API_KEY" http://127.0.0.1:8126/api/v1/slo \
  | python3 -c "import json,sys; print(len(json.load(sys.stdin)['data']))") \
  || fail "datadog-mock /api/v1/slo tidak terbaca"
echo "smoke: datadog-mock SLO count=$dd_count"
[ "$dd_count" -ge 1 ] || fail "datadog-mock tidak berisi SLO hasil apply"

# --- 5. skenario 01 dengan solver otomatis ------------------------------------
step 5 "scenario 01 (solver otomatis, ~8 mnt)"
if [ ! -x "$LAB_BIN" ]; then
  echo "smoke: build lab CLI dulu"
  cargo build -p lab || fail "cargo build -p lab gagal"
fi
# Batas bawah resolved_at insiden: hanya insiden yang REDA di run ini yang
# diakui (verify 01 menerima insiden lama yang sudah resolved; reopen
# mempertahankan created_at lama, jadi created_at tidak bisa dipakai).
SOLVER_T0=$(python3 -c "import datetime; print((datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(seconds=60)).strftime('%Y-%m-%dT%H:%M:%SZ'))")
LAB_BIN="$LAB_BIN" bash scenarios/01/solve.sh || fail "solver skenario 01 gagal (verify tidak PASS)"

# --- 6. incident + notifikasi + pemulihan --------------------------------------
# Pemulihan penuh (incident resolved) mengikuti burn paling lambat 1h/10m:
# empiris FASE 8: ~10 mnt (periode buruk pendek) hingga ~57 mnt (periode
# buruk panjang). Poll hingga 75 mnt (batas bawah CI agar tidak flapping di
# runner lambat; job CI punya timeout 120 mnt).
step 6 "tunggu pemulihan + verifikasi incident/notifikasi (deadline 4500s)"
deadline=$(( $(date +%s) + 4500 ))
poll=0
while :; do
  INC_JSON=$(curl -sf http://127.0.0.1:8081/incidents) || fail "payslo /incidents tidak terbaca"
  if echo "$INC_JSON" | SOLVER_T0="$SOLVER_T0" python3 -c "
import json,sys,os
t0 = os.environ['SOLVER_T0']
incs = json.load(sys.stdin)['incidents']
rec = [i for i in incs if i.get('slo') == 'payment-success-rate' and i.get('resolvedAt') and i.get('resolvedAt','') >= t0]
sys.exit(0 if rec else 1)
"; then
    echo "smoke: incident run ini resolved"
    break
  fi
  poll=$((poll + 1))
  if [ $((poll % 10)) -eq 0 ]; then
    dump_slo_diag
  fi
  if [ "$(date +%s)" -gt "$deadline" ]; then
    dump_slo_diag
    fail "timeout menunggu incident resolved (slow burn belum reda)"
  fi
  echo "smoke: incident belum resolved, tunggu 30s..."
  sleep 30
done
echo "$INC_JSON" | SOLVER_T0="$SOLVER_T0" python3 -c "
import json,sys,os
t0 = os.environ['SOLVER_T0']
incs = json.load(sys.stdin)['incidents']
rec = [i for i in incs if i.get('slo') == 'payment-success-rate' and i.get('resolvedAt') and i.get('resolvedAt','') >= t0]
print('incident ok:', rec[0]['id'], 'resolved_at=', rec[0]['resolvedAt'])
" || fail "assertion incident gagal"
INC_ID=$(echo "$INC_JSON" | SOLVER_T0="$SOLVER_T0" python3 -c "
import json,sys,os
t0 = os.environ['SOLVER_T0']
incs = json.load(sys.stdin)['incidents']
rec = [i for i in incs if i.get('slo') == 'payment-success-rate' and i.get('resolvedAt') and i.get('resolvedAt','') >= t0]
print(rec[0]['id'])")
# Notifikasi: pesan ntfy untuk incident tersebut harus ada di cache topik.
# Catatan: endpoint /json ntfy adalah stream yang tidak pernah selesai sendiri,
# jadi curl SELALU keluar via --max-time (exit 28); yang diassert adalah isinya.
NTFY_JSON=$(curl -s --max-time 8 "http://127.0.0.1:8022/$NTFY_TOPIC/json?since=all" || true)
[ -n "$NTFY_JSON" ] || fail "ntfy topic $NTFY_TOPIC kosong/tidak terbaca"
echo "$NTFY_JSON" | python3 -c "
import json,sys
found = [json.loads(l) for l in sys.stdin if l.strip().startswith('{') and '$INC_ID' in l]
assert found, 'tidak ada notifikasi ntfy untuk incident $INC_ID'
print('ntfy ok:', len(found), 'pesan untuk', '$INC_ID')
" || fail "assertion notifikasi ntfy gagal"

# --- 7. down -------------------------------------------------------------------
step 7 "down"
if [ "${SMOKE_KEEP_UP:-0}" = "1" ]; then
  echo "smoke: SMOKE_KEEP_UP=1, lewati down"
else
  "${COMPOSE[@]}" --profile obs-full down || fail "compose down gagal"
  leftover=$(docker compose --profile traffic --profile obs --profile obs-full --profile payslo ps -q 2>/dev/null || true)
  [ -z "$leftover" ] || fail "masih ada container berjalan setelah down: $leftover"
fi

T1=$(date +%s)
echo "smoke PASS (mode=$MODE, total $((T1 - T0))s, id=$INC_ID)"
