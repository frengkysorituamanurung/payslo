#![forbid(unsafe_code)]

//! Incident correlation: alerts with the same (slo, service) share one open
//! incident; resolved alerts close it when nothing is still firing.
//! Idempotent via alert fingerprints (UNIQUE). Concurrency-safe: one SQLite
//! transaction (`BEGIN IMMEDIATE`) per correlate call over a shared Mutex.

use payslo_model::{AlertStatus, CanonicalAlert};
use rusqlite::{Connection, params};
use thiserror::Error;

use crate::db::{self, DbError};

#[derive(Debug, Error)]
pub enum CorrelateError {
    #[error("database error: {0}")]
    Db(String),
}

impl From<DbError> for CorrelateError {
    fn from(e: DbError) -> Self {
        Self::Db(e.to_string())
    }
}

/// Outcome of one correlate call (for responder dispatch after commit).
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum CorrelateEvent {
    Opened { incident_id: String },
    Resolved { incident_id: String },
}

/// Correlate normalized alerts. Takes `&mut Connection` (exclusive via the
/// store mutex); one transaction per call keeps alert+incident+timeline+audit
/// atomic, and the mutex serializes concurrent webhook deliveries so racing
/// webhooks still yield one incident.
pub fn correlate(
    conn: &mut Connection,
    alerts: &[(CanonicalAlert, String)],
) -> Result<Vec<CorrelateEvent>, CorrelateError> {
    let tx = conn
        .transaction()
        .map_err(|e| CorrelateError::Db(e.to_string()))?;
    let mut events = Vec::new();
    // Pass 1: upsert semua alert lebih dulu. Satu webhook Alertmanager
    // berisi beberapa window burn rate (5m/1m, 15m/5m, 1h/10m) dalam satu
    // group. Kalau transisi diproses sambil insert, alert "resolved" bisa
    // menutup incident sebelum alert "firing" lain di batch yang sama
    // tercatat -> incident flapping (resolve lalu reopen).
    for (alert, fingerprint) in alerts {
        insert_alert(&tx, alert, fingerprint)?;
    }
    // Pass 2: transisi status (open/reopen/touch/resolve).
    for (alert, fingerprint) in alerts {
        match alert.status {
            AlertStatus::Firing => {
                if let Some(id) = open_incident(&tx, &alert.slo, &alert.service)? {
                    link_alert(&tx, &id, fingerprint)?;
                    touch_incident(&tx, &id)?;
                } else {
                    // No open incident: create one, or REOPEN the resolved one.
                    // The id is deterministic per (slo, service), so a plain
                    // INSERT would hit UNIQUE after a previous resolved incident.
                    let (id, reopened) = create_or_reopen_incident(&tx, alert)?;
                    link_alert(&tx, &id, fingerprint)?;
                    if reopened {
                        touch_incident(&tx, &id)?;
                        timeline(&tx, &id, "reopened", "payslo", "firing again after resolve")?;
                        audit(&tx, "payslo", "incident.reopen", &id)?;
                    } else {
                        timeline(
                            &tx,
                            &id,
                            "opened",
                            "payslo",
                            &format!("incident opened by {} alert", alert.severity),
                        )?;
                        audit(&tx, "payslo", "incident.open", &id)?;
                    }
                    events.push(CorrelateEvent::Opened { incident_id: id });
                }
            }
            AlertStatus::Resolved => {
                if let Some(id) = open_incident(&tx, &alert.slo, &alert.service)?
                    && !has_firing(&tx, &id)?
                {
                    resolve_incident(&tx, &id)?;
                    timeline(&tx, &id, "resolved", "payslo", "all alerts resolved")?;
                    audit(&tx, "payslo", "incident.resolve", &id)?;
                    events.push(CorrelateEvent::Resolved { incident_id: id });
                }
            }
        }
    }
    tx.commit().map_err(|e| CorrelateError::Db(e.to_string()))?;
    Ok(events)
}

