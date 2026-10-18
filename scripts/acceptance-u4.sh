#!/usr/bin/env bash
# Acceptance U4 LIVE: bank-bca down -> insiden realtime -> diagnostik ->
# approval (ditahan) -> approve -> provider ter-disable -> pulih -> resolved.
# Timeline + postmortem + audit diverifikasi. Workspace/lab dikembalikan bersih.
set -euo pipefail
cd "$(dirname "$0")/.."

BASE=http://127.0.0.1:8088
ADMIN=http://127.0.0.1:3001
BANK=http://127.0.0.1:4001
PY=python3

cleanup() {
  # Selalu kembalikan bank + routing seperti semula (best effort).
  # TTL dikelola eksplisit di langkah 0/11 (rekreasi gateway).
  curl -s -X POST "$BANK/admin/reset" >/dev/null 2>&1 || true
  curl -s "$ADMIN/admin/routing" 2>/dev/null | $PY -c 'import sys,json
try:
  d=json.load(sys.stdin); d["bca"]["enabled"]=True; print(json.dumps(d))
except Exception: print("")' > /tmp/u4-routing-trap.json 2>/dev/null || true
  if [ -s /tmp/u4-routing-trap.json ]; then
    curl -s -X PUT "$ADMIN/admin/routing" -H 'Content-Type: application/json' -d @/tmp/u4-routing-trap.json >/dev/null 2>&1 || true
  fi
  kill "$SERVE_PID" 2>/dev/null || true
}
trap cleanup EXIT

need() { command -v "$1" >/dev/null 2>&1 || { echo "u4: missing $1" >&2; exit 1; }; }
need curl
need python3
need docker

wait_http() { # $1=url $2=nama
  for _ in $(seq 1 60); do
    if curl -sf --max-time 3 "$1" >/dev/null 2>&1; then return 0; fi
    sleep 2
  done
  echo "u4 FAIL: $2 tak kunjung up ($1)" >&2
  exit 1
}

echo "--- 0. prasyarat: stack + loadgen + TTL 60 (kanon skenario 01) ---"
curl -sf --max-time 5 "$ADMIN/admin/routing" >/dev/null || { echo "u4 FAIL: gateway admin mati"; exit 1; }
curl -sf --max-time 5 "$BANK/admin/profile" >/dev/null || { echo "u4 FAIL: bank-bca mati"; exit 1; }
if [ -z "$(docker compose --profile traffic ps -q loadgen 2>/dev/null)" ]; then
  echo "u4: loadgen mati, menyalakan profil traffic"
  RATE_PER_SEC=2 docker compose --profile traffic up -d loadgen
fi
echo "u4: TTL transaksi -> 60 dtk (seperti scenario start 01)"
GATEWAY_TXN_TTL_SECS=60 docker compose --profile traffic --profile obs up -d --force-recreate gateway-api >/dev/null
wait_http "http://127.0.0.1:3000/healthz" "gateway-publik"
wait_http "$BANK/admin/profile" "bank-bca"
wait_http "$ADMIN/admin/routing" "gateway-admin"

pkill -f 'bin/labserve serve' 2>/dev/null || true
sleep 1
./bin/labserve serve --addr 127.0.0.1:8088 >/tmp/labserve-u4-accept.log 2>&1 &
SERVE_PID=$!
for _ in $(seq 1 40); do
  curl -sf --max-time 2 "$BASE/" >/dev/null 2>&1 && break
  sleep 0.5
done
TOK=$(curl -s "$BASE/" | sed -n 's/.*window.__LAB_TOKEN__="\([^"]*\)".*/\1/p')
AUTH=(-H "Origin: $BASE" -H "X-Lab-Token: $TOK" -H "X-Lab-Actor: trainee")

