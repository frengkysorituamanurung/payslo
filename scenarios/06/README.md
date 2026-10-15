# Skenario 06 — Regresi deploy: gateway versi bad

> Briefing trainee. Cara mendeteksi dan memitigasi TIDAK dibocorkan di sini.
> Butuh bantuan? `lab hint 06` (maks 3, penalti kecil).

## Situasi

Sejak deploy gateway terbaru, sebagian pembayaran gagal cepat dan latensi
p99 melonjak. Bank-bank dan database terlihat normal.

## Tugas

1. Korelasikan gangguan dengan anotasi deploy di Grafana (kapan deploy
   terjadi vs kapan metrik rusak?).
2. Rollback ke versi baik TANPA me-restart container.
3. Jawab (`lab answer <id> "<teks>"`):
   - `diagnosis`: penyebab + bukti (sebut kata "deploy").
   - `fix`: cara rollback + verifikasi (sebut kata "good").
4. `lab verify 06` harus PASS, lalu `lab scenario stop 06`.

## Aturan main

- Dilarang menambah chaos baru atau deploy versi `bad` LAGI setelah mulai
  (skor safety 0 — rollback ke `good` justru wajib).
