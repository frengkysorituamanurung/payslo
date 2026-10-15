#![forbid(unsafe_code)]

//! HMAC-SHA256 webhook verification (constant-time compare).
//! Signing scheme (mirrors gateway-api `webhook::sign`):
//! `signature = hex(hmac_sha256(secret, timestamp + "." + raw_body))`,
//! sent as `X-Signature` with `X-Timestamp`.

use hmac::{Hmac, KeyInit, Mac};
use sha2::Sha256;
use subtle::ConstantTimeEq;

pub fn compute_signature(secret: &str, timestamp: &str, body: &[u8]) -> String {
    match Hmac::<Sha256>::new_from_slice(secret.as_bytes()) {
        Ok(mut mac) => {
            mac.update(timestamp.as_bytes());
            mac.update(b".");
            mac.update(body);
            hex::encode(mac.finalize().into_bytes())
        }
        Err(_) => String::new(),
    }
}

/// Constant-time signature check. Returns `false` on any mismatch or bad hex.
#[must_use]
pub fn verify(secret: &str, timestamp: &str, body: &[u8], signature_hex: &str) -> bool {
    // Reject malformed hex first (length/charset), then compare the hex
    // strings in constant time so timing leaks nothing about the secret.
    if hex::decode(signature_hex.trim()).is_err() {
        return false;
    }
    let expected = compute_signature(secret, timestamp, body);
    expected
        .as_bytes()
        .ct_eq(signature_hex.trim().as_bytes())
        .into()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn valid_signature_verifies() {
        let body = br#"{"order_id":"o1","amount":100}"#;
        let sig = compute_signature("s3cret", "1700000000", body);
        assert!(verify("s3cret", "1700000000", body, &sig));
    }

    #[test]
    fn wrong_signature_rejected() {
        let body = br#"{"order_id":"o1","amount":100}"#;
        assert!(!verify("s3cret", "1700000000", body, &"00".repeat(32)));
        assert!(!verify("s3cret", "1700000000", body, "not-hex!!"));
        // Right signature, wrong key.
        let sig = compute_signature("other", "1700000000", body);
        assert!(!verify("s3cret", "1700000000", body, &sig));
        // Right signature, tampered body.
        let sig = compute_signature("s3cret", "1700000000", body);
        assert!(!verify(
            "s3cret",
            "1700000000",
            br#"{"order_id":"o1","amount":101}"#,
            &sig
        ));
    }
}
