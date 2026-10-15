# Solusi Skenario 04 (MENTOR ONLY)

## Setup

`db_lock` (300 dtk): sesi background memegang
`LOCK TABLE transactions IN ACCESS EXCLUSIVE MODE` + `pg_sleep`.
Semua tulis (dan baca) gateway ke tabel itu menggantung → Envoy 504 (10 dtk),
readiness gagal. Lock melepas sendiri setelah 300 dtk bila tak dibebaskan.

## Diagnosis

1. Envoy 5xx + latensi p99 mentok di timeout; bank sehat; gateway log
   menunjukkan query menggantung.
2. `pg_locks` × `pg_stat_activity`: AccessExclusiveLock di `transactions`,
   query `LOCK TABLE ... pg_sleep`.

## Mitigasi

```sh
lab chaos reset   # pg_terminate_backend pemegang lock lab
```

Tanpa restart postgres. Pencegahan: statement_timeout + lock_timeout di
aplikasi, waspada migrasi/DDL di jam sibuk.

## Verifikasi

- Rasio 5xx Envoy < 0.05 (trafik segar).
- Success ratio > 0.9.
- Jawaban mengandung "lock" dan "terminate".
