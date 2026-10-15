#![forbid(unsafe_code)]

//! merchant-sink: records every accepted webhook and credits the order ledger.
//! LAB-FLAW (visible by design): duplicates are NOT deduplicated — each accepted
//! webhook credits the ledger again, so duplicate callbacks show as double
//! balance (scenario 03).

use std::collections::HashMap;
use std::sync::Arc;

use axum::{
    Json,
    body::Bytes,
    extract::State,
    http::{HeaderMap, StatusCode},
};
use serde::{Deserialize, Serialize};
use tokio::sync::RwLock;
use tracing::info;

use crate::verify;

#[derive(Debug, Clone)]
pub struct SinkConfig {
    pub secret: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct LedgerEntry {
    /// Sum of ALL credited webhooks for this order (duplicates inflate it).
    pub balance: i64,
    /// Number of accepted webhooks for this order.
    pub credits: u64,
}

#[derive(Debug, Clone, Serialize)]
pub struct WebhookEvent {
    pub seq: u64,
    pub transaction_id: String,
    pub order_id: String,
    pub merchant_id: String,
    pub amount: i64,
    pub status: String,
    pub received_at: String,
}

pub struct SinkState {
    pub config: SinkConfig,
    pub inner: RwLock<StateInner>,
}

#[derive(Debug, Default)]
pub struct StateInner {
    pub mode_status: u16,
    pub mode_latency_ms: u64,
    pub ledger: HashMap<String, LedgerEntry>,
    pub events: Vec<WebhookEvent>,
    pub rejected: u64,
}

impl SinkState {
    pub fn new(secret: String) -> Arc<Self> {
        Arc::new(Self {
            config: SinkConfig { secret },
            inner: RwLock::new(StateInner {
                mode_status: 200,
                ..Default::default()
            }),
        })
    }
}

#[derive(Debug, Deserialize)]
pub struct HookPayload {
    pub transaction_id: String,
    pub order_id: String,
    pub merchant_id: String,
    pub amount: i64,
    pub status: String,
}

/// POST /webhook: verify HMAC, apply failure/latency mode, credit ledger.
pub async fn webhook(
    State(state): State<Arc<SinkState>>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Json<serde_json::Value>, (StatusCode, Json<serde_json::Value>)> {
    let bad = |msg: &str| {
        (
            StatusCode::UNAUTHORIZED,
            Json(serde_json::json!({ "error": msg })),
        )
    };
    let signature = headers
        .get("x-signature")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("");
    let timestamp = headers
        .get("x-timestamp")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("");
    if !verify::verify(&state.config.secret, timestamp, &body, signature) {
        state.inner.write().await.rejected += 1;
        return Err(bad("invalid signature"));
    }
    let payload: HookPayload = serde_json::from_slice(&body).map_err(|_| {
        (
            StatusCode::BAD_REQUEST,
            Json(serde_json::json!({ "error": "invalid json" })),
        )
    })?;

    let (mode_status, mode_latency) = {
        let s = state.inner.read().await;
        (s.mode_status, s.mode_latency_ms)
    };
    if mode_latency > 0 {
        tokio::time::sleep(std::time::Duration::from_millis(mode_latency)).await;
    }
    if mode_status != 200 {
        let code = StatusCode::from_u16(mode_status).unwrap_or(StatusCode::INTERNAL_SERVER_ERROR);
        return Err((
            code,
            Json(serde_json::json!({ "error": "merchant sink failure mode (lab)" })),
        ));
    }

    let mut s = state.inner.write().await;
    let credits = {
        let entry = s
            .ledger
            .entry(payload.order_id.clone())
            .or_insert(LedgerEntry {
                balance: 0,
                credits: 0,
            });
        // LAB-FLAW: credit EVERY accepted webhook; duplicates double the balance.
        entry.balance += payload.amount;
        entry.credits += 1;
        entry.credits
    };
    let seq = s.events.len() as u64 + 1;
    s.events.push(WebhookEvent {
        seq,
        transaction_id: payload.transaction_id.clone(),
        order_id: payload.order_id.clone(),
        merchant_id: payload.merchant_id.clone(),
        amount: payload.amount,
        status: payload.status.clone(),
        received_at: time::OffsetDateTime::now_utc()
            .format(&time::format_description::well_known::Rfc3339)
            .unwrap_or_else(|_| String::new()),
    });
    info!(
        order_id = %payload.order_id, amount = payload.amount,
        credits, "webhook credited"
    );
    Ok(Json(serde_json::json!({ "received": true, "seq": seq })))
}

/// GET /state: ledger balances + event list (for trainees and e2e checks).
pub async fn get_state(State(state): State<Arc<SinkState>>) -> Json<serde_json::Value> {
    let s = state.inner.read().await;
    Json(serde_json::json!({
        "ledger": s.ledger,
        "events": s.events,
        "rejected": s.rejected,
    }))
}

/// GET /admin/mode: current failure-mode (for chaos status + snapshots).
pub async fn get_mode(State(state): State<Arc<SinkState>>) -> Json<serde_json::Value> {
    let s = state.inner.read().await;
    Json(serde_json::json!({ "status": s.mode_status, "latency_ms": s.mode_latency_ms }))
}

#[derive(Debug, Deserialize)]
pub struct ModeBody {
    pub status: u16,
    pub latency_ms: Option<u64>,
}

/// PUT /admin/mode: inject 500s / latency (trainee failure drills).
pub async fn put_mode(
    State(state): State<Arc<SinkState>>,
    Json(body): Json<ModeBody>,
) -> Result<Json<serde_json::Value>, (StatusCode, Json<serde_json::Value>)> {
    if body.status != 200 && body.status != 500 {
        return Err((
            StatusCode::BAD_REQUEST,
            Json(serde_json::json!({ "error": "status must be 200|500" })),
        ));
    }
    let mut s = state.inner.write().await;
    s.mode_status = body.status;
    s.mode_latency_ms = body.latency_ms.unwrap_or(0);
    Ok(Json(
        serde_json::json!({ "status": s.mode_status, "latency_ms": s.mode_latency_ms }),
    ))
}

pub async fn healthz() -> Json<serde_json::Value> {
    Json(serde_json::json!({ "ok": true }))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::verify;
    use axum::routing::{get, post, put};

    fn test_router() -> (axum::Router, Arc<SinkState>) {
        let state = SinkState::new("test-secret".to_owned());
        let app = axum::Router::new()
            .route("/healthz", get(healthz))
            .route("/webhook", post(webhook))
            .route("/state", get(get_state))
            .route("/admin/mode", put(put_mode))
            .with_state(state.clone());
        (app, state)
    }

    async fn serve() -> anyhow::Result<(String, reqwest::Client, tokio::task::JoinHandle<()>)> {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await?;
        let addr = listener.local_addr()?;
        let (app, _) = test_router();
        let handle = tokio::spawn(async move {
            if axum::serve(listener, app).await.is_err() {
                // Listener closed; test over.
            }
        });
        Ok((format!("http://{addr}"), reqwest::Client::new(), handle))
    }

    fn signed(secret: &str, payload: &serde_json::Value) -> (String, String, Vec<u8>) {
        let body = serde_json::to_vec(payload).unwrap_or_default();
        let ts = "1700000000".to_owned();
        let sig = verify::compute_signature(secret, &ts, &body);
        (ts, sig, body)
    }

    #[tokio::test]
    async fn webhook_credits_ledger_once() -> anyhow::Result<()> {
        let (base, http, server) = serve().await?;
        let payload = serde_json::json!({
            "transaction_id": "t1", "order_id": "o1",
            "merchant_id": "m", "amount": 50000, "status": "SUCCESS"
        });
        let (ts, sig, body) = signed("test-secret", &payload);
        let resp: serde_json::Value = http
            .post(format!("{base}/webhook"))
            .header("X-Signature", sig)
            .header("X-Timestamp", ts)
            .body(body)
            .send()
            .await?
            .json()
            .await?;
        assert_eq!(resp["received"], true);
        let state: serde_json::Value = http
            .get(format!("{base}/state"))
            .send()
            .await?
            .json()
            .await?;
        assert_eq!(state["ledger"]["o1"]["balance"], 50000);
        assert_eq!(state["ledger"]["o1"]["credits"], 1);
        server.abort();
        Ok(())
    }

    #[tokio::test]
    async fn duplicate_webhooks_double_credit_lab_flaw() -> anyhow::Result<()> {
        let (base, http, server) = serve().await?;
        let payload = serde_json::json!({
            "transaction_id": "t2", "order_id": "o2",
            "merchant_id": "m", "amount": 25000, "status": "SUCCESS"
        });
        for _ in 0..2 {
            let (ts, sig, body) = signed("test-secret", &payload);
            let resp = http
                .post(format!("{base}/webhook"))
                .header("X-Signature", sig)
                .header("X-Timestamp", ts)
                .body(body)
                .send()
                .await?;
            assert_eq!(resp.status(), 200);
        }
        let state: serde_json::Value = http
            .get(format!("{base}/state"))
            .send()
            .await?
            .json()
            .await?;
        // Duplicates are NOT deduplicated by design (scenario 03).
        assert_eq!(state["ledger"]["o2"]["balance"], 50000);
        assert_eq!(state["ledger"]["o2"]["credits"], 2);
        server.abort();
        Ok(())
    }

    #[tokio::test]
    async fn bad_signature_rejected() -> anyhow::Result<()> {
        let (base, http, server) = serve().await?;
        let payload = serde_json::json!({
            "transaction_id": "t3", "order_id": "o3",
            "merchant_id": "m", "amount": 1000, "status": "SUCCESS"
        });
        let body = serde_json::to_vec(&payload).unwrap_or_default();
        let resp = http
            .post(format!("{base}/webhook"))
            .header("X-Signature", "00".repeat(32))
            .header("X-Timestamp", "1700000000")
            .body(body)
            .send()
            .await?;
        assert_eq!(resp.status(), 401);
        server.abort();
        Ok(())
    }

    #[tokio::test]
    async fn failure_mode_returns_500() -> anyhow::Result<()> {
        let (base, http, server) = serve().await?;
        let mode: serde_json::Value = http
            .put(format!("{base}/admin/mode"))
            .json(&serde_json::json!({"status": 500}))
            .send()
            .await?
            .json()
            .await?;
        assert_eq!(mode["status"], 500);
        let payload = serde_json::json!({
            "transaction_id": "t4", "order_id": "o4",
            "merchant_id": "m", "amount": 1000, "status": "SUCCESS"
        });
        let (ts, sig, body) = signed("test-secret", &payload);
        let resp = http
            .post(format!("{base}/webhook"))
            .header("X-Signature", sig)
            .header("X-Timestamp", ts)
            .body(body)
            .send()
            .await?;
        assert_eq!(resp.status(), 500);
        server.abort();
        Ok(())
    }
}
