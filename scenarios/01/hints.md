# Hints Skenario 01 (dibaca berurutan via `lab hint 01`)

## Hint 1

Lihat panel "Success rate per provider" di dashboard Payment Overview.
Satu provider menukik ke nol sementara yang lain sehat.

## Hint 2

Cek profil bank yang mencurigakan: `curl 127.0.0.1:4001/admin/profile`
(ganti port 4001/4002/4003 untuk bca/mandiri/ewallet).
Perhatikan field `profile`.

## Hint 3

Matikan provider yang down sementara: baca contoh di README gateway-api
bagian "Routing", lalu `lab routing disable <provider>`.
Tunggu ~1 menit dan lihat HTTP 2xx pulih di dashboard.