echo "--- 0b. mulai bersih (tanpa insiden open) ---"
OPEN0=$(curl -s "$BASE/api/v1/incidents?status=open" | $PY -c 'import sys,json; print(len(json.load(sys.stdin)["incidents"]))')
if [ "$OPEN0" != "0" ]; then
  echo "u4 FAIL: ada $OPEN0 insiden open sebelum chaos (selesaikan dulu)"
  exit 1
fi
echo "bersih OK"

echo "--- 1. chaos: bank-bca down ---"
curl -sf -X PUT "$BANK/admin/profile" -H 'Content-Type: application/json' -d '{"profile":"down"}' | $PY -c 'import sys,json; print("profile:", json.load(sys.stdin)["profile"])'

echo "--- 2. tunggu insiden open (maks 6 mnt; TTL 60 + burn 5m/1m) ---"
INC=""
for _ in $(seq 1 72); do
  INC=$(curl -s "$BASE/api/v1/incidents?status=open" | $PY -c 'import sys,json
xs=[i for i in json.load(sys.stdin)["incidents"] if i["slo"]=="payment-success-rate"]
print(xs[0]["id"] if xs else "")')
  [ -n "$INC" ] && break
  sleep 5
done
[ -n "$INC" ] || { echo "u4 FAIL: insiden tak muncul"; exit 1; }
echo "incident: $INC"

echo "--- 3. detail: timeline opened + alert firing ---"
curl -s "$BASE/api/v1/incidents/$INC" | $PY -c 'import sys,json
d=json.load(sys.stdin)
kinds=[e["kind"] for e in d["timeline"]]
assert "opened" in kinds, kinds
assert any(a["status"]=="firing" for a in d["alerts"]), "tak ada alert firing"
print("timeline OK:", kinds)
print("runbook steps:", [(s["idx"],s["kind"]) for s in d["runbook"]["steps"]])
assert any(s["kind"]=="remediation" for s in d["runbook"]["steps"])'

echo "--- 4. diagnostik langkah 0 berjalan ---"
curl -s -X POST "$BASE/api/v1/incidents/$INC/runbook/steps/0/run" "${AUTH[@]}" | $PY -c 'import sys,json
d=json.load(sys.stdin); assert d["status"] in ("ok","no_data","source_down"), d; print("diagnostic OK:", d["status"])'
echo "--- 4b. remediation langsung DITOLAK 409 (tanpa jalan pintas) ---"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/api/v1/incidents/$INC/runbook/steps/4/run" "${AUTH[@]}")
if [ "$CODE" = "409" ]; then echo "langsung 409 OK"; else echo "u4 FAIL: run remediation harus 409, dapat $CODE"; exit 1; fi

echo "--- 5. minta approval ---"
AID=$(curl -s -X POST "$BASE/api/v1/incidents/$INC/approvals" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d '{"action":"disable provider bca","step":4}' | $PY -c 'import sys,json; d=json.load(sys.stdin); print(d["approval"]["id"]); assert d["approval"]["status"]=="pending"')
echo "approval: $AID"
DEDUP=$(curl -s -X POST "$BASE/api/v1/incidents/$INC/approvals" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d '{"action":"disable provider bca","step":4}' | $PY -c 'import sys,json; d=json.load(sys.stdin); print(d.get("deduped", False))')
if [ "$DEDUP" = "True" ]; then echo "dedup OK"; else echo "u4 FAIL: dedup expected"; exit 1; fi

echo "--- 6. reject tanpa reason -> 422; approve -> approved; ganda -> 409 ---"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/api/v1/approvals/$AID/decision" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d '{"decision":"rejected","by":"sinta"}')
if [ "$CODE" = "422" ]; then echo "reject-tanpa-reason 422 OK"; else echo "u4 FAIL: harus 422, dapat $CODE"; exit 1; fi
curl -s -X POST "$BASE/api/v1/approvals/$AID/decision" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d '{"decision":"approved","by":"sinta"}' | $PY -c 'import sys,json; d=json.load(sys.stdin); assert d["approval"]["status"]=="approved", d; print("approved OK")'
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/api/v1/approvals/$AID/decision" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d '{"decision":"rejected","by":"sinta","reason":"x"}')
if [ "$CODE" = "409" ]; then echo "keputusan ganda 409 OK"; else echo "u4 FAIL: harus 409, dapat $CODE"; exit 1; fi

echo "--- 7. provider bca ter-disable + timeline remediasi ---"
curl -s "$ADMIN/admin/routing" | $PY -c 'import sys,json; r=json.load(sys.stdin); assert r["bca"]["enabled"] is False, r; print("routing bca=false OK")'
curl -s "$BASE/api/v1/incidents/$INC" | $PY -c 'import sys,json
kinds=[e["kind"] for e in json.load(sys.stdin)["timeline"]]
assert "approval_approved" in kinds and "remediation" in kinds, kinds
print("timeline OK")'

echo "--- 8. catatan + postmortem + settings ---"
curl -s -X POST "$BASE/api/v1/incidents/$INC/notes" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d '{"text":"observasi: bca timeout massal"}' -o /dev/null -w "note HTTP %{http_code}\n"
curl -s -X PUT "$BASE/api/v1/settings" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d '{"traineeName":"sinta"}' | $PY -c 'import sys,json; assert json.load(sys.stdin)["traineeName"]=="sinta"; print("settings OK")'
curl -s -X PUT "$BASE/api/v1/incidents/$INC/postmortem" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d '{"status":"published","sections":{"summary":"bca down","impact":"VA gagal","timeline":"t0 down, t1 mitigasi","rootCause":"bank-bca Down","wentWell":"deteksi","wentWrong":"manual","actions":["auto-disable"]}}' \
  | $PY -c 'import sys,json; d=json.load(sys.stdin)
assert d["status"]=="published" and d["markdown"].startswith("# Postmortem"), d["markdown"][:80]
assert "- [ ] auto-disable" in d["markdown"]
print("postmortem OK")'

echo "--- 9. SSE memuat event incident + approval ---"
curl -sN --max-time 8 "$BASE/api/v1/events" | grep -E "^event: (incident|approval)$" | sort -u | head -4

echo "--- 10. pulih: bank normal -> tunggu resolved (maks 10 mnt) ---"
curl -s -X POST "$BANK/admin/reset" >/dev/null
for _ in $(seq 1 120); do
  ST=$(curl -s "$BASE/api/v1/incidents/$INC" | $PY -c 'import sys,json; print(json.load(sys.stdin)["incident"]["status"])')
  [ "$ST" = "resolved" ] && break
  sleep 5
done
if [ "$ST" = "resolved" ]; then echo "resolved OK"; else echo "u4 FAIL: insiden tak resolved"; exit 1; fi
curl -s "$BASE/api/v1/incidents/$INC" | $PY -c 'import sys,json
kinds=[e["kind"] for e in json.load(sys.stdin)["timeline"]]
assert "resolved" in kinds, kinds
print("timeline resolved OK")'

echo "--- 11. kembalikan routing bca enabled + TTL 600 ---"
CUR=$(curl -s "$ADMIN/admin/routing")
echo "$CUR" | $PY -c 'import sys,json; d=json.load(sys.stdin); d["bca"]["enabled"]=True; print(json.dumps(d))' > /tmp/u4-routing-restore.json
curl -s -X PUT "$ADMIN/admin/routing" -H 'Content-Type: application/json' -d @/tmp/u4-routing-restore.json | $PY -c 'import sys,json; r=json.load(sys.stdin); assert r["bca"]["enabled"] is True; print("routing restored OK")'
GATEWAY_TXN_TTL_SECS=600 docker compose --profile traffic --profile obs up -d --force-recreate gateway-api >/dev/null
wait_http "http://127.0.0.1:3000/healthz" "gateway-publik"
echo "TTL 600 restored OK"

echo "ACCEPTANCE U4 LIVE OK"
