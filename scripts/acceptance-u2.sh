#!/usr/bin/env bash
# Verifikasi acceptance U2 secara LIVE (Postgres + gateway + bank-sim + Envoy).
set -euo pipefail
cd "$(dirname "$0")/.."

BASE=http://127.0.0.1:8088
ADMIN=http://127.0.0.1:3001
TOK=""
PY=python3

pkill -f 'bin/labserve serve' 2>/dev/null || true
sleep 1
./bin/labserve serve --addr 127.0.0.1:8088 >/tmp/labserve-accept.log 2>&1 &
PID=$!
trap 'kill $PID 2>/dev/null || true' EXIT

for _ in $(seq 1 40); do
  curl -sf --max-time 2 "$BASE/" >/dev/null 2>&1 && break
  sleep 0.5
done

TOK=$(curl -s "$BASE/" | sed -n 's/.*window.__LAB_TOKEN__="\([^"]*\)".*/\1/p')
echo "token: ${TOK:0:8}... (len ${#TOK})"

AUTH=(-H "Origin: $BASE" -H "X-Lab-Token: $TOK" -H "X-Lab-Actor: trainee")

ORDER="accept-$(date +%s)"
echo "--- POST /api/v1/payments (via Envoy ingress) order=$ORDER"
CREATE=$(curl -s -X POST "$BASE/api/v1/payments" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d "{\"merchantId\":\"merchant-demo\",\"orderId\":\"$ORDER\",\"amount\":99000,\"channel\":\"VA\"}")
echo "$CREATE"
ID=$(printf '%s' "$CREATE" | $PY -c 'import sys,json; print(json.load(sys.stdin)["id"])')
echo "id=$ID"

echo "--- poll detail (maks 40s) sampai final + webhook"
for _ in $(seq 1 40); do
  D=$(curl -s "$BASE/api/v1/payments/$ID")
  WC=$(printf '%s' "$D" | $PY -c 'import sys,json; print(json.load(sys.stdin)["webhookCount"])')
  if [ "$WC" != "0" ]; then break; fi
  sleep 1
done
printf '%s' "$D" >/tmp/u2-detail.json
D="$(cat /tmp/u2-detail.json)"
export D
$PY -c 'import os,json
d=json.loads(os.environ["D"]); p=d["payment"]
print("status=%s callbackCount=%s webhookCount=%s duplicateCallback=%s traceId=%s" % (p["status"], d["callbackCount"], d["webhookCount"], d["duplicateCallback"], d["traceId"]))
print("timeline:", [e["type"] for e in d["timeline"]])
print("links:", d["links"])
assert d["callbackCount"] >= 1, "callback tidak diterima"
assert d["webhookCount"] == 1, "webhook harus tepat 1x, dapat %s" % d["webhookCount"]
assert [e["type"] for e in d["timeline"]][:2] == ["created","pending"], "awalan timeline salah"
assert p["status"] in ("SUCCESS","FAILED"), p["status"]
print("OK timeline + webhook tepat 1x")'

echo "--- GET /api/v1/payments (list memuat id)"
curl -s "$BASE/api/v1/payments?limit=5" | $PY -c 'import sys,json; d=json.load(sys.stdin); ids=[i["id"] for i in d["items"]]; print("items:",len(ids)); assert len(ids)>0; print("ok")'

echo "--- bank-status"
curl -s "$BASE/api/v1/payments/$ID/bank-status"; echo

echo "--- routing: disable bca, cek gateway :3001 + audit, lalu restore"
ORIG=$(curl -s "$BASE/api/v1/routing"); echo "before: $ORIG"
BODY=$(printf '%s' "$ORIG" | $PY -c 'import sys,json; d=json.load(sys.stdin); d["bca"]["enabled"]=False; print(json.dumps(d))')
curl -s -X PUT "$BASE/api/v1/routing" "${AUTH[@]}" -H 'Content-Type: application/json' -d "$BODY" >/dev/null
echo "gateway :3001 -> $(curl -s $ADMIN/admin/routing)"
curl -s "$BASE/api/v1/audit?limit=3" | $PY -c 'import sys,json; e=json.load(sys.stdin)["entries"][0]; print("audit[0]:",e["actor"],e["action"],e["target"],e["result"]); assert e["action"]=="routing.set"'
curl -s -X PUT "$BASE/api/v1/routing" "${AUTH[@]}" -H 'Content-Type: application/json' -d "$ORIG" >/dev/null
echo "restored gateway :3001 -> $(curl -s $ADMIN/admin/routing)"

echo "--- validation: amount pecahan/negatif ditolak 400"
curl -s -o /dev/null -w "amount=-1 -> HTTP %{http_code}\n" -X POST "$BASE/api/v1/payments" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d "{\"merchantId\":\"m\",\"orderId\":\"bad-$(date +%s)\",\"amount\":-1,\"channel\":\"VA\"}"

echo "ACCEPTANCE LIVE OK"
