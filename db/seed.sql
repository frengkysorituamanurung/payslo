-- PaySLO Lab — synthetic deterministic seed (LAB-ONLY, no real data).
-- Idempotent: safe to re-run (ON CONFLICT DO NOTHING).
-- Fixed UUIDs + fixed timestamps so trainee dashboards look alive on fresh clone.

INSERT INTO transactions
    (id, merchant_id, order_id, channel, provider, amount, fee, status, provider_ref, created_at, updated_at, expires_at)
VALUES
    ('11111111-1111-1111-1111-111111111111', 'merchant-demo', 'seed-001', 'VA', 'bca', 150000, 4000, 'SUCCESS', 'P-bca-0001',
     '2026-10-01T09:00:00Z', '2026-10-01T09:00:05Z', '2026-10-01T09:10:00Z'),
    ('22222222-2222-2222-2222-222222222222', 'merchant-demo', 'seed-002', 'VA', 'mandiri', 250000, 4000, 'SUCCESS', 'P-mandiri-0001',
     '2026-10-01T09:01:00Z', '2026-10-01T09:01:06Z', '2026-10-01T09:11:00Z'),
    ('33333333-3333-3333-3333-333333333333', 'merchant-demo', 'seed-003', 'EWALLET', 'ewallet', 75000, 2000, 'FAILED', 'P-ewallet-0001',
     '2026-10-01T09:02:00Z', '2026-10-01T09:02:04Z', '2026-10-01T09:12:00Z'),
    ('44444444-4444-4444-4444-444444444444', 'merchant-demo', 'seed-004', 'QRIS', 'ewallet', 50000, 1500, 'SUCCESS', 'P-ewallet-0002',
     '2026-10-01T09:03:00Z', '2026-10-01T09:03:03Z', '2026-10-01T09:13:00Z'),
    ('55555555-5555-5555-5555-555555555555', 'merchant-demo', 'seed-005', 'VA', 'bca', 1000000, 4000, 'EXPIRED', NULL,
     '2026-10-01T08:00:00Z', '2026-10-01T08:10:01Z', '2026-10-01T08:10:00Z')
ON CONFLICT (merchant_id, order_id) DO NOTHING;

INSERT INTO callback_log (transaction_id, provider, payload, received_at)
VALUES
    ('11111111-1111-1111-1111-111111111111', 'bca', '{"provider_ref": "P-bca-0001", "status": "SUCCESS"}', '2026-10-01T09:00:05Z'),
    ('22222222-2222-2222-2222-222222222222', 'mandiri', '{"provider_ref": "P-mandiri-0001", "status": "SUCCESS"}', '2026-10-01T09:01:06Z'),
    ('33333333-3333-3333-3333-333333333333', 'ewallet', '{"provider_ref": "P-ewallet-0001", "status": "FAILED"}', '2026-10-01T09:02:04Z'),
    ('44444444-4444-4444-4444-444444444444', 'ewallet', '{"provider_ref": "P-ewallet-0002", "status": "SUCCESS"}', '2026-10-01T09:03:03Z')
ON CONFLICT DO NOTHING;

INSERT INTO webhook_delivery (transaction_id, attempt, http_status, status, created_at)
VALUES
    ('11111111-1111-1111-1111-111111111111', 1, 200, 'DELIVERED', '2026-10-01T09:00:06Z'),
    ('22222222-2222-2222-2222-222222222222', 1, 200, 'DELIVERED', '2026-10-01T09:01:07Z'),
    ('33333333-3333-3333-3333-333333333333', 1, 200, 'DELIVERED', '2026-10-01T09:02:05Z'),
    ('44444444-4444-4444-4444-444444444444', 1, 200, 'DELIVERED', '2026-10-01T09:03:04Z')
ON CONFLICT DO NOTHING;
