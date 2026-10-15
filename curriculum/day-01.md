# Hari 1 — Kenali Sistem

## Tujuan

Menjelaskan arsitektur PaySLO Lab dan menjalankan stack secara mandiri.

## Aktivitas

1. Baca [README](../README.md) + [AGENTS](../AGENTS.MD) (cukup bagian peran,
   arsitektur, P1–P8).
2. `make up-lite`, `make status`, `make open`.
3. Kirim 1 pembayaran manual via curl, ikuti statusnya sampai final.
4. Lihat saldo merchant di `http://127.0.0.1:5000/state`.

## Latihan

- `lab scenario list`, `lab scenario start 03 --force` lalu langsung
  `lab scenario stop 03` ( merasakan setup/teardown tanpa mengerjakan).
- `lab chaos status`, `lab chaos reset`.

## Pertanyaan refleksi

1. Mengapa HTTP 200 tidak berarti pembayaran sukses (P2)?
2. Komponen apa yang dilewati satu pembayaran dari loadgen ke merchant?

## Output dikumpulkan

- Screenshot `make status` (semua healthy) + 1 transaksi sukses (id + status).
