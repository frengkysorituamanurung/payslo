# Solusi Skenario 02 (MENTOR ONLY)

## Setup

- mandiri → `late-callback` dengan `callback_delay_s=300` (+20 dtk bawaan
  profil = ~320 dtk): callback datang setelah gauge 5 menit → pending stuck.
- bca → `ambiguous`: bank sukses tetapi respons 7 dtk > timeout gateway 5 dtk
  → gateway 503, callback sukses datang belakangan (resolve via
  transaction_id — FASE 2 fix).

## Diagnosis

1. Gauge pending menumpuk di mandiri; 503 sporadis di bca yang tetap sukses
   (cek `GET /status/{ref}` ke bank).
2. Profil menjelaskan semuanya; tidak ada yang perlu di-disable (kedua bank
   pada dasarnya memproses dengan benar, hanya timing yang rusak).

## Mitigasi

Reset kedua bank ke normal (`POST /admin/reset` atau `lab chaos reset`).
Aman: mengembalikan timing normal; callback in-flight tetap mendarat dan
transaksi lama kedaluwarsa via expiry job.

## Verifikasi

- Success ratio > 0.9 (trafik segar).
- Jawaban mengandung "late-callback" dan "reset".
