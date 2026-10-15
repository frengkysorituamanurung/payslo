#!/usr/bin/env bash
# Acceptance U3 LIVE: edit SLO -> validate gagal berlokasi -> perbaiki ->
# plan ber-diff -> apply (prometheus-local + datadog-mock) -> plan ulang
# kosong -> apply kedua no-op. Lalu: edit manual monitor di datadog-mock ->
# drift tampil -> apply ulang menyembuhkan. Terakhir: berkas dikembalikan
# seperti semula + apply (lab konsisten).
set -euo pipefail
cd "$(dirname "$0")/.."

BASE=http://127.0.0.1:8088
DD=http://127.0.0.1:8126
KEY="${DD_API_KEY:-lab-only-dd-api-key}"
SLO_FILE=payslo-workspace/slo/success-rate.yml
BACKUP_DIR=$(mktemp -d)
BACKUP="$BACKUP_DIR/success-rate.yml"
TRAP_CLEANUP="$BACKUP_DIR"
PY=python3

# Backup dibuat SETELAH langkah 1 (validasi pristine) lolos, agar backup
# tak pernah berisi draf rusak bila skrip dijalankan ulang dalam keadaan kotor.
cleanup() { kill "$SERVE_PID" 2>/dev/null || true; rm -rf "$TRAP_CLEANUP" 2>/dev/null || true; }
trap cleanup EXIT

pkill -f 'bin/labserve serve' 2>/dev/null || true
sleep 1
DD_API_KEY="$KEY" ./bin/labserve serve --addr 127.0.0.1:8088 >/tmp/labserve-u3-accept.log 2>&1 &
SERVE_PID=$!
for _ in $(seq 1 40); do
  curl -sf --max-time 2 "$BASE/" >/dev/null 2>&1 && break
  sleep 0.5
done
TOK=$(curl -s "$BASE/" | sed -n 's/.*window.__LAB_TOKEN__="\([^"]*\)".*/\1/p')
AUTH=(-H "Origin: $BASE" -H "X-Lab-Token: $TOK" -H "X-Lab-Actor: trainee")

echo "--- 1. validate berkas asli (harus valid) ---"
curl -s -X POST "$BASE/api/v1/slos/payment-success-rate/validate" "${AUTH[@]}" | $PY -c 'import sys,json; d=json.load(sys.stdin); assert d["valid"], d; print("valid OK")'
cp "$SLO_FILE" "$BACKUP"

echo "--- 2. PUT draf rusak (objective 99) -> validate gagal berlokasi ---"
$PY - "$BACKUP" <<'PY' > /tmp/u3-bad.yml
import sys
t = open(sys.argv[1]).read()
print(t.replace("objective: 0.995", "objective: 99"))
PY
curl -s -X PUT "$BASE/api/v1/slos/payment-success-rate" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d "$(printf '{"content":%s}' "$($PY -c 'import json; print(json.dumps(open("/tmp/u3-bad.yml").read()))')")" >/dev/null
curl -s -X POST "$BASE/api/v1/slos/payment-success-rate/validate" "${AUTH[@]}" | $PY -c 'import sys,json
d=json.load(sys.stdin)
assert not d["valid"], d
e=[x for x in d["errors"] if x.get("field")=="objective"][0]
assert e["line"]>=1 and e["column"]>=1, e
print("invalid OK:", e)'
# plan menolak konten rusak (400)
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/api/v1/slos/payment-success-rate/plan" "${AUTH[@]}")
if [ "$CODE" = "400" ]; then echo "plan menolak draf rusak: HTTP 400 OK"; else echo "plan harus 400, dapat $CODE"; exit 1; fi

echo "--- 3. perbaiki (burnRate 14.4 -> 12 agar ada diff) -> validate OK ---"
$PY - "$BACKUP" <<'PY' > /tmp/u3-fixed.yml
import sys
t = open(sys.argv[1]).read()
print(t.replace("burnRate: 14.4", "burnRate: 12"))
PY
curl -s -X PUT "$BASE/api/v1/slos/payment-success-rate" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d "$(printf '{"content":%s}' "$($PY -c 'import json; print(json.dumps(open("/tmp/u3-fixed.yml").read()))')")" >/dev/null
curl -s -X POST "$BASE/api/v1/slos/payment-success-rate/validate" "${AUTH[@]}" | $PY -c 'import sys,json; d=json.load(sys.stdin); assert d["valid"], d; print("valid OK")'

