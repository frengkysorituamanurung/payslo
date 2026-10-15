#![forbid(unsafe_code)]

//! HTTP routes for gateway-api.
//!
//! P2: HTTP 200 does NOT mean payment success — accepted payments return 200
//! with `status: PENDING|FAILED`. Only internal errors/timeouts become 5xx.

use std::sync::Arc;

use axum::{
    Json,
    extract::{Path, State},
    http::HeaderMap,
};
use rand::{RngExt, SeedableRng};
use rand_chacha::ChaCha8Rng;
use serde::{Deserialize, Serialize};
use tokio::sync::{Mutex, RwLock};
use tracing::{info, warn};
use uuid::Uuid;

use crate::bank::{self, ChargeRequest};
use crate::config::Config;
use crate::db;
use crate::domain::{
    Amount, Channel, Provider, ReleaseVersion, RoutingConfig, TxnEvent, TxnStatus,
};
use crate::error::AppError;
use crate::webhook;

/// Shared application state.
pub struct AppState {
    pub pool: sqlx::PgPool,
    pub routing: RwLock<RoutingConfig>,
    pub release: RwLock<ReleaseVersion>,
    pub rng: Mutex<ChaCha8Rng>,
    pub http: reqwest::Client,
    pub config: Config,
}

/// Build the full route table (test-only: production serves the split
/// tables, see `main`). U0 keeps this for integration tests.
/// U0: `main` serves `public_router` on GATEWAY_PORT and `admin_router` on
/// GATEWAY_ADMIN_PORT (127.0.0.1:3001); Envoy blocks /admin/* at the edge.
#[cfg(test)]
pub fn router(state: Arc<AppState>) -> axum::Router {
    public_router(state.clone()).merge(admin_router(state))
}

/// Public ingress table (via Envoy :8080 and direct :3000). No /admin/* here.
pub fn public_router(state: Arc<AppState>) -> axum::Router {
    use axum::routing::{get, post};
    axum::Router::new()
        .route("/healthz", get(healthz))
        .route("/readyz", get(readyz))
        .route("/v1/payments", post(create_payment))
        .route("/v1/payments/{id}", get(get_payment))
        .route("/v1/callbacks/{provider}", post(callback))
        .with_state(state)
}

/// Admin table: loopback-only listener, never exposed via Envoy.
pub fn admin_router(state: Arc<AppState>) -> axum::Router {
    use axum::routing::{get, put};
    axum::Router::new()
        .route("/admin/routing", get(get_routing).put(put_routing))
        .route("/admin/release", put(put_release))
        .with_state(state)
}

impl AppState {
    pub fn new(pool: sqlx::PgPool, config: Config) -> Self {
        let seed = config.seed;
        Self {
            pool,
            routing: RwLock::new(RoutingConfig::default()),
            release: RwLock::new(ReleaseVersion::Good),
            rng: Mutex::new(ChaCha8Rng::seed_from_u64(seed)),
            http: reqwest::Client::new(),
            config,
        }
    }
}

// ------------------------------------------------------------------ DTOs

#[derive(Debug, Deserialize)]
pub struct CreatePayment {
    pub merchant_id: String,
    pub order_id: String,
    pub amount: i64,
    pub channel: String,
}

#[derive(Debug, Serialize)]
pub struct PaymentResponse {
    pub id: Uuid,
    pub merchant_id: String,
    pub order_id: String,
    pub channel: Channel,
    pub provider: Provider,
    pub amount: i64,
    pub fee: i64,
    pub status: TxnStatus,
    pub provider_ref: Option<String>,
    /// RFC 3339 timestamps (DB boundary is text; see db.rs).
    pub created_at: String,
    pub expires_at: String,
}

impl PaymentResponse {
    fn of(t: &db::Transaction) -> Self {
        Self {
            id: t.id,
            merchant_id: t.merchant_id.clone(),
            order_id: t.order_id.clone(),
            channel: t.channel,
            provider: t.provider,
            amount: t.amount.0,
            fee: t.fee.0,
            status: t.status,
            provider_ref: t.provider_ref.clone(),
            created_at: t.created_at.clone(),
            expires_at: t.expires_at.clone(),
        }
    }
}

