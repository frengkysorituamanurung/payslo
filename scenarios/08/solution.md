# Solusi Skenario 08 (MENTOR ONLY)

## Setup / teardown

`dd_reset`: hapus semua objek managed-by:payslo di mock (isolasi tes).

## Alur baku

```sh
export DD_API_KEY=lab-only-dd-api-key
payslo validate payslo-workspace/slo --workspace payslo-workspace
payslo apply payslo-workspace/slo --workspace payslo-workspace \
  --connection datadog-mock        # SLO dulu, baru monitor; SLO id dibuat server
payslo plan ... --connection datadog-mock   # 0 changes (idempoten)
# Edit manual satu monitor, lalu:
payslo drift --workspace payslo-workspace --connection datadog-mock
# -> drift: <slo> on datadog-mock (backend differs from last apply)
payslo apply ...                    # sembuh
```

## Poin pembelajaran

- Capability matrix: tanpa recording rules; timeframe diskrit; PromQL opaque.
- Warning window mapping muncul di setiap plan/apply (eksplisit).
- Urutan apply (SLO→monitor) penting karena query monitor mereferensikan id.

## Verifikasi

- ≥3 SLO dan ≥7 monitor di mock.
- Jawaban menyebut "7d" dan "drift".
