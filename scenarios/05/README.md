# Skenario 05 — Webhook merchant gagal: sink me-return 500

> Briefing trainee. Cara mendeteksi dan memitigasi TIDAK dibocorkan di sini.
> Butuh bantuan? `lab hint 05` (maks 3, penalti kecil).

## Situasi

Pembayaran sukses seperti biasa, tetapi merchant tidak menerima notifikasi
webhook sama sekali. Tidak ada alert yang menyala — SLI pembayaran hijau.

## Tugas

1. Temukan di mana notifikasi hilang (gateway? jaringan? merchant?) beserta
   buktinya.
2. Pulihkan aliran webhook.
3. Jawab (`lab answer <id> "<teks>"`):
   - `diagnosis`: di mana hilangnya + bukti (kode status apa?).
   - `fix`: cara memulihkan.
4. `lab verify 05` harus PASS, lalu `lab scenario stop 05`.

## Aturan main

- Dilarang menambah chaos baru atau deploy versi `bad` (skor safety 0).
