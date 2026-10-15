# Hints Skenario 08 (dibaca berurutan via `lab hint 08`)

## Hint 1

Koneksi sudah terdaftar di `payslo-workspace/connections.yml`
(`datadog-mock`). API key lab ada di `.env.example`.

## Hint 2

Alur: `payslo validate` → `payslo apply --connection datadog-mock` →
`payslo plan` (harus 0 changes) → edit satu monitor via
`PUT /api/v1/monitor/{id}` → `payslo drift` → re-apply untuk sembuh.
Perhatikan baris `warning` saat apply: window apa yang dipetakan?

## Hint 3

Datadog hanya mengenal timeframe 7d/30d/90d — window lab 1h dipetakan ke 7d
(dengan warning eksplisit, tidak diam-diam). Daftar lengkap perbedaan ada di
`payslo/CAPABILITIES.md` dan output `payslo capabilities`.
