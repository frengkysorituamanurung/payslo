# gateway-api

Payment intake + bank routing + callbacks + merchant webhooks. Port 3000.

> P1: crate ini SENGAJA tanpa endpoint metrics Prometheus/instrumentasi metrik.
> SLI diambil dari Envoy + sql_exporter. Hanya log JSON + teruskan `traceparent`.
> P2: HTTP 200 **bukan** berarti sukses — pembayaran yang diterima meembalikan 200
> dengan `status: PENDING|FAILED`. Hanya error internal/timeout yang 5xx.

## Cara run (lokal, butuh Postgres)

```sh
DATABASE_URL=postgres://lab_app:lab-only-change-me@127.0.0.1:5432/payslo_lab \
MERCHANT_WEBHOOK_SECRET=lab-only-webhook-secret-change-me \
BANK_BCA_URL=http://127.0.0.1:4001 BANK_MANDIRI_URL=http://127.0.0.1:4002 \
EWALLET_URL=http://127.0.0.1:4003 MERCHANT_SINK_URL=http://127.0.0.1:5000 \
cargo run -p gateway-api
```

Via compose: `make up-lite`. Healthcheck image: `gateway-api --healthcheck`.

## Env var

| Var | Default | Arti |
|---|---|---|
| `GATEWAY_PORT` | 3000 | Port listen |
| `DATABASE_URL` | — (wajib) | Postgres app |
| `BANK_BCA_URL`/`BANK_MANDIRI_URL`/`EWALLET_URL` | 127.0.0.1:4001/2/3 | bank-sim |
| `MERCHANT_SINK_URL` | 127.0.0.1:5000 | merchant-sink |
| `MERCHANT_WEBHOOK_SECRET` | — (wajib) | HMAC webhook (lab-only) |
| `GATEWAY_TIMEOUT_SECS` | 5 | Timeout charge ke bank |
| `GATEWAY_TXN_TTL_SECS` | 600 | PENDING → EXPIRED |
| `GATEWAY_EXPIRY_INTERVAL_SECS` | 10 | Interval sweep expiry |
| `SEED` | 42 | Seed RNG lab |

## Contoh curl

```sh
# Buat pembayaran (200 PENDING, final via callback)
curl -X POST 127.0.0.1:3000/v1/payments -H 'Content-Type: application/json' \
  -d '{"merchant_id":"m1","order_id":"o-1","amount":50000,"channel":"VA"}'

# Cek status
curl 127.0.0.1:3000/v1/payments/<id>

# Routing: matikan bca (VA jadi 100% mandiri)
curl -X PUT 127.0.0.1:3000/admin/routing -H 'Content-Type: application/json' \
  -d '{"bca":{"enabled":false,"weight":60},"mandiri":{"enabled":true,"weight":40},"ewallet":{"enabled":true,"weight":100}}'

# Mode release buruk (20% 500 + latency, untuk skenario regresi)
curl -X PUT 127.0.0.1:3000/admin/release -H 'Content-Type: application/json' \
  -d '{"version":"bad"}'

curl 127.0.0.1:3000/healthz  # {"ok":true}
curl 127.0.0.1:3000/readyz   # cek koneksi DB
```

Bobot default: VA bca 60 / mandiri 40; EWALLET & QRIS ewallet 100.
Callback duplikat TIDAK di-dedup (`// LAB-FLAW:` di `routes.rs`) — skenario 03.
