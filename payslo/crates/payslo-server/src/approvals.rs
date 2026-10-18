#![forbid(unsafe_code)]

//! U4: incident detail + manual notes + remediation approvals.
//!
//! The 5b schema (`runbook_executions`, `approval_requests`) already exists;
//! this module exposes it over HTTP. Lab serve (`lab serve`) is the only
//! writer besides webhooks: it creates one approval per remediation step,
//! forwards the trainee/mentor decision with approver identity, and executes
//! the remediation itself afterwards. There is deliberately NO "execute
//! remediation" endpoint: execution must always pass through an approved
//! decision recorded here.

use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use axum::{
    Json,
    extract::{Path, State},
    http::StatusCode,
};
use rusqlite::params;
use serde::Deserialize;
use serde_json::Value;

use crate::app::AppState;
use crate::db;

type ApiResult = Result<Json<Value>, (StatusCode, Json<Value>)>;

fn err(status: StatusCode, msg: &str) -> (StatusCode, Json<Value>) {
    (status, Json(serde_json::json!({"error": msg})))
}

/// Short unique id (`apr-…` / `exe-…`): nanos + pid is plenty for a lab.
fn new_id(prefix: &str) -> String {
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_nanos())
        .unwrap_or(0);
    format!("{prefix}-{nanos:x}-{}", std::process::id())
}

fn row_to_incident(row: &rusqlite::Row<'_>) -> rusqlite::Result<Value> {
    Ok(serde_json::json!({
        "id": row.get::<_, String>(0)?,
        "title": row.get::<_, String>(1)?,
        "slo": row.get::<_, String>(2)?,
        "service": row.get::<_, String>(3)?,
        "status": row.get::<_, String>(4)?,
        "severity": row.get::<_, String>(5)?,
        "runbook": row.get::<_, Option<String>>(6)?.unwrap_or_default(),
        "createdAt": row.get::<_, String>(7)?,
        "updatedAt": row.get::<_, String>(8)?,
        "resolvedAt": row.get::<_, Option<String>>(9)?,
    }))
}

/// GET /incidents/{id}: incident + timeline (oldest first) + correlated
/// alerts (linked, plus unlinked rows with the same slo/service — link rows
/// are written lazily, so matching by (slo, service) is the honest view).
pub async fn incident_detail(
    State(state): State<Arc<AppState>>,
    Path(id): Path<String>,
) -> ApiResult {
    let guard = state
        .db
        .lock()
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    let incident: Value = guard
        .query_row(
            "SELECT id, title, slo, service, status, severity, runbook, created_at, updated_at, resolved_at
             FROM incidents WHERE id = ?1",
            params![id],
            row_to_incident,
        )
        .map_err(|e| match e {
            rusqlite::Error::QueryReturnedNoRows => err(StatusCode::NOT_FOUND, "incident not found"),
            other => err(StatusCode::SERVICE_UNAVAILABLE, &other.to_string()),
        })?;
    let slo = incident
        .get("slo")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_owned();
    let service = incident
        .get("service")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_owned();
    let mut stmt = guard
        .prepare(
            "SELECT ts, kind, actor, message FROM timeline
             WHERE incident_id = ?1 ORDER BY id",
        )
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    let timeline = stmt
        .query_map(params![id], |row| {
            Ok(serde_json::json!({
                "ts": row.get::<_, String>(0)?,
                "kind": row.get::<_, String>(1)?,
                "actor": row.get::<_, String>(2)?,
                "message": row.get::<_, String>(3)?,
            }))
        })
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?
        .collect::<Result<Vec<_>, _>>()
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    let mut stmt = guard
        .prepare(
            "SELECT fingerprint, source, slo, service, severity, status, payload, received_at FROM alerts
             WHERE incident_id = ?1 OR (slo = ?2 AND service = ?3)
             ORDER BY received_at",
        )
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    let alerts = stmt
        .query_map(params![id, slo, service], |row| {
            let payload: String = row.get(6)?;
            let payload: Value = serde_json::from_str(&payload).unwrap_or(Value::Null);
            Ok(serde_json::json!({
                "fingerprint": row.get::<_, String>(0)?,
                "source": row.get::<_, String>(1)?,
                "slo": row.get::<_, String>(2)?,
                "service": row.get::<_, String>(3)?,
                "severity": row.get::<_, String>(4)?,
                "status": row.get::<_, String>(5)?,
                "payload": payload,
                "receivedAt": row.get::<_, String>(7)?,
            }))
        })
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?
        .collect::<Result<Vec<_>, _>>()
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    Ok(Json(serde_json::json!({
        "incident": incident,
        "timeline": timeline,
        "alerts": alerts,
    })))
}

