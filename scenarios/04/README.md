# Skenario 04 — DB lambat: lock eksklusif di transactions

> Briefing trainee. Cara mendeteksi dan memitigasi TIDAK dibocorkan di sini.
> Butuh bantuan? `lab hint 04` (maks 3, penalti kecil).

## Situasi

Hampir semua request ke gateway timeout. Tidak ada deploy baru, bank-bank
terlihat normal. Sesuatu di lapisan data macet.

## Tugas

1. Identifikasi lapisan yang macet dan buktinya (lock apa? tabel apa? siapa
   pemegangnya?).
2. Bebaskan tanpa me-restart container database.
3. Jawab (`lab answer <id> "<teks>"`):
   - `diagnosis`: jenis lock + tabel + bukti.
   - `fix`: cara membebaskan + pencegahan.
4. `lab verify 04` harus PASS, lalu `lab scenario stop 04`.

## Aturan main

- Dilarang menambah chaos baru atau deploy versi `bad` (skor safety 0).
- Jangan me-restart postgres (data seed + state lab ada di situ).
