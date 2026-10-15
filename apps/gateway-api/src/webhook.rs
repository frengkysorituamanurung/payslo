#![forbid(unsafe_code)]

//! merchant-sink webhook delivery: HMAC-SHA256 (`X-Signature`, `X-Timestamp`),
//! retry 3x with backoff. Shared signing scheme documented in merchant-sink README:
//! `signature = hex(hmac_sha256(secret, timestamp + "." + raw_body))`.

use hmac::{Hmac, KeyInit, Mac};
use sha2::Sha256;
use tracing::{info, warn};

pub type HmacSha256 = Hmac<Sha256>;

/// Compute the `X-Signature` value for a raw JSON body.
#[must_use]
pub fn sign(secret: &str, timestamp: &str, body: &[u8]) -> String {
    // HMAC accepts any key length, so `new_from_slice` cannot fail here; the
    // empty-string fallback only satisfies the no-unwrap lint.
    match HmacSha256::new_from_slice(secret.as_bytes()) {
        Ok(mut mac) => {
            mac.update(timestamp.as_bytes());
            mac.update(b".");
            mac.update(body);
            hex::encode(mac.finalize().into_bytes())
        }
        Err(_) => String::new(),
    }
}

/// Current Unix timestamp (seconds) for `X-Timestamp`.
#[must_use]
pub fn now_timestamp() -> String {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs().to_string())
        .unwrap_or_else(|_| "0".to_owned())
}

/// Deliver one webhook, retrying 3 attempts with backoff. Returns the last
/// HTTP status on success, or `None` when all attempts failed.
pub async fn deliver(
    http: &reqwest::Client,
    sink_base_url: &str,
    secret: &str,
    payload: &serde_json::Value,
) -> Option<i32> {
    let body = serde_json::to_vec(payload).unwrap_or_default();
    let timestamp = now_timestamp();
    let signature = sign(secret, &timestamp, &body);
    let url = format!("{sink_base_url}/webhook");
    let backoffs = [
        std::time::Duration::from_millis(500),
        std::time::Duration::from_secs(1),
        std::time::Duration::from_secs(2),
    ];
    for (i, backoff) in backoffs.iter().enumerate() {
        let attempt = i + 1;
        match http
            .post(&url)
            .header("Content-Type", "application/json")
            .header("X-Signature", &signature)
            .header("X-Timestamp", &timestamp)
            .body(body.clone())
            .timeout(std::time::Duration::from_secs(5))
            .send()
            .await
        {
            Ok(resp) => {
                let status = resp.status().as_u16() as i32;
                if resp.status().is_success() {
                    info!(attempt, status, "webhook delivered");
                    return Some(status);
                }
                warn!(attempt, status, "webhook rejected, retrying");
            }
            Err(e) => {
                warn!(attempt, error = %e, "webhook error, retrying");
            }
        }
        tokio::time::sleep(*backoff).await;
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn signature_is_deterministic() {
        let a = sign("secret", "1700000000", b"{\"a\":1}");
        let b = sign("secret", "1700000000", b"{\"a\":1}");
        assert_eq!(a, b);
        assert_eq!(a.len(), 64);
    }

    #[test]
    fn signature_changes_with_key_body_or_timestamp() {
        let base = sign("secret", "1700000000", b"{\"a\":1}");
        assert_ne!(base, sign("other", "1700000000", b"{\"a\":1}"));
        assert_ne!(base, sign("secret", "1700000001", b"{\"a\":1}"));
        assert_ne!(base, sign("secret", "1700000000", b"{\"a\":2}"));
    }
}
