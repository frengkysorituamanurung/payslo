# datadog-mock

Tiruan subset Datadog API v1 (SLO + monitor). **BUKAN Datadog asli.**
Port 8126.

## Endpoint

- `POST/GET /api/v1/slo`, `GET/PUT/DELETE /api/v1/slo/{id}`,
  `GET /api/v1/slo/{id}/history` (sederhana, nilai mock).
- `POST/GET /api/v1/monitor`, `GET/PUT/DELETE /api/v1/monitor/{id}`.
- List mendukung `?tags=a,b` (semua tag harus cocok).
- Auth: header `DD-API-KEY` harus sama dengan `DD_API_KEY` (salah → 403).
- Penyimpanan in-memory + persist ke file (`DD_MOCK_DB_FILE`, bila diset).
- ID dibuat server (SLO `slo_xxxxxxxx`, monitor integer dari 1001).

Field payload mengikuti dokumentasi resmi Datadog (lihat
`docs/ASSUMPTIONS.md` untuk daftar terverifikasi vs simplifikasi mock).

## Cara run

```sh
DD_API_KEY=lab-only-dd-api-key DATADOG_MOCK_PORT=8126 cargo run -p datadog-mock
```

Via compose: `docker compose --profile payslo up -d datadog-mock`.
Healthcheck image: `datadog-mock --healthcheck`.

## Contoh curl

```sh
KEY=lab-only-dd-api-key
curl -X POST 127.0.0.1:8126/api/v1/slo -H "DD-API-KEY: $KEY" \
  -H 'Content-Type: application/json' \
  -d '{"type":"metric","name":"demo","thresholds":[{"target":99.0,"timeframe":"7d"}],"tags":["managed-by:payslo"]}'
curl -H "DD-API-KEY: $KEY" '127.0.0.1:8126/api/v1/slo?tags=managed-by:payslo'
```
