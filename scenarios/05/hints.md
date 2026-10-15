# Hints Skenario 05 (dibaca berurutan via `lab hint 05`)

## Hint 1

SLI pembayaran hijau tetapi merchant kosong: masalahnya di PENGIRIMAN,
bukan pembayaran. Lihat metrik `payment_webhook_deliveries_total`
berdasarkan status, dan log gateway saat mengirim webhook.

## Hint 2

Cek mode merchant-sink: dokumentasi endpoint admin-nya ada di README
merchant-sink. Status berapa yang dikembalikan untuk webhook saat ini?

## Hint 3

Kembalikan mode ke 200 (contoh ada di README merchant-sink), kirim satu
pembayaran uji, dan pastikan `DELIVERED` mengalir lagi. Jawabanmu harus
menyebut kode "500" (sebab) dan "200" (pulih).