#[derive(Debug, Deserialize)]
pub struct NoteBody {
    pub actor: Option<String>,
    pub text: Option<String>,
}

/// POST /incidents/{id}/notes: manual trainee note -> timeline (kind `note`).
pub async fn create_note(
    State(state): State<Arc<AppState>>,
    Path(id): Path<String>,
    Json(body): Json<NoteBody>,
) -> ApiResult {
    let actor = body.actor.unwrap_or_default().trim().to_owned();
    let actor = if actor.is_empty() {
        "trainee".to_owned()
    } else {
        actor
    };
    let text = body.text.unwrap_or_default().trim().to_owned();
    if text.is_empty() {
        return Err(err(StatusCode::BAD_REQUEST, "note text must not be empty"));
    }
    if text.len() > 2000 {
        return Err(err(
            StatusCode::BAD_REQUEST,
            "note text too long (max 2000 chars)",
        ));
    }
    let guard = state
        .db
        .lock()
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    let exists: i64 = guard
        .query_row(
            "SELECT COUNT(*) FROM incidents WHERE id = ?1",
            params![id],
            |r| r.get(0),
        )
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    if exists == 0 {
        return Err(err(StatusCode::NOT_FOUND, "incident not found"));
    }
    guard
        .execute(
            "INSERT INTO timeline (incident_id, ts, kind, actor, message) VALUES (?1, ?2, 'note', ?3, ?4)",
            params![id, db::now_rfc3339(), actor, text],
        )
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    guard
        .execute(
            "INSERT INTO audit_log (ts, actor, action, details) VALUES (?1, ?2, 'incident.note', ?3)",
            params![db::now_rfc3339(), actor, id],
        )
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    Ok(Json(serde_json::json!({"ok": true, "incidentId": id})))
}

fn row_to_approval(row: &rusqlite::Row<'_>) -> rusqlite::Result<Value> {
    Ok(serde_json::json!({
        "id": row.get::<_, String>(0)?,
        "executionId": row.get::<_, String>(1)?,
        "stepIdx": row.get::<_, i64>(2)?,
        "incidentId": row.get::<_, String>(3)?,
        "action": row.get::<_, String>(4)?,
        "status": row.get::<_, String>(5)?,
        "requestedAt": row.get::<_, String>(6)?,
        "decidedAt": row.get::<_, Option<String>>(7)?,
        "decidedBy": row.get::<_, Option<String>>(8)?,
        "reason": row.get::<_, Option<String>>(9)?,
    }))
}

