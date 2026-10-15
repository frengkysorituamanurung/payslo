# merchant-sink

Simulasi merchant: menerima webhook gateway, verifikasi HMAC, mencatat ledger.
Port 5000.

Skema signature (sama dengan `gateway-api/src/webhook.rs`):

```text
signature = hex(hmac_sha256(secret, timestamp + "." + raw_body))
header: X-Signature: <signature>, X-Timestamp: <unix seconds>
```

Verifikasi constant-time (`subtle`). Tanpa toleransi umur timestamp (lab).

> LAB-FLAW: setiap webhook yang valid mengkredit ledger — duplikat MENGGANDAKAN
> saldo (disengaja, untuk skenario 03). Lihat `src/routes.rs`.

## Cara run

```sh
MERCHANT_SINK_PORT=5000 MERCHANT_WEBHOOK_SECRET=lab-only-webhook-secret-change-me \
cargo run -p merchant-sink
```

Healthcheck image: `merchant-sink --healthcheck`.

## Env var

| Var | Default | Arti |
|---|---|---|
| `MERCHANT_SINK_PORT` | 5000 | Port listen |
| `MERCHANT_WEBHOOK_SECRET` | lab-only default + peringatan | Secret HMAC (lab-only) |

## Contoh curl

```sh
curl 127.0.0.1:5000/state   # {"ledger":{"o-1":{"balance":50000,"credits":1}},"events":[...],"rejected":0}

# Mode gagal (latihan retry gateway): webhook berikut jadi 500
curl -X PUT 127.0.0.1:5000/admin/mode -H 'Content-Type: application/json' \
  -d '{"status":500,"latency_ms":0}'
curl -X PUT 127.0.0.1:5000/admin/mode -H 'Content-Type: application/json' \
  -d '{"status":200}'
```
