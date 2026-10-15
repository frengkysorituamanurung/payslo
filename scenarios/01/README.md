# Skenario 01 — VA timeout: bank-bca down

> Briefing trainee. HANYA briefing — cara mendeteksi dan memitigasi TIDAK
> dibocorkan di sini. Butuh bantuan? `lab hint 01` (maks 3, penalti kecil).

## Situasi

Sejak beberapa menit lalu, pelanggan kanal VA mengeluh pembayaran macet.
Load generator terus mengirim trafik normal (VA 60%, EWALLET 30%, QRIS 10%).

## Tugas

1. Buka Grafana (Payment Overview + Provider Health) dan tentukan provider
   mana yang bermasalah beserta buktinya.
2. Mitigasi agar transaksi VA kembali lancar TANPA me-restart container
   (petunjuk: routing provider bisa diubah saat runtime).
3. Jawab dua pertanyaan (`lab answer <id> "<teks>"`):
   - `diagnosis`: provider apa + bukti (metrik/log apa)?
   - `divergence`: mengapa rasio HTTP 2xx tetap tinggi sementara payment
     success anjlok?
4. Verifikasi: `lab verify 01` harus PASS, lalu `lab scenario stop 01`.

## Aturan main

- Dilarang menambah chaos baru (`lab chaos ...`) atau deploy versi `bad`
  selama skenario (skor safety 0).
- Semua aksi lewat `lab` tercatat otomatis (TTD/TTM).