fn insert_alert(
    tx: &rusqlite::Transaction<'_>,
    alert: &CanonicalAlert,
    fingerprint: &str,
) -> Result<(), CorrelateError> {
    let payload = serde_json::to_string(alert).unwrap_or_default();
    // Upsert by fingerprint: identical re-deliveries are no-ops, while a
    // firing->resolved transition updates the same row (no duplicates).
    tx.execute(
        "INSERT INTO alerts
             (incident_id, fingerprint, source, slo, service, severity, status, payload, received_at)
         VALUES (NULL, ?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)
         ON CONFLICT (fingerprint) DO UPDATE SET
           status = excluded.status,
           payload = excluded.payload,
           received_at = excluded.received_at",
        params![
            fingerprint,
            alert.labels.get("source").cloned().unwrap_or_default(),
            alert.slo,
            alert.service,
            alert.severity,
            match alert.status {
                AlertStatus::Firing => "firing",
                AlertStatus::Resolved => "resolved",
            },
            payload,
            db::now_rfc3339(),
        ],
    )
    .map_err(|e| CorrelateError::Db(e.to_string()))?;
    Ok(())
}

fn open_incident(
    tx: &rusqlite::Transaction<'_>,
    slo: &str,
    service: &str,
) -> Result<Option<String>, CorrelateError> {
    let mut stmt = tx
        .prepare("SELECT id FROM incidents WHERE slo = ?1 AND service = ?2 AND status <> 'resolved' ORDER BY created_at LIMIT 1")
        .map_err(|e| CorrelateError::Db(e.to_string()))?;
    let mut rows = stmt
        .query(params![slo, service])
        .map_err(|e| CorrelateError::Db(e.to_string()))?;
    match rows.next() {
        Ok(Some(row)) => Ok(Some(
            row.get(0).map_err(|e| CorrelateError::Db(e.to_string()))?,
        )),
        Ok(None) => Ok(None),
        Err(e) => Err(CorrelateError::Db(e.to_string())),
    }
}

fn create_or_reopen_incident(
    tx: &rusqlite::Transaction<'_>,
    alert: &CanonicalAlert,
) -> Result<(String, bool), CorrelateError> {
    let id = format!("inc-{}", fingerprint_id(&alert.slo, &alert.service));
    let title = format!("{} on {} ({})", alert.slo, alert.service, alert.severity);
    let now = db::now_rfc3339();
    // Reopen path: a resolved row with this deterministic id may exist.
    let changed = tx.execute(
        "INSERT INTO incidents (id, title, slo, service, status, severity, runbook, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, 'open', ?5, ?6, ?7, ?7)
         ON CONFLICT (id) DO UPDATE SET
           status = 'open', resolved_at = NULL, updated_at = excluded.updated_at
         WHERE incidents.status = 'resolved'",
        params![
            id,
            title,
            alert.slo,
            alert.service,
            alert.severity,
            alert.runbook_url.clone().unwrap_or_default(),
            now,
        ],
    )
    .map_err(|e| CorrelateError::Db(e.to_string()))?;
    // changed == 1: fresh insert OR reopen. changed == 0: conflicting open row
    // (lost a race with another writer); the open_incident check above said
    // none, so treat as usable and let link/touch proceed.
    let reopened = changed == 1
        && tx
            .query_row(
                "SELECT COUNT(*) FROM timeline WHERE incident_id = ?1 AND kind = 'opened'",
                params![id],
                |r| r.get::<_, i64>(0),
            )
            .map_err(|e| CorrelateError::Db(e.to_string()))?
            > 0;
    Ok((id, reopened))
}

/// Deterministic incident id per (slo, service): re-deliveries and both
/// sources converge on the same row even across restarts.
fn fingerprint_id(slo: &str, service: &str) -> String {
    use sha2::{Digest, Sha256};
    let mut hasher = Sha256::new();
    hasher.update(slo.as_bytes());
    hasher.update([0]);
    hasher.update(service.as_bytes());
    hex::encode(hasher.finalize())[..16].to_owned()
}