/// GET /approvals[?incident=&status=]: approval queue (default all).
pub async fn list_approvals(
    State(state): State<Arc<AppState>>,
    axum::extract::Query(params): axum::extract::Query<std::collections::BTreeMap<String, String>>,
) -> Json<Value> {
    let items: Vec<Value> = state
        .db
        .lock()
        .map(|guard| {
            let mut sql =
                "SELECT a.id, a.execution_id, a.step_idx, a.incident_id, a.action, a.status,
                    a.requested_at, a.decided_at, a.decided_by, a.reason
                FROM approval_requests a WHERE 1 = 1"
                    .to_owned();
            let mut args: Vec<String> = Vec::new();
            if let Some(incident) = params.get("incident") {
                sql.push_str(" AND a.incident_id = ?");
                args.push(incident.clone());
            }
            if let Some(status) = params.get("status") {
                sql.push_str(" AND a.status = ?");
                args.push(status.clone());
            }
            sql.push_str(" ORDER BY a.requested_at DESC LIMIT 100");
            guard
                .prepare(&sql)
                .and_then(|mut stmt| {
                    stmt.query_map(rusqlite::params_from_iter(args.iter()), row_to_approval)?
                        .collect::<Result<Vec<_>, _>>()
                })
                .unwrap_or_default()
        })
        .unwrap_or_default();
    Json(serde_json::json!({"approvals": items}))
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CreateApprovalBody {
    pub incident_id: Option<String>,
    pub action: Option<String>,
    pub step_idx: Option<i64>,
    pub requested_by: Option<String>,
}

/// POST /approvals: open one approval request for a remediation action.
/// Idempotent on pending (incident_id, action): returns the existing row with
/// `"deduped": true` so double-clicks and retries are safe.
pub async fn create_approval(
    State(state): State<Arc<AppState>>,
    Json(body): Json<CreateApprovalBody>,
) -> ApiResult {
    let incident_id = body.incident_id.unwrap_or_default().trim().to_owned();
    let action = body.action.unwrap_or_default().trim().to_owned();
    let requested_by = body.requested_by.unwrap_or_default().trim().to_owned();
    let requested_by = if requested_by.is_empty() {
        "trainee".to_owned()
    } else {
        requested_by
    };
    if incident_id.is_empty() || action.is_empty() {
        return Err(err(
            StatusCode::BAD_REQUEST,
            "incident_id and action are required",
        ));
    }
    if action.len() > 200 {
        return Err(err(
            StatusCode::BAD_REQUEST,
            "action too long (max 200 chars)",
        ));
    }
    let guard = state
        .db
        .lock()
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    let incident: Value = guard
        .query_row(
            "SELECT id, title, slo, service, status, severity, runbook, created_at, updated_at, resolved_at
             FROM incidents WHERE id = ?1",
            params![incident_id],
            row_to_incident,
        )
        .map_err(|e| match e {
            rusqlite::Error::QueryReturnedNoRows => err(StatusCode::NOT_FOUND, "incident not found"),
            other => err(StatusCode::SERVICE_UNAVAILABLE, &other.to_string()),
        })?;
    if incident.get("status").and_then(|v| v.as_str()) == Some("resolved") {
        return Err(err(StatusCode::CONFLICT, "incident already resolved"));
    }
    // Idempotency: a pending request for the same (incident, action) is reused.
    let existing: Option<Value> = guard
        .query_row(
            "SELECT a.id, a.execution_id, a.step_idx, a.incident_id, a.action, a.status,
                    a.requested_at, a.decided_at, a.decided_by, a.reason
             FROM approval_requests a
             WHERE a.incident_id = ?1 AND a.action = ?2 AND a.status = 'pending'
             ORDER BY a.requested_at LIMIT 1",
            params![incident_id, action],
            row_to_approval,
        )
        .map(Some)
        .unwrap_or(None);
    if let Some(found) = existing {
        return Ok(Json(
            serde_json::json!({"approval": found, "deduped": true}),
        ));
    }
    let runbook = incident
        .get("runbook")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_owned();
    let execution_id = new_id("exe");
    let approval_id = new_id("apr");
    let now = db::now_rfc3339();
    let step_idx = body.step_idx.unwrap_or(0);
    guard
        .execute(
            "INSERT INTO runbook_executions (id, incident_id, runbook, status, created_at, updated_at)
             VALUES (?1, ?2, ?3, 'waiting_approval', ?4, ?4)",
            params![execution_id, incident_id, runbook, now],
        )
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    guard
        .execute(
            "INSERT INTO approval_requests
                (id, execution_id, step_idx, incident_id, action, status, requested_at)
             VALUES (?1, ?2, ?3, ?4, ?5, 'pending', ?6)",
            params![
                approval_id,
                execution_id,
                step_idx,
                incident_id,
                action,
                now
            ],
        )
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    guard
        .execute(
            "INSERT INTO timeline (incident_id, ts, kind, actor, message) VALUES (?1, ?2, 'approval_requested', ?3, ?4)",
            params![incident_id, now, requested_by, format!("approval requested: {action}")],
        )
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    guard
        .execute(
            "INSERT INTO audit_log (ts, actor, action, details) VALUES (?1, ?2, 'approval.request', ?3)",
            params![now, requested_by, approval_id],
        )
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    let created: Value = guard
        .query_row(
            "SELECT a.id, a.execution_id, a.step_idx, a.incident_id, a.action, a.status,
                    a.requested_at, a.decided_at, a.decided_by, a.reason
             FROM approval_requests a WHERE a.id = ?1",
            params![approval_id],
            row_to_approval,
        )
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    Ok(Json(
        serde_json::json!({"approval": created, "deduped": false}),
    ))
}

#[derive(Debug, Deserialize)]
pub struct DecisionBody {
    pub decision: Option<String>,
    pub by: Option<String>,
    pub reason: Option<String>,
}

/// POST /approvals/{id}/decision: record an approval decision.
/// - unknown id -> 404; already decided -> 409 (safe double decision);
/// - reject without reason -> 422; unknown decision value -> 422.
pub async fn decide_approval(
    State(state): State<Arc<AppState>>,
    Path(id): Path<String>,
    Json(body): Json<DecisionBody>,
) -> ApiResult {
    let decision = body.decision.unwrap_or_default().trim().to_lowercase();
    if decision != "approved" && decision != "rejected" {
        return Err(err(
            StatusCode::UNPROCESSABLE_ENTITY,
            "decision must be approved or rejected",
        ));
    }
    let by = body.by.unwrap_or_default().trim().to_owned();
    if by.is_empty() {
        return Err(err(
            StatusCode::BAD_REQUEST,
            "approver identity (by) is required",
        ));
    }
    let reason = body.reason.unwrap_or_default().trim().to_owned();
    if decision == "rejected" && reason.is_empty() {
        return Err(err(
            StatusCode::UNPROCESSABLE_ENTITY,
            "reason is required when rejecting",
        ));
    }
    let guard = state
        .db
        .lock()
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    let current: Option<(String, String, String)> = guard
        .query_row(
            "SELECT status, incident_id, action FROM approval_requests WHERE id = ?1",
            params![id],
            |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?)),
        )
        .map(Some)
        .unwrap_or(None);
    let Some((status, incident_id, action)) = current else {
        return Err(err(StatusCode::NOT_FOUND, "approval not found"));
    };
    if status != "pending" {
        return Err(err(
            StatusCode::CONFLICT,
            &format!("approval already decided ({status})"),
        ));
    }
    let now = db::now_rfc3339();
    guard
        .execute(
            "UPDATE approval_requests SET status = ?1, decided_at = ?2, decided_by = ?3, reason = ?4 WHERE id = ?5",
            params![decision, now, by, reason, id],
        )
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    let exec_status = if decision == "approved" {
        "running"
    } else {
        "failed"
    };
    guard
        .execute(
            "UPDATE runbook_executions SET status = ?1, updated_at = ?2
             WHERE id = (SELECT execution_id FROM approval_requests WHERE id = ?3)",
            params![exec_status, now, id],
        )
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    let timeline_msg = if reason.is_empty() {
        format!("approval {decision} by {by}: {action}")
    } else {
        format!("approval {decision} by {by}: {action} ({reason})")
    };
    guard
        .execute(
            "INSERT INTO timeline (incident_id, ts, kind, actor, message) VALUES (?1, ?2, ?3, ?4, ?5)",
            params![
                incident_id,
                now,
                format!("approval_{decision}"),
                by,
                timeline_msg
            ],
        )
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    guard
        .execute(
            "INSERT INTO audit_log (ts, actor, action, details) VALUES (?1, ?2, 'approval.decide', ?3)",
            params![now, by, format!("{id} {decision}")],
        )
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    let updated: Value = guard
        .query_row(
            "SELECT a.id, a.execution_id, a.step_idx, a.incident_id, a.action, a.status,
                    a.requested_at, a.decided_at, a.decided_by, a.reason
             FROM approval_requests a WHERE a.id = ?1",
            params![id],
            row_to_approval,
        )
        .map_err(|e| err(StatusCode::SERVICE_UNAVAILABLE, &e.to_string()))?;
    Ok(Json(serde_json::json!({"approval": updated})))
}
