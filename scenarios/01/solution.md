# Solusi Skenario 01 (MENTOR ONLY)

## Setup

`scenario start 01` mengeset bank-bca ke profil `down` (charge menggantung;
gateway timeout 5 dtk → 503) dan TTL transaksi 60 dtk agar EXPIRED cepat
terbentuk dan burn-rate alert menyala dalam ~3-4 menit.

## Diagnosis

1. Payment Overview → "Success rate per provider": `bca` → 0, lainnya ~1.
2. Provider Health → upstream timeouts naik; log gateway `bank charge timed out`.
3. `curl 127.0.0.1:4001/admin/profile` → `{"profile":"down"}`.
4. Divergensi P2: gateway me-return 503 cepat untuk timeout (bagian dari 503
   masuk rasio 5xx, TAPI setelah mitigasi 2xx pulih sementara payment-success
   bca tetap 0 — HTTP mengukur respons, bukan outcome bisnis).

## Mitigasi

```sh
lab routing disable bca   # VA 100% ke mandiri
```

## Verifikasi

- `payment-success-rate` incident ada (payslo).
- `sli_above` success ratio > 0.9 (trafik segar pasca-mitigasi).
- Jawaban mengandung "bca" dan "200".

## Teardown

`scenario stop 01`: bank reset, routing bca/mandiri enabled, TTL 600.