fn touch_incident(tx: &rusqlite::Transaction<'_>, id: &str) -> Result<(), CorrelateError> {
    tx.execute(
        "UPDATE incidents SET updated_at = ?1 WHERE id = ?2",
        params![db::now_rfc3339(), id],
    )
    .map_err(|e| CorrelateError::Db(e.to_string()))?;
    Ok(())
}

fn has_firing(tx: &rusqlite::Transaction<'_>, incident_id: &str) -> Result<bool, CorrelateError> {
    // Firing alerts are linked lazily: an alert belongs to the incident when
    // its (slo, service) matches; link rows on first sight.
    link_alerts(tx, incident_id)?;
    let count: i64 = tx
        .query_row(
            "SELECT COUNT(*) FROM alerts WHERE incident_id = ?1 AND status = 'firing'",
            params![incident_id],
            |row| row.get(0),
        )
        .map_err(|e| CorrelateError::Db(e.to_string()))?;
    Ok(count > 0)
}

fn link_alert(
    tx: &rusqlite::Transaction<'_>,
    incident_id: &str,
    fingerprint: &str,
) -> Result<(), CorrelateError> {
    tx.execute(
        "UPDATE alerts SET incident_id = ?1 WHERE fingerprint = ?2",
        params![incident_id, fingerprint],
    )
    .map_err(|e| CorrelateError::Db(e.to_string()))?;
    Ok(())
}

fn link_alerts(tx: &rusqlite::Transaction<'_>, incident_id: &str) -> Result<(), CorrelateError> {
    tx.execute(
        "UPDATE alerts SET incident_id = ?1 WHERE incident_id IS NULL
         AND (slo, service) = (SELECT slo, service FROM incidents WHERE id = ?1)",
        params![incident_id],
    )
    .map_err(|e| CorrelateError::Db(e.to_string()))?;
    Ok(())
}

fn resolve_incident(tx: &rusqlite::Transaction<'_>, id: &str) -> Result<(), CorrelateError> {
    let now = db::now_rfc3339();
    tx.execute(
        "UPDATE incidents SET status = 'resolved', resolved_at = ?1, updated_at = ?1 WHERE id = ?2",
        params![now, id],
    )
    .map_err(|e| CorrelateError::Db(e.to_string()))?;
    Ok(())
}

fn timeline(
    tx: &rusqlite::Transaction<'_>,
    incident_id: &str,
    kind: &str,
    actor: &str,
    message: &str,
) -> Result<(), CorrelateError> {
    tx.execute(
        "INSERT INTO timeline (incident_id, ts, kind, actor, message) VALUES (?1, ?2, ?3, ?4, ?5)",
        params![incident_id, db::now_rfc3339(), kind, actor, message],
    )
    .map_err(|e| CorrelateError::Db(e.to_string()))?;
    Ok(())
}

