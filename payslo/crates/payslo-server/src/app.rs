#![forbid(unsafe_code)]

//! HTTP app: webhook ingestion (Alertmanager + Datadog) with optional shared
//! token auth (`X-Payslo-Token`). Correlate runs in `spawn_blocking`; responder
//! dispatch happens after the transaction commits.

use std::collections::BTreeMap;
use std::sync::{Arc, Mutex};

use axum::{
    Json, Router,
    extract::State,
    http::HeaderMap,
    routing::{get, post},
};
use rusqlite::Connection;
use serde_json::Value;
use tracing::{info, warn};

use crate::incidents::{CorrelateEvent, correlate};
use crate::normalize;
use crate::{
    approvals,
    responder::{Responder, ResponderEvent},
};

#[derive(Debug, Clone)]
pub struct AppConfig {
    /// Required value of `X-Payslo-Token` when set; open when None (lab).
    pub webhook_token: Option<String>,
    pub grafana_public_url: String,
}

pub struct AppState {
    pub db: Mutex<Connection>,
    pub responder: Arc<dyn Responder>,
    pub config: AppConfig,
}

impl AppState {
    #[must_use]
    pub fn new(db: Connection, responder: Arc<dyn Responder>, config: AppConfig) -> Arc<Self> {
        Arc::new(Self {
            db: Mutex::new(db),
            responder,
            config,
        })
    }
}

fn check_token(
    state: &Arc<AppState>,
    headers: &HeaderMap,
) -> Result<(), (axum::http::StatusCode, Json<Value>)> {
    if let Some(want) = state.config.webhook_token.as_deref() {
        let got = headers
            .get("x-payslo-token")
            .and_then(|v| v.to_str().ok())
            .unwrap_or("");
        if got != want {
            return Err((
                axum::http::StatusCode::UNAUTHORIZED,
                Json(serde_json::json!({"error": "bad webhook token"})),
            ));
        }
    }
    Ok(())
}

async fn ingest(
    state: Arc<AppState>,
    source: &str,
    body: Value,
) -> Result<Json<Value>, (axum::http::StatusCode, Json<Value>)> {
    let err = |msg: String| {
        (
            axum::http::StatusCode::BAD_REQUEST,
            Json(serde_json::json!({"error": msg})),
        )
    };
    let alerts = match source {
        "alertmanager" => normalize::from_alertmanager(&body).map_err(|e| err(e.to_string()))?,
        "datadog" => normalize::from_datadog(&body).map_err(|e| err(e.to_string()))?,
        _ => return Err(err("unknown source".to_owned())),
    };
    if alerts.is_empty() {
        return Ok(Json(serde_json::json!({"ok": true, "events": 0})));
    }
    let count = alerts.len();
    let state_cb = state.clone();
    let events = tokio::task::spawn_blocking(move || {
        let mut guard = state_cb.db.lock().map_err(|e| format!("db lock: {e}"))?;
        correlate(&mut guard, &alerts).map_err(|e| e.to_string())
    })
    .await
    .map_err(|e| err(format!("db task: {e}")))?
    .map_err(err)?;
    // Dispatch notifications after commit.
    for event in &events {
        let notification = match event {
            CorrelateEvent::Opened { incident_id } => {
                let (title, slo, service, severity, runbook) =
                    incident_summary(&state, incident_id);
                Some(ResponderEvent::Opened {
                    incident_id: incident_id.clone(),
                    title,
                    slo,
                    service,
                    severity,
                    runbook,
                })
            }
            CorrelateEvent::Resolved { incident_id } => {
                let (title, slo, service, _, _) = incident_summary(&state, incident_id);
                Some(ResponderEvent::Resolved {
                    incident_id: incident_id.clone(),
                    title,
                    slo,
                    service,
                })
            }
        };
        if let Some(notification) = notification
            && let Err(e) = state.responder.notify(&notification).await
        {
            warn!(error = %e, "responder notify failed");
        }
    }
    info!(source, count, events = events.len(), "webhook ingested");
    Ok(Json(
        serde_json::json!({"ok": true, "events": events.len()}),
    ))
}

fn incident_summary(
    state: &Arc<AppState>,
    incident_id: &str,
) -> (String, String, String, String, String) {
    let guard = state.db.lock();
    let Ok(guard) = guard else {
        return (
            "?".to_owned(),
            "?".to_owned(),
            "?".to_owned(),
            "ticket".to_owned(),
            String::new(),
        );
    };
    guard
        .query_row(
            "SELECT title, slo, service, severity, COALESCE(runbook, '') FROM incidents WHERE id = ?1",
            [incident_id],
            |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                    row.get::<_, String>(3)?,
                    row.get::<_, String>(4)?,
                ))
            },
        )
        .unwrap_or_else(|_| ("?".to_owned(), "?".to_owned(), "?".to_owned(), "ticket".to_owned(), String::new()))
}

