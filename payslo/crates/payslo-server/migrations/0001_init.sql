-- PaySLO server schema (SQLite, rusqlite bundled, WAL).
-- Covers FASE 5a (incidents/alerts/timeline/audit) and 5b (runbooks/approvals).
-- audit_log is append-only: triggers abort UPDATE and DELETE.
-- Remediation steps MUST require approval: CHECK on runbook_steps plus
-- load-time enforcement in payslo-runbook (see docs).

CREATE TABLE IF NOT EXISTS incidents (
    id          TEXT PRIMARY KEY,
    title       TEXT NOT NULL,
    slo         TEXT NOT NULL,
    service     TEXT NOT NULL,
    status      TEXT NOT NULL CHECK (status IN ('open', 'acknowledged', 'resolved')),
    severity    TEXT NOT NULL,
    runbook     TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    resolved_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_incidents_slo_service
    ON incidents (slo, service, status);

CREATE TABLE IF NOT EXISTS alerts (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    incident_id  TEXT REFERENCES incidents (id),
    fingerprint  TEXT NOT NULL UNIQUE,
    source       TEXT NOT NULL,
    slo          TEXT NOT NULL,
    service      TEXT NOT NULL,
    severity     TEXT NOT NULL,
    status       TEXT NOT NULL CHECK (status IN ('firing', 'resolved')),
    payload      TEXT NOT NULL,
    received_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_alerts_incident ON alerts (incident_id);
CREATE INDEX IF NOT EXISTS idx_alerts_slo_service ON alerts (slo, service, status);

CREATE TABLE IF NOT EXISTS timeline (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    incident_id TEXT NOT NULL REFERENCES incidents (id),
    ts          TEXT NOT NULL,
    kind        TEXT NOT NULL,
    actor       TEXT NOT NULL,
    message     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_timeline_incident ON timeline (incident_id);

CREATE TABLE IF NOT EXISTS audit_log (
    id     INTEGER PRIMARY KEY AUTOINCREMENT,
    ts     TEXT NOT NULL,
    actor  TEXT NOT NULL,
    action TEXT NOT NULL,
    details TEXT NOT NULL DEFAULT '{}'
);

CREATE TRIGGER IF NOT EXISTS audit_log_no_update
BEFORE UPDATE ON audit_log
BEGIN
    SELECT RAISE(ABORT, 'audit_log is append-only: UPDATE forbidden');
END;

CREATE TRIGGER IF NOT EXISTS audit_log_no_delete
BEFORE DELETE ON audit_log
BEGIN
    SELECT RAISE(ABORT, 'audit_log is append-only: DELETE forbidden');
END;

-- 5b tables (runbook execution + approvals). Defined now so the schema is
-- created once; enforced here AND at load time in payslo-runbook.
CREATE TABLE IF NOT EXISTS runbook_executions (
    id          TEXT PRIMARY KEY,
    incident_id TEXT NOT NULL REFERENCES incidents (id),
    runbook     TEXT NOT NULL,
    status      TEXT NOT NULL CHECK (status IN ('running', 'waiting_approval', 'done', 'failed')),
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runbook_steps (
    execution_id      TEXT NOT NULL REFERENCES runbook_executions (id),
    idx               INTEGER NOT NULL,
    name              TEXT NOT NULL,
    kind              TEXT NOT NULL CHECK (kind IN ('diagnostic', 'remediation', 'manual')),
    requires_approval INTEGER NOT NULL DEFAULT 0,
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'running', 'ok', 'failed', 'skipped', 'waiting_approval', 'approved', 'rejected')),
    output            TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (execution_id, idx),
    CONSTRAINT remediation_needs_approval
        CHECK (kind <> 'remediation' OR requires_approval = 1)
);

CREATE TABLE IF NOT EXISTS step_executions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    execution_id TEXT NOT NULL REFERENCES runbook_executions (id),
    step_idx     INTEGER NOT NULL,
    started_at   TEXT NOT NULL,
    finished_at  TEXT,
    status       TEXT NOT NULL,
    output       TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS approval_requests (
    id           TEXT PRIMARY KEY,
    execution_id TEXT NOT NULL REFERENCES runbook_executions (id),
    step_idx     INTEGER NOT NULL,
    incident_id  TEXT NOT NULL REFERENCES incidents (id),
    action       TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'approved', 'rejected', 'expired')),
    requested_at TEXT NOT NULL,
    decided_at   TEXT,
    decided_by   TEXT
);
