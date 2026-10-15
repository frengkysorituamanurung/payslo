#![forbid(unsafe_code)]

//! Outbound charge call to bank-sim (timeout-guarded, traceparent forwarded).
//! P1: no Prometheus/metrics instrumentation here — only structured logs.

use serde::{Deserialize, Serialize};
use tracing::info;

use crate::domain::Provider;
use crate::error::AppError;

#[derive(Debug, Clone, Serialize)]
pub struct ChargeRequest {
    pub transaction_id: String,
    pub provider: Provider,
    pub amount: i64,
    pub channel: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct ChargeResponse {
    pub provider_ref: String,
}

pub enum ChargeOutcome {
    /// Bank accepted; transaction becomes PENDING and waits for callback.
    Accepted(String),
    /// Bank rejected fast (4xx/5xx); transaction becomes FAILED (HTTP 200).
    Rejected,
}

/// POST `{bank}/charge` with a hard timeout. Forwards `traceparent` when present.
pub async fn charge(
    http: &reqwest::Client,
    bank_base_url: &str,
    req: &ChargeRequest,
    traceparent: Option<&str>,
    timeout: std::time::Duration,
) -> Result<ChargeOutcome, AppError> {
    let url = format!("{bank_base_url}/charge");
    let mut builder = http.post(&url).json(req).timeout(timeout);
    if let Some(tp) = traceparent {
        builder = builder.header("traceparent", tp);
    }
    let resp = builder.send().await.map_err(|e| {
        if e.is_timeout() {
            AppError::BankTimeout
        } else {
            AppError::Downstream(e)
        }
    })?;
    let status = resp.status();
    if status.is_success() {
        let body: ChargeResponse = resp.json().await.map_err(AppError::Downstream)?;
        info!(provider_ref = %body.provider_ref, "bank charge accepted");
        Ok(ChargeOutcome::Accepted(body.provider_ref))
    } else {
        info!(%status, "bank charge rejected fast");
        Ok(ChargeOutcome::Rejected)
    }
}
