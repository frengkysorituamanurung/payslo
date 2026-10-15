# bank-sim

Simulasi bank/e-wallet. Satu binary, tiga instance via `PROVIDER_NAME`
(bank-bca :4001, bank-mandiri :4002, ewallet-sim :4003 → container port 4000).

## Cara run (satu instance)

```sh
PROVIDER_NAME=bca BANK_PORT=4001 GATEWAY_URL=http://127.0.0.1:3000 \
cargo run -p bank-sim
```

Healthcheck image: `bank-sim --healthcheck` (butuh `BANK_PORT` yang sama).

## Env var

| Var | Default | Arti |
|---|---|---|
| `PROVIDER_NAME` | bca | Nama provider (bca/mandiri/ewallet) |
| `BANK_PORT` | 4000 | Port listen |
| `PROFILE` | normal | normal/slow/flaky/down/ambiguous/late-callback/dup-callback |
| `ERROR_RATE` | 0.2 | Peluang gagal profil flaky [0,1] |
| `LATENCY_MS_MIN`/`LATENCY_MS_MAX` | 100/300 | Latensi profil flaky/late/dup |
| `CALLBACK_DELAY_S` | 1 | Tunda callback async |
| `DUPLICATE_CALLBACKS` | 1 | Jumlah callback profil dup (2–3) |
| `GATEWAY_URL` | 127.0.0.1:3000 | Tujuan callback |
| `SEED` | 42 | Seed `ChaCha8Rng` (reproducible; di-mix nama provider) |

## Profil

- **normal**: 100–300ms, sukses 99,5%.
- **slow**: 5–15s (melewati timeout gateway 5s).
- **flaky**: gagal acak 5xx/timeout sesuai `ERROR_RATE`.
- **down**: tidak pernah merespons (gateway timeout → 5xx, tidak hang).
- **ambiguous**: sukses tapi respons 7s (gateway timeout, callback sukses datang belakangan).
- **late-callback**: respons cepat, callback +20s.
- **dup-callback**: callback dikirim 2–3x (tidak di-dedup gateway).

## Contoh curl

```sh
curl -X POST 127.0.0.1:4001/charge -H 'Content-Type: application/json' \
  -d '{"transaction_id":"txn-1","provider":"bca","amount":50000,"channel":"VA"}'
# {"provider_ref":"P-bca-0001"}

curl 127.0.0.1:4001/status/P-bca-0001   # status sisi bank (trainee)

curl 127.0.0.1:4001/admin/profile        # lihat profil aktif
curl -X PUT 127.0.0.1:4001/admin/profile -H 'Content-Type: application/json' \
  -d '{"profile":"down"}'                # matikan bank (skenario)
curl -X POST 127.0.0.1:4001/admin/reset  # kembali ke default env
```
