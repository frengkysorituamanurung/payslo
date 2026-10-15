# Skenario 08 — Migrasi backend ke datadog-mock

> Briefing trainee. Cara mengerjakan TIDAK dibocorkan di sini.
> Butuh bantuan? `lab hint 08` (maks 3, penalti kecil).

## Situasi

Perusahaan mengevaluasi backend SLO kedua (Datadog) di samping Prometheus.
SLO lab yang sama harus hidup di kedua backend. Kandidat utama: datadog-mock
lokal (`:8126`).

## Tugas

1. Baca `payslo/CAPABILITIES.md`: apa yang TIDAK didukung backend Datadog
   dibanding Prometheus?
2. Apply ketiga SLO lab ke koneksi `datadog-mock` (cari tahu perintah
   `payslo` yang tepat; butuh `DD_API_KEY`).
3. Buktikan idempotensi (apply kedua = no-op) dan deteksi drift: ubah satu
   monitor manual via API, lalu tunjukkan output `payslo drift`.
4. Kembalikan (re-apply agar bersih).
5. Jawab (`lab answer <id> "<teks>"`):
   - `mapping`: window 1h dipetakan ke apa + warning apa yang muncul?
   - `drift`: bagaimana drift terdeteksi + contoh outputnya?
6. `lab verify 08` harus PASS, lalu `lab scenario stop 08`.

## Aturan main

- Dilarang menambah chaos baru atau deploy versi `bad` (skor safety 0).
