# Runbook: payment-latency-p99 (lab stub, trainee-owned)

## Gejala
Alert `PaySLOBurnRateHigh{slo="payment-latency-p99"}` firing.

## Diagnosis
1. Dashboard **Provider Health** → panel p50/p99: upstream mana yang lambat?
2. Profil bank `slow` (5–15 dtk) membuat gateway timeout (5 dtk) → 503.
3. Cek `envoy_cluster_upstream_rq_timeout` naik atau tidak.

## Mitigasi
- Kecilkan `LATENCY_MS_MAX` via `PUT /admin/profile` atau kembalikan profil
  ke `normal` via `POST /admin/reset`.