fn audit(
    tx: &rusqlite::Transaction<'_>,
    actor: &str,
    action: &str,
    details: &str,
) -> Result<(), CorrelateError> {
    tx.execute(
        "INSERT INTO audit_log (ts, actor, action, details) VALUES (?1, ?2, ?3, ?4)",
        params![db::now_rfc3339(), actor, action, details],
    )
    .map_err(|e| CorrelateError::Db(e.to_string()))?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::BTreeMap;

    fn memdb() -> Result<Connection, String> {
        let conn = Connection::open_in_memory().map_err(|e| e.to_string())?;
        conn.execute_batch(include_str!("../migrations/0001_init.sql"))
            .map_err(|e| e.to_string())?;
        Ok(conn)
    }

    fn alert(slo: &str, service: &str, status: AlertStatus) -> (CanonicalAlert, String) {
        let labels = BTreeMap::from([
            ("slo".to_owned(), slo.to_owned()),
            ("service".to_owned(), service.to_owned()),
        ]);
        let fp = crate::normalize::fingerprint("test", "n", &labels);
        (
            CanonicalAlert {
                slo: slo.to_owned(),
                service: service.to_owned(),
                severity: "page".to_owned(),
                status,
                starts_at: None,
                ends_at: None,
                labels,
                runbook_url: None,
            },
            fp,
        )
    }

    #[test]
    fn same_slo_service_shares_incident() -> Result<(), String> {
        let mut conn = memdb()?;
        let a = alert("s", "svc", AlertStatus::Firing);
        let b = alert("s", "svc", AlertStatus::Firing);
        // Different fingerprints (different labels would differ; force distinct).
        let b2 = (b.0.clone(), format!("{}-2", b.1));
        correlate(&mut conn, &[a]).map_err(|e| e.to_string())?;
        correlate(&mut conn, &[b2]).map_err(|e| e.to_string())?;
        let n: i64 = conn
            .query_row("SELECT COUNT(*) FROM incidents", [], |r| r.get(0))
            .map_err(|e| e.to_string())?;
        assert_eq!(n, 1);
        Ok(())
    }

    #[test]
    fn identical_resend_no_duplicates() -> Result<(), String> {
        let mut conn = memdb()?;
        let a = alert("s", "svc", AlertStatus::Firing);
        correlate(&mut conn, std::slice::from_ref(&a)).map_err(|e| e.to_string())?;
        correlate(&mut conn, std::slice::from_ref(&a)).map_err(|e| e.to_string())?;
        let incidents: i64 = conn
            .query_row("SELECT COUNT(*) FROM incidents", [], |r| r.get(0))
            .map_err(|e| e.to_string())?;
        let alerts: i64 = conn
            .query_row("SELECT COUNT(*) FROM alerts", [], |r| r.get(0))
            .map_err(|e| e.to_string())?;
        assert_eq!((incidents, alerts), (1, 1));
        Ok(())
    }

    #[test]
    fn resolved_closes_when_nothing_firing() -> Result<(), String> {
        let mut conn = memdb()?;
        let firing = alert("s", "svc", AlertStatus::Firing);
        correlate(&mut conn, std::slice::from_ref(&firing)).map_err(|e| e.to_string())?;
        // Same logical alert (same fingerprint) resolving closes the incident.
        let resolved = (
            CanonicalAlert {
                status: AlertStatus::Resolved,
                ..firing.0.clone()
            },
            firing.1.clone(),
        );
        let events =
            correlate(&mut conn, std::slice::from_ref(&resolved)).map_err(|e| e.to_string())?;
        assert_eq!(events.len(), 1);
        assert!(matches!(events[0], CorrelateEvent::Resolved { .. }));
        let status: String = conn
            .query_row("SELECT status FROM incidents", [], |r| r.get(0))
            .map_err(|e| e.to_string())?;
        assert_eq!(status, "resolved");
        Ok(())
    }

    #[test]
    fn batch_with_firing_alert_does_not_close_incident() -> Result<(), String> {
        // Regression: satu webhook group berisi beberapa window burn rate.
        // Alert resolved diproses sebelum alert firing lain di batch yang sama
        // -> incident tertutup prematur lalu reopen (flapping).
        let mut conn = memdb()?;
        let firing = alert("s", "svc", AlertStatus::Firing);
        correlate(&mut conn, std::slice::from_ref(&firing)).map_err(|e| e.to_string())?;
        // Batch: resolved lebih dulu, lalu masih firing (window lain).
        let resolved = (
            CanonicalAlert {
                status: AlertStatus::Resolved,
                ..firing.0.clone()
            },
            format!("{}-resolved", firing.1),
        );
        let still_firing = (firing.0.clone(), format!("{}-firing", firing.1));
        let events = correlate(&mut conn, &[resolved, still_firing]).map_err(|e| e.to_string())?;
        assert!(
            events.is_empty(),
            "incident tidak boleh ditutup saat masih ada alert firing di batch yang sama"
        );
        let status: String = conn
            .query_row("SELECT status FROM incidents", [], |r| r.get(0))
            .map_err(|e| e.to_string())?;
        assert_eq!(status, "open");
        Ok(())
    }

    #[test]
    fn unrelated_resolved_does_not_close() -> Result<(), String> {
        let mut conn = memdb()?;
        let firing = alert("s", "svc", AlertStatus::Firing);
        correlate(&mut conn, std::slice::from_ref(&firing)).map_err(|e| e.to_string())?;
        // A different alert resolving changes nothing while firing rows remain.
        let other = alert("other", "svc", AlertStatus::Resolved);
        let events =
            correlate(&mut conn, std::slice::from_ref(&other)).map_err(|e| e.to_string())?;
        assert!(events.is_empty());
        Ok(())
    }

    #[test]
    fn firing_after_resolve_reopens_same_incident() -> Result<(), String> {
        // Regression (FASE 8 smoke): deterministic id per (slo, service) hit
        // UNIQUE on re-fire after resolve; must reopen instead.
        let mut conn = memdb()?;
        let firing = alert("s", "svc", AlertStatus::Firing);
        let first =
            correlate(&mut conn, std::slice::from_ref(&firing)).map_err(|e| e.to_string())?;
        assert_eq!(first.len(), 1);
        let resolved = (
            CanonicalAlert {
                status: AlertStatus::Resolved,
                ..firing.0.clone()
            },
            firing.1.clone(),
        );
        correlate(&mut conn, std::slice::from_ref(&resolved)).map_err(|e| e.to_string())?;
        // Fire again (new fingerprint, e.g. next Alertmanager cycle).
        let again = (firing.0.clone(), format!("{}-next", firing.1));
        let events =
            correlate(&mut conn, std::slice::from_ref(&again)).map_err(|e| e.to_string())?;
        assert_eq!(events.len(), 1);
        assert!(matches!(events[0], CorrelateEvent::Opened { .. }));
        let (n, status): (i64, String) = conn
            .query_row("SELECT COUNT(*), MAX(status) FROM incidents", [], |r| {
                Ok((r.get(0)?, r.get(1)?))
            })
            .map_err(|e| e.to_string())?;
        assert_eq!((n, status.as_str()), (1, "open"));
        Ok(())
    }

    #[test]
    fn audit_append_only() -> Result<(), String> {
        let conn = memdb()?;
        conn.execute(
            "INSERT INTO audit_log (ts, actor, action) VALUES ('t', 'a', 'act')",
            [],
        )
        .map_err(|e| e.to_string())?;
        assert!(
            conn.execute("UPDATE audit_log SET action = 'x'", [])
                .is_err()
        );
        assert!(conn.execute("DELETE FROM audit_log", []).is_err());
        Ok(())
    }

    #[test]
    fn remediation_requires_approval_db() -> Result<(), String> {
        let conn = memdb()?;
        conn.execute(
            "INSERT INTO incidents (id, title, slo, service, status, severity, created_at, updated_at)
             VALUES ('inc-x', 't', 's', 'svc', 'open', 'page', 't', 't')",
            [],
        )
        .map_err(|e| e.to_string())?;
        conn.execute(
            "INSERT INTO runbook_executions (id, incident_id, runbook, status, created_at, updated_at)
             VALUES ('e1', 'inc-x', 'r', 'running', 't', 't')",
            [],
        )
        .map_err(|e| e.to_string())?;
        // remediation without approval is rejected by CHECK.
        assert!(
            conn.execute(
                "INSERT INTO runbook_steps (execution_id, idx, name, kind, requires_approval)
                 VALUES ('e1', 0, 's', 'remediation', 0)",
                [],
            )
            .is_err()
        );
        // ...and accepted with approval.
        assert!(
            conn.execute(
                "INSERT INTO runbook_steps (execution_id, idx, name, kind, requires_approval)
                 VALUES ('e1', 1, 's', 'remediation', 1)",
                [],
            )
            .is_ok()
        );
        Ok(())
    }
}
