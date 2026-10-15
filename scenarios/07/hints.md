# Hints Skenario 07 (dibaca berurutan via `lab hint 07`)

## Hint 1

Lihat `ALERTS{alertname="PaySLOBurnRateHigh"}` di Prometheus: label `window`
membedakan fast-burn (5m) vs slow-burn (1h). Mana yang flap?

## Hint 2

Fast-burn yang flap + slow-burn yang tenang = error kecil intermiten, bukan
outage. Cari sumber error kecilnya: profil bank apa yang acak? Cek
`GET /admin/profile` ketiga bank.

## Hint 3

Reset bank yang flaky ke normal, lalu jawab: fast window untuk deteksi cepat
(rawan noise), slow window untuk kepastian. Usulan tuning misal: naikkan
threshold page 14.4 → 20, atau wajibkan slow-burn ikut menyala sebelum paging.
