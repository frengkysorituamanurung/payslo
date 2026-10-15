# Hints Skenario 06 (dibaca berurutan via `lab hint 06`)

## Hint 1

Buka dashboard Payment Overview: HTTP 5xx naik TAJAM pada satu titik waktu
sementara bank-bank sehat. Cek anotasi (ikon garis vertikal) — ada deploy
apa tepat sebelum titik itu?

## Hint 2

Versi gateway bisa dicek dan diganti saat runtime: baca README gateway-api
bagian "Mode release buruk", atau `lab deploy --help`.

## Hint 3

Rollback: `lab deploy gateway --version good`. Tunggu ~2 menit agar jendela
[2m] bersih, lalu verifikasi 5xx < 0.05.
