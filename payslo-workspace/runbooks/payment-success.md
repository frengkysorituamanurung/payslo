# Runbook: payment-success-rate (lab stub, trainee-owned)

## Gejala
Alert `PaySLOBurnRateHigh{slo="payment-success-rate"}` firing.

## Diagnosis
1. Dashboard **Payment Overview** → panel "Success rate per provider": provider
   mana yang anjlok?
2. Dashboard **Provider Health** → timeouts upstream naik? (`bank down`?)
3. `curl 127.0.0.1:4001/admin/profile` (ganti port per provider) → profil aktif.
4. Log gateway: filter `status="FAILED"` di dashboard **Incident Triage**.

## Mitigasi
- Bank down → `PUT /admin/routing` nonaktifkan provider itu (manual, tanpa
  circuit breaker otomatis — P3).
- Verifikasi: success rate pulih, HTTP 2xx kembali ~1.0.
