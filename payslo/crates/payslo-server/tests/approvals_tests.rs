//! U4 acceptance: incident detail + notes + approvals lifecycle.
//! Decision safety: double decision -> 409; reject without reason -> 422.

use std::sync::Arc;

use payslo_server::app::{AppConfig, AppState};
use payslo_server::responder::NullResponder;

async fn serve() -> Result<(String, tokio::task::JoinHandle<()>), String> {
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
    let app = payslo_server::app::router(state);
    let handle = tokio::spawn(async move { if axum::serve(listener, app).await.is_err() {} });
    Ok((format!("http://{addr}"), handle))
}

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

/// XSS payload must round-trip verbatim (escaping is the client's job;
///
/// the server must neither execute nor mangle it).
fn xss_payload() -> serde_json::Value {
    serde_json::json!({
        "alerts": [{
            "status": "firing",
            "labels": {
                "alertname": "<script>alert(1)</script>",
                "slo": "payment-success-rate",
                "service": "gateway-api",
                "severity": "page",
                "note": "<img src=x onerror=alert(2)>",
            },
            "annotations": {},
        }],
    })
}

async fn fire(base: &str, body: &serde_json::Value) -> Result<serde_json::Value, String> {
    reqwest::Client::new()
        .post(format!("{base}/webhooks/alertmanager"))
        .json(body)
        .send()
        .await
        .map_err(|e| e.to_string())?
        .error_for_status()
        .map_err(|e| e.to_string())?
        .json()
        .await
        .map_err(|e| e.to_string())
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

async fn post(
    base: &str,
    path: &str,
    body: &serde_json::Value,
) -> Result<(u16, serde_json::Value), String> {
    let resp = reqwest::Client::new()
        .post(format!("{base}{path}"))
        .json(body)
        .send()
        .await
        .map_err(|e| e.to_string())?;
    let status = resp.status().as_u16();
    let resp_body: serde_json::Value = resp.json().await.map_err(|e| e.to_string())?;
    Ok((status, resp_body))
}

async fn incident_id(base: &str) -> Result<String, String> {
    fire(base, &am_payload()).await?;
    let (_, list) = get(base, "/incidents").await?;
    list.get("incidents")
        .and_then(|v| v.as_array())
        .and_then(|a| a.first())
        .and_then(|i| i.get("id"))
        .and_then(|v| v.as_str())
        .map(str::to_owned)
        .ok_or_else(|| "no incident opened".to_owned())
}

#[tokio::test]
async fn detail_notes_lifecycle() -> Result<(), String> {
    let (base, server) = serve().await?;
    let id = incident_id(&base).await?;
    // Unknown incident -> 404.
    let (status, _) = get(&base, "/incidents/inc-tidak-ada").await?;
    assert_eq!(status, 404, "unknown incident must 404");
    let (status, detail) = get(&base, &format!("/incidents/{id}")).await?;
    assert_eq!(status, 200);
    assert_eq!(detail["incident"]["id"], id);
    assert!(
        detail["timeline"]
            .as_array()
            .is_some_and(|t| t.iter().any(|e| e["kind"] == "opened")),
        "timeline must contain opened"
    );
    assert!(
        detail["alerts"].as_array().is_some_and(|a| !a.is_empty()),
        "correlated alerts must be present"
    );
    // Empty note rejected.
    let (status, _) = post(
        &base,
        &format!("/incidents/{id}/notes"),
        &serde_json::json!({"text": "  "}),
    )
    .await?;
    assert_eq!(status, 400, "empty note must 400");
    // Note round-trips into the timeline.
    let (status, _) = post(
        &base,
        &format!("/incidents/{id}/notes"),
        &serde_json::json!({"actor": "sinta", "text": "cek panel provider"}),
    )
    .await?;
    assert_eq!(status, 200);
    let (_, detail) = get(&base, &format!("/incidents/{id}")).await?;
    assert!(
        detail["timeline"]
            .as_array()
            .is_some_and(|t| t.iter().any(|e| e["kind"] == "note"
                && e["actor"] == "sinta"
                && e["message"] == "cek panel provider")),
        "note must appear in timeline"
    );
    server.abort();
    Ok(())
}

#[tokio::test]
async fn approval_decision_safety() -> Result<(), String> {
    let (base, server) = serve().await?;
    let id = incident_id(&base).await?;
    // Missing fields -> 400.
    let (status, _) = post(&base, "/approvals", &serde_json::json!({"action": "x"})).await?;
    assert_eq!(status, 400);
    // Create.
    let (status, created) = post(
        &base,
        "/approvals",
        &serde_json::json!({"incidentId": id, "action": "disable provider bca", "requestedBy": "sinta"}),
    )
    .await?;
    assert_eq!(status, 200);
    assert_eq!(created["deduped"], false);
    let apr = created["approval"]["id"]
        .as_str()
        .ok_or("no approval id")?
        .to_owned();
    assert_eq!(created["approval"]["status"], "pending");
    // Idempotent re-create -> same row, deduped.
    let (_, again) = post(
        &base,
        "/approvals",
        &serde_json::json!({"incidentId": id, "action": "disable provider bca"}),
    )
    .await?;
    assert_eq!(again["deduped"], true);
    assert_eq!(again["approval"]["id"], apr);
    // Unknown decision value -> 422.
    let (status, _) = post(
        &base,
        &format!("/approvals/{apr}/decision"),
        &serde_json::json!({"decision": "maybe", "by": "sinta"}),
    )
    .await?;
    assert_eq!(status, 422);
    // Reject without reason -> 422 (acceptance).
    let (status, _) = post(
        &base,
        &format!("/approvals/{apr}/decision"),
        &serde_json::json!({"decision": "rejected", "by": "sinta"}),
    )
    .await?;
    assert_eq!(status, 422, "reject without reason must 422");
    // Missing approver -> 400.
    let (status, _) = post(
        &base,
        &format!("/approvals/{apr}/decision"),
        &serde_json::json!({"decision": "approved"}),
    )
    .await?;
    assert_eq!(status, 400);
    // Approve.
    let (status, decided) = post(
        &base,
        &format!("/approvals/{apr}/decision"),
        &serde_json::json!({"decision": "approved", "by": "sinta"}),
    )
    .await?;
    assert_eq!(status, 200);
    assert_eq!(decided["approval"]["status"], "approved");
    assert_eq!(decided["approval"]["decidedBy"], "sinta");
    // Double decision -> 409 (acceptance: handled safely).
    let (status, body) = post(
        &base,
        &format!("/approvals/{apr}/decision"),
        &serde_json::json!({"decision": "rejected", "by": "sinta", "reason": "terlambat"}),
    )
    .await?;
    assert_eq!(status, 409, "second decision must 409, got {body}");
    // Timeline records the decision.
    let (_, detail) = get(&base, &format!("/incidents/{id}")).await?;
    assert!(
        detail["timeline"]
            .as_array()
            .is_some_and(|t| t.iter().any(|e| e["kind"] == "approval_approved")),
        "approval must appear in timeline"
    );
    server.abort();
    Ok(())
}

#[tokio::test]
async fn reject_with_reason_ok() -> Result<(), String> {
    let (base, server) = serve().await?;
    let id = incident_id(&base).await?;
    let (_, created) = post(
        &base,
        "/approvals",
        &serde_json::json!({"incidentId": id, "action": "restart gateway"}),
    )
    .await?;
    let apr = created["approval"]["id"]
        .as_str()
        .ok_or("no approval id")?
        .to_owned();
    let (status, decided) = post(
        &base,
        &format!("/approvals/{apr}/decision"),
        &serde_json::json!({"decision": "rejected", "by": "mentor", "reason": "risiko terlalu besar"}),
    )
    .await?;
    assert_eq!(status, 200);
    assert_eq!(decided["approval"]["status"], "rejected");
    assert_eq!(decided["approval"]["reason"], "risiko terlalu besar");
    server.abort();
    Ok(())
}

#[tokio::test]
async fn xss_payload_verbatim() -> Result<(), String> {
    let (base, server) = serve().await?;
    fire(&base, &xss_payload()).await?;
    let (_, list) = get(&base, "/incidents").await?;
    let id = list["incidents"][0]["id"]
        .as_str()
        .ok_or("no incident")?
        .to_owned();
    let (_, detail) = get(&base, &format!("/incidents/{id}")).await?;
    let alerts = detail["alerts"].as_array().ok_or("no alerts array")?;
    assert!(
        alerts
            .iter()
            .any(|a| a["payload"]["labels"]["alertname"] == "<script>alert(1)</script>"),
        "payload must round-trip verbatim (UI escapes on render)"
    );
    server.abort();
    Ok(())
}
