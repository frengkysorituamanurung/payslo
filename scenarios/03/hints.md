# Hints Skenario 03 (dibaca berurutan via `lab hint 03`)

## Hint 1

Buka `http://127.0.0.1:5000/state` dan cari order dengan `credits: 2`.
Saldo = 2x nominal. Webhook yang sama diterima dua kali.

## Hint 2

Cek `callback_log` di database (user `lab_ro`, read-only!) atau profil bank
ewallet (`curl 127.0.0.1:4003/admin/profile`). Perhatikan `duplicate_callbacks`.

## Hint 3

Hentikan sumbernya: `curl -X POST 127.0.0.1:4003/admin/reset`.
Perbaikan permanen (di luar lab): merchant harus idempoten (dedup by
transaction_id); gateway sengaja tidak dedup (LAB-FLAW, lihat mentor-guide).
