# Mentor Guide — PaySLO Lab

Panduan fasilitator onboarding. Kunci jawaban: `scenarios/*/solution.md`.

## Mode mentor (U6) — jujur tentang gate

Mode mentor dibuka dengan PIN via env `MENTOR_PIN` (default lab: `1234` di
`.env.example`). **INI ADALAH PEMBATAS SPOILER, BUKAN KEAMANAN.** Yang
ditangani:

- Menu `/mentor` tidak ditampilkan untuk mode trainee (UI tidak me-render
  item navigasi; endpoint API mentor mengembalikan 403 tanpa token).
- Trainee tetap bisa `curl` admin bank-sim/gateway langsung (honor system) —
  gate ini tidak menutup itu, dan tidak mengklaim menutup.
- 5x salah PIN -> jeda 5 menit (429 + `Retry-After`). Token dicabut saat
  keluar mode mentor.

Jangan sebut PIN di depan trainee. Ganti PIN lewat `.env` bila perlu.

## Daftar cacat sengaja (P3 — JANGAN dibocorkan di depan)

1. **Callback duplikat tidak di-dedup** gateway (`// LAB-FLAW:` di
   `apps/gateway-api/src/routes.rs`) → skenario 03 (saldo ganda).
2. **Tanpa circuit breaker otomatis** → mitigasi selalu manual via
   `PUT /admin/routing` (skenario 01).
3. **Merchant-sink mengkredit tiap webhook** (LAB-FLAW ganda terlihat).
4. **Release `bad`** (20% 500 + 300ms) untuk skenario 06.
5. **Ambiguous bank** (respons > timeout, callback sukses belakangan).

## Waktu ideal per skenario

| Skenario | Setup+deteksi | Mitigasi+verify | Total |
|---|---|---|---|
| 01 | ~7 mnt | ~3 mnt | ~10 mnt |
| 02 | ~7 mnt | ~3 mnt | ~10 mnt |
| 03 | ~2 mnt | ~2 mnt | ~5 mnt |
| 04 | ~2 mnt | ~4 mnt | ~6 mnt |
| 05 | ~1 mnt | ~2 mnt | ~4 mnt |
| 06–09 | ~5 mnt | ~3 mnt | ~8 mnt |

## Pertanyaan pancingan

- "HTTP 2xx 100% — berarti aman?" (P2: belum tentu, lihat payment success.)
- "Alert page menyala 1 menit lalu padam — darurat?" (07: bedakan window.)
- "DB lambat — restart postgres?" (04: jangan; terminate lock saja.)
- "SLO hijau tapi merchant komplain?" (03/05: SLI yang salah diukur.)

## Kesalahan umum trainee

1. Me-restart container untuk masalah konfigurasi (routing/profil bisa runtime).
2. Menambah chaos untuk "mengetes" di tengah skenario (safety 0).
3. Menjawab tanpa bukti metrik (diagnosis butuh angka).
4. Mengabaikan `warning` saat `payslo apply` (08: mapping timeframe).

## Rubrik penilaian

Otomatis /80: TTD 15, TTM 25, diagnosis 25, safety 15 (lihat `lab verify`).
Manual /20: komunikasi 10 (kabar insiden jelas? update berkala?), postmortem
10 (lihat template di bawah). Lulus: PASS otomatis + manual ≥ 12.

## Template postmortem

```text
# Postmortem: <judul> (<tanggal>)
## Ringkasan (2 kalimat)
## Dampak (berapa transaksi/pelanggan, berapa lama)
## Timeline (waktu + kejadian + aksi)
## Akar masalah (5 whys singkat)
## Mitigasi (apa yang menghentikan pendarahan)
## Tindak lanjut (pemilik + tenggat)
```
