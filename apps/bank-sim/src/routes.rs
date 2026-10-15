#![forbid(unsafe_code)]

//! bank-sim HTTP routes.

use std::sync::Arc;
use std::time::Duration;

use axum::{
    Json,
    extract::{Path, State},
    http::{HeaderMap, StatusCode},
};
use rand::RngExt;
use serde::{Deserialize, Serialize};
use tracing::{info, warn};

use crate::profile::{Decision, Profile, decide, latency_range_ms};
use crate::state::{BankTxnStatus, ProfilePatch, Shared};

#[derive(Debug, Deserialize)]
pub struct ChargeBody {
    pub transaction_id: String,
    // Echoed by the gateway; kept for forward-compat and trainee inspection.
    #[allow(dead_code)]
    pub provider: Option<String>,
    #[allow(dead_code)]
    pub amount: Option<i64>,
    #[allow(dead_code)]
    pub channel: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct ChargeOk {
    pub provider_ref: String,
}

/// POST /charge: apply profile behavior, respond, then callback async.
pub async fn charge(
    State(shared): State<Arc<Shared>>,
    headers: HeaderMap,
    Json(body): Json<ChargeBody>,
) -> Result<Json<ChargeOk>, (StatusCode, Json<serde_json::Value>)> {
    let traceparent = headers
        .get("traceparent")
        .and_then(|v| v.to_str().ok())
        .map(str::to_owned);

    let (provider_ref, profile, delay_ms, decision, delay_cb, dupes, gateway_url) = {
        let mut s = shared.inner.lock().await;
        s.counter += 1;
        let provider_ref = format!("P-{}-{:04}", shared.provider_name, s.counter);
        // Copy out of the MutexGuard first: disjoint field borrows through a
        // guard deref are not allowed, so snapshot into locals.
        let prof = s.profile;
        let err_rate = s.error_rate;
        let lo_min = s.latency_ms_min;
        let lo_max = s.latency_ms_max;
        let cb_delay = s.callback_delay_s;
        let dupes_cfg = s.duplicate_callbacks.max(1);
        let (lo, hi) = latency_range_ms(prof, lo_min, lo_max);
        let delay_ms = s.rng.random_range(lo..=hi);
        let decision = decide(prof, &mut s.rng, err_rate);
        s.charges
            .insert(provider_ref.clone(), BankTxnStatus::Charged);
        (
            provider_ref,
            prof,
            delay_ms,
            decision,
            cb_delay,
            dupes_cfg,
            shared.gateway_url.clone(),
        )
    };

    info!(
        provider_ref = %provider_ref, transaction_id = %body.transaction_id,
        amount = ?body.amount, channel = ?body.channel,
        profile = ?profile, latency_ms = delay_ms, decision = ?decision,
        "charge received"
    );

    // Simulate processing latency. `down` sleeps ~1h so the gateway times out.
    tokio::time::sleep(Duration::from_millis(delay_ms)).await;

    match decision {
        Decision::Http500 => {
            warn!(provider_ref = %provider_ref, "flaky 500 injected");
            Err((
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(serde_json::json!({ "error": "bank flaky 500 (lab)" })),
            ))
        }
        Decision::Hang => {
            // Keep hanging until the caller gives up; sleep in chunks so a
            // profile switch/reset does not leave us wedged forever.
            loop {
                tokio::time::sleep(Duration::from_secs(60)).await;
            }
        }
        Decision::Accept(success) => {
            let status = if success {
                BankTxnStatus::Success
            } else {
                BankTxnStatus::Failed
            };
            shared
                .inner
                .lock()
                .await
                .charges
                .insert(provider_ref.clone(), status);
            let finality = if success { "SUCCESS" } else { "FAILED" }.to_owned();
            let sends = if profile == Profile::DupCallback {
                dupes.clamp(2, 3)
            } else {
                1
            };
            let tp = traceparent.clone();
            let http_cb = shared.http.clone();
            let ref_cb = provider_ref.clone();
            // Late-callback stretch (+20s) encoded here so the job stays small.
            let effective_delay = if profile == Profile::LateCallback {
                delay_cb + 20
            } else {
                delay_cb
            };
            let job = CallbackJob {
                gateway_url,
                provider_name: shared.provider_name.clone(),
                provider_ref: ref_cb,
                transaction_id: body.transaction_id.clone(),
                finality,
                delay_s: effective_delay,
                sends,
                traceparent: tp,
            };
            tokio::spawn(async move {
                send_callbacks(&http_cb, job).await;
            });
            Ok(Json(ChargeOk { provider_ref }))
        }
    }
}

/// One async callback delivery job (keeps arg count small for clippy).
struct CallbackJob {
    gateway_url: String,
    provider_name: String,
    provider_ref: String,
    transaction_id: String,
    finality: String,
    delay_s: u64,
    sends: u32,
    traceparent: Option<String>,
}

async fn send_callbacks(http: &reqwest::Client, job: CallbackJob) {
    // Late-callback profile stretches the configured delay (+20s) for the lab.
    // The job carries no profile flag, so late-ness is encoded at spawn time:
    // `delay_s` already includes the +20s stretch (see `charge`).
    tokio::time::sleep(Duration::from_secs(job.delay_s)).await;
    for i in 0..job.sends {
        if i > 0 {
            tokio::time::sleep(Duration::from_millis(200)).await;
        }
        let url = format!("{}/v1/callbacks/{}", job.gateway_url, job.provider_name);
        let mut req = http.post(&url).json(&serde_json::json!({
            "provider_ref": job.provider_ref,
            "transaction_id": job.transaction_id,
            "status": job.finality,
        }));
        if let Some(tp) = job.traceparent.as_deref() {
            req = req.header("traceparent", tp);
        }
        match req.send().await {
            Ok(resp) => {
                info!(provider_ref = %job.provider_ref, status = %resp.status(), attempt = i + 1, "callback sent")
            }
            Err(e) => {
                warn!(provider_ref = %job.provider_ref, error = %e, attempt = i + 1, "callback failed")
            }
        }
    }
}

/// GET /status/{provider_ref}: bank-side view for trainees.
pub async fn status(
    State(shared): State<Arc<Shared>>,
    Path(provider_ref): Path<String>,
) -> Result<Json<serde_json::Value>, (StatusCode, Json<serde_json::Value>)> {
    let s = shared.inner.lock().await;
    match s.charges.get(&provider_ref) {
        Some(st) => Ok(Json(serde_json::json!({
            "provider": shared.provider_name,
            "provider_ref": provider_ref,
            "status": format!("{st:?}").to_uppercase(),
        }))),
        None => Err((
            StatusCode::NOT_FOUND,
            Json(serde_json::json!({ "error": "unknown provider_ref" })),
        )),
    }
}

/// GET /admin/profile.
pub async fn get_profile(State(shared): State<Arc<Shared>>) -> Json<crate::state::ProfileView> {
    Json(shared.view().await)
}

/// PUT /admin/profile (partial patch; validates ranges).
pub async fn put_profile(
    State(shared): State<Arc<Shared>>,
    Json(patch): Json<ProfilePatch>,
) -> Result<Json<crate::state::ProfileView>, (StatusCode, Json<serde_json::Value>)> {
    let bad = |msg: &str| {
        (
            StatusCode::BAD_REQUEST,
            Json(serde_json::json!({ "error": msg })),
        )
    };
    if let Some(r) = patch.error_rate
        && !(0.0..=1.0).contains(&r)
    {
        return Err(bad("error_rate must be in [0,1]"));
    }
    let mut s = shared.inner.lock().await;
    if let Some(p) = patch.profile {
        s.profile = p;
    }
    if let Some(r) = patch.error_rate {
        s.error_rate = r;
    }
    if let Some(v) = patch.latency_ms_min {
        s.latency_ms_min = v;
    }
    if let Some(v) = patch.latency_ms_max {
        s.latency_ms_max = v;
    }
    if s.latency_ms_min > s.latency_ms_max {
        return Err(bad("latency_ms_min must be <= latency_ms_max"));
    }
    if let Some(v) = patch.callback_delay_s {
        s.callback_delay_s = v;
    }
    if let Some(v) = patch.duplicate_callbacks {
        s.duplicate_callbacks = v;
    }
    info!(profile = ?s.profile, "bank profile updated");
    drop(s);
    Ok(Json(shared.view().await))
}

/// POST /admin/reset: restore startup defaults.
pub async fn reset(State(shared): State<Arc<Shared>>) -> Json<serde_json::Value> {
    shared.reset().await;
    Json(serde_json::json!({ "ok": true }))
}

pub async fn healthz() -> Json<serde_json::Value> {
    Json(serde_json::json!({ "ok": true }))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::Config;
    use crate::state::Shared;

    fn test_shared() -> Arc<Shared> {
        let config = Config {
            provider_name: "bca".to_owned(),
            port: 4000,
            profile: Profile::Normal,
            error_rate: 0.0,
            latency_ms_min: 1,
            latency_ms_max: 2,
            callback_delay_s: 0,
            duplicate_callbacks: 1,
            gateway_url: "http://127.0.0.1:9".to_owned(),
            seed: 42,
        };
        Shared::new(&config)
    }

    fn test_router(shared: Arc<Shared>) -> axum::Router {
        use axum::routing::{get, post};
        axum::Router::new()
            .route("/healthz", get(healthz))
            .route("/charge", post(charge))
            .route("/status/{provider_ref}", get(status))
            .route("/admin/profile", get(get_profile).put(put_profile))
            .route("/admin/reset", post(reset))
            .with_state(shared)
    }

    async fn serve() -> anyhow::Result<(String, reqwest::Client, tokio::task::JoinHandle<()>)> {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await?;
        let addr = listener.local_addr()?;
        let handle = tokio::spawn(async move {
            let app = test_router(test_shared());
            if axum::serve(listener, app).await.is_err() {
                // Listener closed; test over.
            }
        });
        Ok((format!("http://{addr}"), reqwest::Client::new(), handle))
    }

    #[tokio::test]
    async fn charge_and_status() -> anyhow::Result<()> {
        let (base, http, server) = serve().await?;
        let charge: serde_json::Value = http
            .post(format!("{base}/charge"))
            .json(&serde_json::json!({
                "transaction_id": "txn-1",
                "provider": "bca",
                "amount": 100000,
                "channel": "VA"
            }))
            .send()
            .await?
            .json()
            .await?;
        let provider_ref = charge["provider_ref"]
            .as_str()
            .unwrap_or_default()
            .to_owned();
        assert!(provider_ref.starts_with("P-bca-"), "{charge}");
        // error_rate 0.0 + normal profile: deterministic success.
        let st: serde_json::Value = http
            .get(format!("{base}/status/{provider_ref}"))
            .send()
            .await?
            .json()
            .await?;
        assert_eq!(st["status"], "SUCCESS", "{st}");
        // Unknown ref -> 404.
        let resp = http.get(format!("{base}/status/P-bca-9999")).send().await?;
        assert_eq!(resp.status(), 404);
        server.abort();
        Ok(())
    }

    #[tokio::test]
    async fn admin_profile_roundtrip_and_validation() -> anyhow::Result<()> {
        let (base, http, server) = serve().await?;
        let view: serde_json::Value = http
            .get(format!("{base}/admin/profile"))
            .send()
            .await?
            .json()
            .await?;
        assert_eq!(view["profile"], "normal");
        let updated: serde_json::Value = http
            .put(format!("{base}/admin/profile"))
            .json(&serde_json::json!({"profile": "flaky", "error_rate": 0.5}))
            .send()
            .await?
            .json()
            .await?;
        assert_eq!(updated["profile"], "flaky");
        // Invalid knobs rejected.
        let resp = http
            .put(format!("{base}/admin/profile"))
            .json(&serde_json::json!({"error_rate": 2.0}))
            .send()
            .await?;
        assert_eq!(resp.status(), 400);
        let resp = http
            .put(format!("{base}/admin/profile"))
            .json(&serde_json::json!({"latency_ms_min": 500, "latency_ms_max": 100}))
            .send()
            .await?;
        assert_eq!(resp.status(), 400);
        // Reset restores startup defaults.
        let reset: serde_json::Value = http
            .post(format!("{base}/admin/reset"))
            .send()
            .await?
            .json()
            .await?;
        assert_eq!(reset["ok"], true);
        let view: serde_json::Value = http
            .get(format!("{base}/admin/profile"))
            .send()
            .await?
            .json()
            .await?;
        assert_eq!(view["profile"], "normal");
        server.abort();
        Ok(())
    }
}