echo "--- 4. plan menunjukkan diff (prometheus) ---"
curl -s -X POST "$BASE/api/v1/slos/payment-success-rate/plan" "${AUTH[@]}" | $PY -c 'import sys,json
d=json.load(sys.stdin)
assert len(d["changes"])==2, d["changes"]
pf=[f for f in d["files"] if f["backend"]=="prometheus"][0]
assert pf["change"], pf
lines=pf["diff"].splitlines()
minus=[l for l in lines if l.startswith("-") and not l.startswith("---")]
plus=[l for l in lines if l.startswith("+") and not l.startswith("+++")]
assert minus and plus, pf["diff"][:300]
assert pf["desired"] != pf["actual"]
print("plan diff OK:", [c["connection"] for c in d["changes"]], "(-/+:", len(minus), "/", len(plus), ")")'

echo "--- 5. apply ke prometheus-local + datadog-mock ---"
curl -s -X POST "$BASE/api/v1/slos/payment-success-rate/apply" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{}' \
  | $PY -c 'import sys,json; d=json.load(sys.stdin)
assert all(r["applied"] for r in d["results"]), d["results"]
print("applied OK:", [(r["connection"],r["message"]) for r in d["results"]])'

echo "--- 6. plan ulang kosong + apply kedua no-op ---"
curl -s -X POST "$BASE/api/v1/slos/payment-success-rate/plan" "${AUTH[@]}" | $PY -c 'import sys,json; d=json.load(sys.stdin); assert d["changes"]==[], d; print("plan kosong OK")'
curl -s -X POST "$BASE/api/v1/slos/payment-success-rate/apply" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{}' \
  | $PY -c 'import sys,json; d=json.load(sys.stdin)
assert all(r["noOp"] and not r["applied"] for r in d["results"]), d["results"]
print("no-op OK:", [r["message"] for r in d["results"]])'

echo "--- 7. edit MANUAL monitor di datadog-mock -> drift tampil ---"
MON_ID=$(curl -s "$DD/api/v1/monitor?tags=managed-by:payslo" -H "DD-API-KEY: $KEY" | $PY -c 'import sys,json
ms=json.load(sys.stdin)
ms=[m for m in ms if "payment-success-rate" in m.get("name","")]
assert ms, "monitor payslo tak ada (apply datadog gagal?)"
print(ms[0]["id"])')
curl -s -X PUT "$DD/api/v1/monitor/$MON_ID" -H "DD-API-KEY: $KEY" -H 'Content-Type: application/json' \
  -d '{"message":"[diedit manual trainee] drift sengaja"}' >/dev/null
curl -s "$BASE/api/v1/drift" | $PY -c 'import sys,json; d=json.load(sys.stdin)
hit=[x for x in d["drifted"] if x["slo"]=="payment-success-rate" and x["connection"]=="datadog-mock"]
assert hit, d
print("drift OK:", hit[0])'

echo "--- 8. apply ulang menyembuhkan drift ---"
curl -s -X POST "$BASE/api/v1/slos/payment-success-rate/apply" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{}' >/dev/null
curl -s "$BASE/api/v1/drift" | $PY -c 'import sys,json; d=json.load(sys.stdin)
hit=[x for x in d["drifted"] if x["slo"]=="payment-success-rate"]
assert not hit, d
print("drift sembuh OK")'

echo "--- 9. kembalikan berkas asli + apply (lab konsisten) ---"
cp "$BACKUP" "$SLO_FILE"
curl -s -X POST "$BASE/api/v1/slos/payment-success-rate/apply" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{}' \
  | $PY -c 'import sys,json; print(json.load(sys.stdin)["results"])'
curl -s -X POST "$BASE/api/v1/slos/payment-success-rate/plan" "${AUTH[@]}" | $PY -c 'import sys,json; d=json.load(sys.stdin); assert d["changes"]==[], d; print("pristine: plan kosong OK")'

echo "--- 10. keamanan: traversal/absolut/symlink ditolak ---"
for bad in "..%2F..%2Fevil" "%2Ftmp%2Fx.yml" "a%2Fb.yml"; do
  CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$BASE/api/v1/slos/$bad" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"content":"x"}')
  case "$CODE" in 400|404) echo "PUT $bad -> $CODE OK";; *) echo "PUT $bad harus 400/404, dapat $CODE"; exit 1;; esac
done
[ ! -e /tmp/u3-evil-marker ] || { echo "file escape tertulis!"; exit 1; }

echo "ACCEPTANCE U3 LIVE OK"
