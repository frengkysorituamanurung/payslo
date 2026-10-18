//! FASE 5a acceptance: two sources -> one incident; resend -> no dup;
//! concurrent webhooks stay consistent.

use std::sync::Arc;

use payslo_server::app::{AppConfig, AppState};
use payslo_server::responder::NullResponder;

fn am_payload() -> serde_json::Value {
    serde_json::json!({
        "alerts": [{
            "status": "firing",
            "labels": {
                "alertname": "PaySLOBurnRateHigh",
                "slo": "payment-success-rate",
                "service": "gateway-api",
                "severity": "page",
            },
            "annotations": {"runbook_url": "runbooks/payment-success.md"},
        }],
    })
}

fn dd_payload() -> serde_json::Value {
    serde_json::json!({
        "title": "PaySLOBurnRateHigh",
        "text": "burn",
        "alert_type": "error",
        "priority": "P1",
        "tags": ["slo:payment-success-rate", "service:gateway-api", "severity:page"],
    })
}

async fn serve() -> Result<(String, Arc<AppState>, tokio::task::JoinHandle<()>), String> {
    let conn =
        payslo_server::db::open(std::path::Path::new(":memory:")).map_err(|e| e.to_string())?;
    let state = AppState::new(
        conn,
        Arc::new(NullResponder),
        AppConfig {
            webhook_token: None,
            grafana_public_url: "http://127.0.0.1:3002".to_owned(),
        },
    );
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
        .await
        .map_err(|e| e.to_string())?;
    let addr = listener.local_addr().map_err(|e| e.to_string())?;
    let app = payslo_server::app::router(state.clone());
    let handle = tokio::spawn(async move { if axum::serve(listener, app).await.is_err() {} });
    Ok((format!("http://{addr}"), state, handle))
}

fn counts(state: &Arc<AppState>) -> Result<(i64, i64, i64), String> {
    let guard = state.db.lock().map_err(|e| e.to_string())?;
    let incidents: i64 = guard
        .query_row("SELECT COUNT(*) FROM incidents", [], |r| r.get(0))
        .map_err(|e| e.to_string())?;
    let alerts: i64 = guard
        .query_row("SELECT COUNT(*) FROM alerts", [], |r| r.get(0))
        .map_err(|e| e.to_string())?;
    let audit: i64 = guard
        .query_row("SELECT COUNT(*) FROM audit_log", [], |r| r.get(0))
        .map_err(|e| e.to_string())?;
    Ok((incidents, alerts, audit))
}

async fn post(base: &str, path: &str, body: &serde_json::Value) -> Result<(), String> {
    reqwest::Client::new()
        .post(format!("{base}{path}"))
        .json(body)
        .send()
        .await
        .map_err(|e| e.to_string())?
        .error_for_status()
        .map_err(|e| e.to_string())?;
    Ok(())
}

#[tokio::test]
async fn two_sources_one_incident() -> Result<(), String> {
    let (base, state, server) = serve().await?;
    post(&base, "/webhooks/alertmanager", &am_payload()).await?;
    post(&base, "/webhooks/datadog", &dd_payload()).await?;
    // Identical resend of the alertmanager payload: no duplicates.
    post(&base, "/webhooks/alertmanager", &am_payload()).await?;
    let (incidents, alerts, audit) = counts(&state)?;
    assert_eq!(incidents, 1, "two sources must share one incident");
    assert_eq!(alerts, 2, "am + dd fingerprints (resend collapsed)");
    assert!(audit >= 1, "audit_log must record the open");
    server.abort();
    Ok(())
}

#[tokio::test]
async fn concurrent_webhooks_one_incident() -> Result<(), String> {
    let (base, state, server) = serve().await?;
    let mut handles = Vec::new();
    for _ in 0..8 {
        let base = base.clone();
        let body = am_payload();
        handles.push(tokio::spawn(async move {
            post(&base, "/webhooks/alertmanager", &body).await
        }));
    }
    for handle in handles {
        handle
            .await
            .map_err(|e| e.to_string())?
            .map_err(|e| e.to_string())?;
    }
    let (incidents, alerts, _) = counts(&state)?;
    assert_eq!(incidents, 1, "concurrent webhooks must converge");
    assert_eq!(alerts, 1, "identical fingerprints collapse");
    server.abort();
    Ok(())
}

/// U6: GET /audit (sumber timeline mentor). Audit tercatat saat insiden
/// dibuka; endpoint mengembalikan entri {ts, actor, action, details}.
#[tokio::test]
async fn audit_endpoint_lists_entries() -> Result<(), String> {
    let (base, state, server) = serve().await?;
    post(&base, "/webhooks/alertmanager", &am_payload()).await?;
    let (_, _, audit_count) = counts(&state)?;
    assert!(audit_count >= 1, "incident.open must be audited");
    let (status, body) = get(&base, "/audit").await?;
    assert_eq!(status, 200, "GET /audit must be public like /incidents");
    let entries = body["entries"].as_array().cloned().unwrap_or_default();
    assert!(
        !entries.is_empty(),
        "audit entries must list after incident open"
    );
    let first = &entries[0];
    for key in ["ts", "actor", "action", "details"] {
        assert!(first.get(key).is_some(), "audit entry missing {key}");
    }
    assert_eq!(first["action"], "incident.open");
    // limit=1 -> tepat 1 entri.
    let (_, body1) = get(&base, "/audit?limit=1").await?;
    assert_eq!(body1["entries"].as_array().map(Vec::len), Some(1));
    server.abort();
    Ok(())
}

async fn get(base: &str, path: &str) -> Result<(u16, serde_json::Value), String> {
    let resp = reqwest::Client::new()
        .get(format!("{base}{path}"))
        .send()
        .await
        .map_err(|e| e.to_string())?;
    let status = resp.status().as_u16();
    let body = resp.json().await.map_err(|e| e.to_string())?;
    Ok((status, body))
}
