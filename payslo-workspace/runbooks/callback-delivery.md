# Runbook: callback-delivery (lab stub, trainee-owned)

## Gejala
Alert `PaySLOBurnRateHigh{slo="callback-delivery"}` firing.

## Diagnosis
1. `payment_webhook_deliveries_total{status="FAILED"}` naik?
2. Dashboard **Incident Triage** → log gateway "webhook rejected/error".
3. `GET /admin/mode` di merchant-sink: mode 500 atau latency tinggi?

## Mitigasi
- Kembalikan mode merchant ke 200: `PUT /admin/mode {"status":200}`.
- Gateway retry 3x otomatis; yang gagal tercatat `FAILED` di `webhook_delivery`.
