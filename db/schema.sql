-- PaySLO Lab — application schema (Postgres).
-- LAB-ONLY: synthetic data, no real credentials.
-- This file is mounted to /docker-entrypoint-initdb.d on fresh volumes AND
-- kept consistent with db/migrations/0001_init.sql (see schema-consistency test
-- in apps/gateway-api/tests/). DDL uses IF NOT EXISTS so both paths are safe.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ---------------------------------------------------------------- transactions
CREATE TABLE IF NOT EXISTS transactions (
    id            UUID        PRIMARY KEY,
    merchant_id   TEXT        NOT NULL,
    order_id      TEXT        NOT NULL,
    channel       TEXT        NOT NULL CHECK (channel IN ('VA', 'EWALLET', 'QRIS')),
    provider      TEXT        NOT NULL CHECK (provider IN ('bca', 'mandiri', 'ewallet')),
    amount        BIGINT      NOT NULL CHECK (amount > 0),
    fee           BIGINT      NOT NULL CHECK (fee >= 0),
    status        TEXT        NOT NULL CHECK (status IN ('CREATED', 'PENDING', 'SUCCESS', 'FAILED', 'EXPIRED')),
    provider_ref  TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ NOT NULL,
    UNIQUE (merchant_id, order_id)
);

CREATE INDEX IF NOT EXISTS idx_transactions_status_created
    ON transactions (status, created_at);
CREATE INDEX IF NOT EXISTS idx_transactions_provider_status
    ON transactions (provider, status);

-- ---------------------------------------------------------------- callback_log
CREATE TABLE IF NOT EXISTS callback_log (
    id             BIGSERIAL   PRIMARY KEY,
    transaction_id UUID        NOT NULL REFERENCES transactions (id),
    provider       TEXT        NOT NULL,
    payload        JSONB       NOT NULL,
    received_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_callback_log_transaction
    ON callback_log (transaction_id);

-- ------------------------------------------------------------ webhook_delivery
CREATE TABLE IF NOT EXISTS webhook_delivery (
    id             BIGSERIAL   PRIMARY KEY,
    transaction_id UUID        NOT NULL REFERENCES transactions (id),
    attempt        INTEGER     NOT NULL CHECK (attempt >= 1),
    http_status    INTEGER,
    status         TEXT        NOT NULL CHECK (status IN ('DELIVERED', 'FAILED')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_webhook_delivery_transaction
    ON webhook_delivery (transaction_id);

-- ------------------------------------------------- read-only role for lab (P4)
-- LAB-ONLY credential, synthetic lab. Never use real secrets here.
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'lab_ro') THEN
        CREATE ROLE lab_ro WITH LOGIN PASSWORD 'lab-only-ro-change-me';
    END IF;
END
$$;

GRANT CONNECT ON DATABASE payslo_lab TO lab_ro;
GRANT USAGE ON SCHEMA public TO lab_ro;
GRANT SELECT ON transactions, callback_log, webhook_delivery TO lab_ro;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO lab_ro;