fn trace_id_from(headers: &HeaderMap) -> Option<&str> {
    headers.get("traceparent").and_then(|v| v.to_str().ok())
}

fn fee_for(channel: Channel) -> Amount {
    match channel {
        Channel::Va => Amount(4000),
        Channel::Ewallet => Amount(2000),
        Channel::Qris => Amount(1500),
    }
}

// ------------------------------------------------------------ POST /v1/payments

pub async fn create_payment(
    State(state): State<Arc<AppState>>,
    headers: HeaderMap,
    Json(body): Json<CreatePayment>,
) -> Result<Json<PaymentResponse>, AppError> {
    if body.merchant_id.trim().is_empty() || body.order_id.trim().is_empty() {
        return Err(AppError::BadRequest(
            "merchant_id and order_id are required".into(),
        ));
    }
    if body.amount <= 0 {
        return Err(AppError::BadRequest(
            "amount must be a positive integer (IDR)".into(),
        ));
    }
    let channel: Channel = body
        .channel
        .parse()
        .map_err(|_| AppError::BadRequest("channel must be VA|EWALLET|QRIS".into()))?;
    let trace_id = trace_id_from(&headers).unwrap_or("-");

    // Release "bad": +latency and 20% fast 500s (deploy-regression scenario).
    if *state.release.read().await == ReleaseVersion::Bad {
        let roll: f64 = state.rng.lock().await.random_range(0.0..1.0);
        tokio::time::sleep(std::time::Duration::from_millis(300)).await;
        if roll < 0.20 {
            warn!(trace_id, "release-bad injected 500");
            return Err(AppError::Internal(anyhow::anyhow!(
                "release-bad injected failure"
            )));
        }
    }

    let provider = {
        let routing = *state.routing.read().await;
        let roll: f64 = state.rng.lock().await.random_range(0.0..1.0);
        crate::domain::pick_provider(channel, routing, roll).ok_or(AppError::NoProvider)?
    };

    let id = Uuid::new_v4();
    let expires_at = (time::OffsetDateTime::now_utc()
        + time::Duration::seconds(state.config.txn_ttl.as_secs() as i64))
    .format(&time::format_description::well_known::Rfc3339)
    .unwrap_or_default();
    db::insert_created(
        &state.pool,
        db::NewTransaction {
            id,
            merchant_id: body.merchant_id.trim(),
            order_id: body.order_id.trim(),
            channel,
            provider,
            amount: Amount(body.amount),
            fee: fee_for(channel),
            expires_at,
        },
    )
    .await?;
    info!(
        transaction_id = %id, %trace_id, provider = %provider,
        status = %TxnStatus::Created, "payment created"
    );

    // Charge the bank (timeout-guarded). Fast rejections -> FAILED (HTTP 200).
    let charge_req = ChargeRequest {
        transaction_id: id.to_string(),
        provider,
        amount: body.amount,
        channel: channel.to_string(),
    };
    let bank_url = state.config.bank_url(provider).to_owned();
    let traceparent = headers
        .get("traceparent")
        .and_then(|v| v.to_str().ok())
        .map(str::to_owned);
    // Every accepted payment reaches PENDING — including charge timeouts.
    // (FASE 2 fix: timeouts used to leak rows in CREATED, invisible to the
    // expiry job, the pending gauge, and late callbacks.)
    match bank::charge(
        &state.http,
        &bank_url,
        &charge_req,
        traceparent.as_deref(),
        state.config.charge_timeout,
    )
    .await
    {
        Ok(bank::ChargeOutcome::Accepted(provider_ref)) => {
            let txn = db::apply_event(
                &state.pool,
                id,
                TxnEvent::ChargeAccepted,
                Some(&provider_ref),
            )
            .await?;
            info!(
                transaction_id = %id, %trace_id, provider = %provider,
                status = %txn.status, "charge accepted, awaiting callback"
            );
            Ok(Json(PaymentResponse::of(&txn)))
        }
        Ok(bank::ChargeOutcome::Rejected) => {
            // CREATED -> PENDING -> FAILED keeps every transition inside the
            // state machine (P8): no jump from CREATED straight to FAILED.
            db::apply_event(&state.pool, id, TxnEvent::ChargeAccepted, None).await?;
            let txn = db::apply_event(&state.pool, id, TxnEvent::BankFailure, None).await?;
            info!(
                transaction_id = %id, %trace_id, provider = %provider,
                status = %txn.status, "bank rejected fast"
            );
            Ok(Json(PaymentResponse::of(&txn)))
        }
        Err(e) => {
            db::apply_event(&state.pool, id, TxnEvent::ChargeAccepted, None).await?;
            warn!(
                transaction_id = %id, %trace_id, provider = %provider,
                error = %e, "charge failed, still PENDING for late callbacks/expiry"
            );
            Err(e)
        }
    }
}

