# Hints Skenario 02 (dibaca berurutan via `lab hint 02`)

## Hint 1

Lihat gauge `payment_transactions_pending` (panel "Pending stuck").
Transaksi PENDING yang berumur > 5 menit menumpuk di satu provider.

## Hint 2

Cek profil KEDUA bank VA: `curl 127.0.0.1:4001/admin/profile` (bca) dan
`curl 127.0.0.1:4002/admin/profile` (mandiri). Perhatikan `profile` dan
`callback_delay_s`. Satu bank menunda callback, satu bank merespons lebih
lambat dari timeout gateway.

## Hint 3

Kembalikan kedua bank ke normal: `curl -X POST 127.0.0.1:4001/admin/reset`
dan sama untuk 4002. Tunggu callback yang tertunda mendarat (~5 menit),
lalu verifikasi gauge kembali ke nol.
