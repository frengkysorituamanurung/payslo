# Solusi Skenario 06 (MENTOR ONLY)

## Setup

Gateway ke versi `bad`: 20% respons 500 cepat + 300ms latency tambahan pada
semua request. Tidak ada baris transaksi untuk yang 500 (gagal sebelum
insert) sehingga burn-ratio SLI tidak menyala — terdeteksi via HTTP 5xx.

## Diagnosis

1. Anotasi deploy di Grafana berimpit dengan awal spike 5xx.
2. `PUT /admin/release` dalam keadaan `bad` (atau tanya siapa deploy terakhir).

## Mitigasi

```sh
lab deploy gateway --version good   # PUT /admin/release, tanpa restart
```

## Verifikasi

- Rasio 5xx Envoy [2m] < 0.05.
- Jawaban mengandung "deploy" dan "good".