// ------------------------------------------------------- GET /v1/payments/{id}

pub async fn get_payment(
    State(state): State<Arc<AppState>>,
    Path(id): Path<Uuid>,
) -> Result<Json<PaymentResponse>, AppError> {
    Ok(Json(PaymentResponse::of(&db::get(&state.pool, id).await?)))
}

// ------------------------------------------------- POST /v1/callbacks/{provider}

#[derive(Debug, Deserialize)]
pub struct CallbackBody {
    pub provider_ref: String,
    pub status: String,
    /// Present since FASE 2 (bank-sim always sends it). Fallback key when the
    /// gateway never learned the provider_ref (e.g. charge timed out but the
    /// bank still processed and calls back late — `ambiguous` profile).
    pub transaction_id: Option<String>,
}

pub async fn callback(
    State(state): State<Arc<AppState>>,
    headers: HeaderMap,
    Path(provider_raw): Path<String>,
    Json(body): Json<CallbackBody>,
) -> Result<Json<serde_json::Value>, AppError> {
    let provider: Provider = provider_raw
        .parse()
        .map_err(|_| AppError::BadRequest("unknown provider".into()))?;
    let trace_id = trace_id_from(&headers).unwrap_or("-");
    let payload = serde_json::json!({
        "provider_ref": body.provider_ref,
        "status": body.status,
    });

    let mut txn = match db::find_by_provider_ref(&state.pool, &body.provider_ref).await? {
        Some(t) => t,
        None => match body.transaction_id.as_deref() {
            // Fallback: charge timed out before we learned the provider_ref.
            Some(raw) => {
                let id: Uuid = raw
                    .parse()
                    .map_err(|_| AppError::BadRequest("invalid transaction_id".into()))?;
                db::get(&state.pool, id).await?
            }
            None => return Err(AppError::NotFound),
        },
    };
    // A callback may arrive while the row is still CREATED (e.g. crash between
    // insert and charge): walk through PENDING first, then the final event.
    if txn.status == TxnStatus::Created {
        txn = db::apply_event(
            &state.pool,
            txn.id,
            TxnEvent::ChargeAccepted,
            Some(&body.provider_ref),
        )
        .await?;
    }
    // LAB-FLAW: duplicate callbacks are NOT deduplicated — every callback is
    // logged and forwarded as a webhook (scenario 03 shows double credit).
    db::insert_callback_log(&state.pool, txn.id, &provider.to_string(), &payload).await?;

    let event = match body.status.as_str() {
        "SUCCESS" => TxnEvent::BankSuccess,
        "FAILED" => TxnEvent::BankFailure,
        _ => return Err(AppError::BadRequest("status must be SUCCESS|FAILED".into())),
    };
    // LAB-FLAW: duplicate callbacks are NOT deduplicated. When the transaction
    // is already terminal the transition is illegal, but the callback stays
    // logged and is still forwarded as a webhook (double credit downstream).
    let updated = if txn.status.is_terminal() {
        db::get(&state.pool, txn.id).await?
    } else {
        db::apply_event(&state.pool, txn.id, event, None).await?
    };
    info!(
        transaction_id = %txn.id, %trace_id, provider = %provider,
        status = %updated.status, "callback processed"
    );

    // Forward webhook to merchant-sink (retry 3x), record each attempt outcome.
    let hook_payload = serde_json::json!({
        "transaction_id": txn.id.to_string(),
        "order_id": txn.order_id,
        "merchant_id": txn.merchant_id,
        "amount": txn.amount.0,
        "status": updated.status.to_string(),
    });
    let http_status = webhook::deliver(
        &state.http,
        &state.config.merchant_sink_url,
        &state.config.merchant_webhook_secret,
        &hook_payload,
    )
    .await;
    db::insert_webhook_delivery(
        &state.pool,
        txn.id,
        1,
        http_status,
        if http_status.is_some() {
            "DELIVERED"
        } else {
            "FAILED"
        },
    )
    .await?;

    Ok(Json(
        serde_json::json!({ "ok": true, "status": updated.status.to_string() }),
    ))
}

