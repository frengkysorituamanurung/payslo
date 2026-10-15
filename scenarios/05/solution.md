# Solusi Skenario 05 (MENTOR ONLY)

## Setup

merchant-sink ke mode 500: semua webhook gateway (retry 3x) gagal,
tercatat `FAILED` di `webhook_delivery`. Transaksi tetap SUCCESS — tidak
ada alert (pelajaran: butuh SLI pengiriman terpisah).

## Diagnosis

1. `payment_webhook_deliveries_total{status="FAILED"}` naik; log gateway
   "webhook rejected, retrying".
2. `PUT /admin/mode` merchant dalam keadaan 500 (cek riwayat; tidak ada GET).

## Mitigasi

Kembalikan mode 200 (`lab chaos reset` atau PUT manual), kirim pembayaran
uji, pastikan `DELIVERED` mengalir.

## Verifikasi

- `increase(DELIVERED[3m]) > 0` (butuh trafik segar — solver mengirim sendiri).
- Jawaban mengandung "500" dan "200".
