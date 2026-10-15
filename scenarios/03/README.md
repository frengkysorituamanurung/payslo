# Skenario 03 — Callback duplikat: saldo merchant ganda

> Briefing trainee. Cara mendeteksi dan memitigasi TIDAK dibocorkan di sini.
> Butuh bantuan? `lab hint 03` (maks 3, penalti kecil).

## Situasi

Tim finance melaporkan saldo merchant untuk sejumlah order bernilai dua kali
lipat. Di sisi gateway, semua pembayaran tercatat sukses normal — tidak ada
alert yang menyala.

## Tugas

1. Buktikan dari mana penggandaan berasal (merchant `/state` menunjukkan apa?
   bank apa yang terlibat?).
2. Hentikan duplikasi yang sedang berjalan.
3. Jawab (`lab answer <id> "<teks>"`):
   - `diagnosis`: mekanisme duplikat (kata kunci: duplikat, callback).
   - `fix`: mitigasi + perbaikan permanen yang diusulkan.
4. `lab verify 03` harus PASS, lalu `lab scenario stop 03`.

## Aturan main

- Dilarang menambah chaos baru atau deploy versi `bad` (skor safety 0).