// ------------------------------------------------------------ admin: routing

pub async fn get_routing(State(state): State<Arc<AppState>>) -> Json<RoutingConfig> {
    Json(*state.routing.read().await)
}

pub async fn put_routing(
    State(state): State<Arc<AppState>>,
    Json(routing): Json<RoutingConfig>,
) -> Json<RoutingConfig> {
    *state.routing.write().await = routing;
    info!(?routing, "routing updated");
    Json(routing)
}

// ------------------------------------------------------------ admin: release

#[derive(Debug, Deserialize)]
pub struct ReleaseBody {
    pub version: ReleaseVersion,
}

pub async fn put_release(
    State(state): State<Arc<AppState>>,
    Json(body): Json<ReleaseBody>,
) -> Json<serde_json::Value> {
    *state.release.write().await = body.version;
    info!(version = ?body.version, "release version updated");
    Json(serde_json::json!({ "version": body.version }))
}

// ------------------------------------------------------------ health

pub async fn healthz() -> Json<serde_json::Value> {
    Json(serde_json::json!({ "ok": true }))
}

pub async fn readyz(
    State(state): State<Arc<AppState>>,
) -> Result<Json<serde_json::Value>, AppError> {
    sqlx::query("SELECT 1")
        .fetch_one(&state.pool)
        .await
        .map_err(AppError::Db)?;
    Ok(Json(serde_json::json!({ "ok": true })))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::Duration;

    fn test_config() -> Config {
        Config {
            port: 3000,
            database_url: "postgres://lab_app:lab-only-change-me@127.0.0.1:5432/payslo_lab"
                .to_owned(),
            bank_bca_url: "http://127.0.0.1:4001".to_owned(),
            bank_mandiri_url: "http://127.0.0.1:4002".to_owned(),
            ewallet_url: "http://127.0.0.1:4003".to_owned(),
            merchant_sink_url: "http://127.0.0.1:5000".to_owned(),
            merchant_webhook_secret: "test-secret".to_owned(),
            charge_timeout: Duration::from_secs(5),
            txn_ttl: Duration::from_secs(600),
            expiry_interval: Duration::from_secs(10),
            seed: 42,
            admin_port: 3001,
        }
    }

    fn test_state() -> anyhow::Result<Arc<AppState>> {
        let pool = sqlx::postgres::PgPoolOptions::new()
            .max_connections(1)
            .connect_lazy(&test_config().database_url)?;
        Ok(Arc::new(AppState::new(pool, test_config())))
    }

    async fn serve() -> anyhow::Result<(String, tokio::task::JoinHandle<()>)> {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await?;
        let addr = listener.local_addr()?;
        let handle = tokio::spawn(async move {
            let state = test_state();
            match state {
                Ok(state) => {
                    if axum::serve(listener, router(state)).await.is_err() {
                        // Listener closed; test over.
                    }
                }
                Err(e) => eprintln!("test server setup failed: {e:#}"),
            }
        });
        Ok((format!("http://{addr}"), handle))
    }

    #[tokio::test]
    async fn healthz_ok() -> anyhow::Result<()> {
        let (base, server) = serve().await?;
        let body: serde_json::Value = reqwest::get(format!("{base}/healthz"))
            .await?
            .json()
            .await?;
        assert_eq!(body, serde_json::json!({ "ok": true }));
        server.abort();
        Ok(())
    }

    async fn serve_router(
        make: fn(Arc<AppState>) -> axum::Router,
    ) -> anyhow::Result<(String, tokio::task::JoinHandle<()>)> {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await?;
        let addr = listener.local_addr()?;
        let handle = tokio::spawn(async move {
            match test_state() {
                Ok(state) => {
                    if axum::serve(listener, make(state)).await.is_err() {
                        // Listener closed; test over.
                    }
                }
                Err(e) => eprintln!("test server setup failed: {e:#}"),
            }
        });
        Ok((format!("http://{addr}"), handle))
    }

    /// U0: /admin/* hanya ada di listener admin, tidak di publik (Envoy juga
    /// memblokir prefix /admin/* di edge).
    #[tokio::test]
    async fn admin_routes_only_on_admin_listener() -> anyhow::Result<()> {
        let (pub_base, pub_srv) = serve_router(public_router).await?;
        let (adm_base, adm_srv) = serve_router(admin_router).await?;
        let http = reqwest::Client::new();
        assert_eq!(
            http.get(format!("{pub_base}/admin/routing"))
                .send()
                .await?
                .status(),
            reqwest::StatusCode::NOT_FOUND
        );
        assert!(
            http.get(format!("{adm_base}/admin/routing"))
                .send()
                .await?
                .status()
                .is_success()
        );
        assert!(
            http.get(format!("{pub_base}/healthz"))
                .send()
                .await?
                .status()
                .is_success()
        );
        assert_eq!(
            http.get(format!("{adm_base}/healthz"))
                .send()
                .await?
                .status(),
            reqwest::StatusCode::NOT_FOUND
        );
        pub_srv.abort();
        adm_srv.abort();
        Ok(())
    }

    #[tokio::test]
    async fn admin_routing_roundtrip() -> anyhow::Result<()> {
        let (base, server) = serve().await?;
        let http = reqwest::Client::new();
        let routing: serde_json::Value = http
            .get(format!("{base}/admin/routing"))
            .send()
            .await?
            .json()
            .await?;
        assert_eq!(routing["bca"]["weight"], 60);
        let updated: serde_json::Value = http
            .put(format!("{base}/admin/routing"))
            .json(&serde_json::json!({
                "bca": {"enabled": false, "weight": 60},
                "mandiri": {"enabled": true, "weight": 40},
                "ewallet": {"enabled": true, "weight": 100}
            }))
            .send()
            .await?
            .json()
            .await?;
        assert_eq!(updated["bca"]["enabled"], false);
        server.abort();
        Ok(())
    }

    #[tokio::test]
    async fn admin_release_roundtrip() -> anyhow::Result<()> {
        let (base, server) = serve().await?;
        let http = reqwest::Client::new();
        let resp: serde_json::Value = http
            .put(format!("{base}/admin/release"))
            .json(&serde_json::json!({ "version": "bad" }))
            .send()
            .await?
            .json()
            .await?;
        assert_eq!(resp["version"], "bad");
        server.abort();
        Ok(())
    }

    #[tokio::test]
    async fn create_payment_validation() -> anyhow::Result<()> {
        let (base, server) = serve().await?;
        let http = reqwest::Client::new();
        // amount <= 0 rejected before touching the DB.
        for payload in [
            serde_json::json!({"merchant_id":"m","order_id":"o","amount":0,"channel":"VA"}),
            serde_json::json!({"merchant_id":"m","order_id":"o","amount":-5,"channel":"VA"}),
            serde_json::json!({"merchant_id":"m","order_id":"o","amount":100,"channel":"BOGUS"}),
            serde_json::json!({"merchant_id":"","order_id":"o","amount":100,"channel":"VA"}),
        ] {
            let resp = http
                .post(format!("{base}/v1/payments"))
                .json(&payload)
                .send()
                .await?;
            assert_eq!(resp.status(), 400, "{payload}");
        }
        server.abort();
        Ok(())
    }

    #[tokio::test]
    async fn callback_unknown_provider_rejected() -> anyhow::Result<()> {
        let (base, server) = serve().await?;
        let http = reqwest::Client::new();
        let resp = http
            .post(format!("{base}/v1/callbacks/bogus"))
            .json(&serde_json::json!({"provider_ref":"x","status":"SUCCESS"}))
            .send()
            .await?;
        assert_eq!(resp.status(), 400);
        server.abort();
        Ok(())
    }
}
