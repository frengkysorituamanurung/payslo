# Skenario 02 — Stuck pending: callback terlambat + ambiguous

> Briefing trainee. Cara mendeteksi dan memitigasi TIDAK dibocorkan di sini.
> Butuh bantuan? `lab hint 02` (maks 3, penalti kecil).

## Situasi

Transaksi PENDING menumpuk jauh lebih lama dari biasanya. Sebagian respons
gateway 503, tetapi anehnya pembayaran-pembayaran itu tercatat sukses di bank.

## Tugas

1. Temukan bank mana yang berperilaku aneh dan jelaskan mekanismenya
   (profil apa? delay berapa?).
2. Pulihkan agar transaksi kembali final dengan cepat.
3. Jawab (`lab answer <id> "<teks>"`):
   - `diagnosis`: bank + mekanisme (kata kunci: profil dan delay).
   - `fix`: mitigasi yang dilakukan dan mengapa aman.
4. `lab verify 02` harus PASS, lalu `lab scenario stop 02`.

## Aturan main

- Dilarang menambah chaos baru atau deploy versi `bad` (skor safety 0).