async fn alertmanager_webhook(
    State(state): State<Arc<AppState>>,
    headers: HeaderMap,
    Json(body): Json<Value>,
) -> Result<Json<Value>, (axum::http::StatusCode, Json<Value>)> {
    check_token(&state, &headers)?;
    ingest(state, "alertmanager", body).await
}

async fn datadog_webhook(
    State(state): State<Arc<AppState>>,
    headers: HeaderMap,
    Json(body): Json<Value>,
) -> Result<Json<Value>, (axum::http::StatusCode, Json<Value>)> {
    check_token(&state, &headers)?;
    ingest(state, "datadog", body).await
}

async fn healthz() -> Json<Value> {
    Json(serde_json::json!({"ok": true}))
}

/// GET /audit[?limit=]: audit log PaySLO (append-only; sumber timeline mentor
/// di lab serve, digabung dengan audit lab serve berurutan waktu).
async fn audit_list(
    State(state): State<Arc<AppState>>,
    axum::extract::Query(params): axum::extract::Query<BTreeMap<String, String>>,
) -> Json<Value> {
    let limit: i64 = params
        .get("limit")
        .and_then(|v| v.parse().ok())
        .unwrap_or(200)
        .clamp(1, 1000);
    let entries: Vec<Value> = state
        .db
        .lock()
        .map(|guard| {
            guard
                .prepare("SELECT ts, actor, action, details FROM audit_log ORDER BY id LIMIT ?1")
                .and_then(|mut stmt| {
                    let rows = stmt.query_map(
                        [limit],
                        |row| {
                            Ok(serde_json::json!({
                                "ts": row.get::<_, String>(0)?,
                                "actor": row.get::<_, String>(1)?,
                                "action": row.get::<_, String>(2)?,
                                "details": row.get::<_, String>(3)?,
                            }))
                        },
                    )?;
                    rows.collect::<Result<Vec<_>, _>>()
                })
                .unwrap_or_default()
        })
        .unwrap_or_default();
    Json(serde_json::json!({"entries": entries}))
}

/// GET /incidents[?slo=&service=]: machine-readable incident list for the
/// lab CLI (avoids SQLite file sharing between container and host).
async fn list_incidents(
    State(state): State<Arc<AppState>>,
    axum::extract::Query(params): axum::extract::Query<BTreeMap<String, String>>,
) -> Json<Value> {
    let items: Vec<Value> = state
        .db
        .lock()
        .map(|guard| {
            let mut sql =
                "SELECT id, title, slo, service, status, severity, created_at, resolved_at FROM incidents WHERE 1 = 1"
                    .to_owned();
            // Positional params in fixed order; only present filters append.
            let mut args: Vec<String> = Vec::new();
            if let Some(slo) = params.get("slo") {
                sql.push_str(" AND slo = ?");
                args.push(slo.clone());
            }
            if let Some(service) = params.get("service") {
                sql.push_str(" AND service = ?");
                args.push(service.clone());
            }
            sql.push_str(" ORDER BY created_at DESC LIMIT 100");
            guard
                .prepare(&sql)
                .and_then(|mut stmt| {
                    let rows = stmt.query_map(
                        rusqlite::params_from_iter(args.iter()),
                        |row| {
                            Ok(serde_json::json!({
                                "id": row.get::<_, String>(0)?,
                                "title": row.get::<_, String>(1)?,
                                "slo": row.get::<_, String>(2)?,
                                "service": row.get::<_, String>(3)?,
                                "status": row.get::<_, String>(4)?,
                                "severity": row.get::<_, String>(5)?,
                                "createdAt": row.get::<_, String>(6)?,
                                "resolvedAt": row.get::<_, Option<String>>(7)?,
                            }))
                        },
                    )?;
                    rows.collect::<Result<Vec<_>, _>>()
                })
                .unwrap_or_default()
        })
        .unwrap_or_default();
    Json(serde_json::json!({"incidents": items}))
}

/// Build the route table (5a webhooks + U4 incident/approval reads & writes).
pub fn router(state: Arc<AppState>) -> Router {
    Router::new()
        .route("/healthz", get(healthz))
        .route("/audit", get(audit_list))
        .route("/incidents", get(list_incidents))
        .route("/incidents/{id}", get(approvals::incident_detail))
        .route("/incidents/{id}/notes", post(approvals::create_note))
        .route(
            "/approvals",
            get(approvals::list_approvals).post(approvals::create_approval),
        )
        .route("/approvals/{id}/decision", post(approvals::decide_approval))
        .route("/webhooks/alertmanager", post(alertmanager_webhook))
        .route("/webhooks/datadog", post(datadog_webhook))
        .with_state(state)
}
