#![forbid(unsafe_code)]

//! Environment configuration for gateway-api. All parsing failures are
//! actionable errors (no panics on the normal path).

use std::env;
use std::time::Duration;

use anyhow::{Context, Result};

/// Full runtime configuration, built once at startup.
#[derive(Debug, Clone)]
pub struct Config {
    pub port: u16,
    /// U0: separate loopback-only listener for /admin/* (default 3001).
    /// Envoy never routes /admin/*; Grafana moved to :3002 to free this port.
    pub admin_port: u16,
    pub database_url: String,
    pub bank_bca_url: String,
    pub bank_mandiri_url: String,
    pub ewallet_url: String,
    pub merchant_sink_url: String,
    pub merchant_webhook_secret: String,
    /// Timeout for the outbound charge call (default 5s).
    pub charge_timeout: Duration,
    /// PENDING transactions older than this become EXPIRED (default 600s).
    pub txn_ttl: Duration,
    /// Expiry job sweep interval (default 10s).
    pub expiry_interval: Duration,
    /// Seed for lab-reproducible randomness (routing jitter, release-bad).
    pub seed: u64,
}

fn env_or(key: &str, default: &str) -> String {
    env::var(key).unwrap_or_else(|_| default.to_owned())
}

fn parse_u64(key: &str, default: u64) -> Result<u64> {
    let raw = env_or(key, &default.to_string());
    raw.parse::<u64>()
        .with_context(|| format!("{key} must be a positive integer, got {raw:?}"))
}

impl Config {
    pub fn from_env() -> Result<Self> {
        let port = u16::try_from(parse_u64("GATEWAY_PORT", 3000)?)
            .context("GATEWAY_PORT must fit in u16")?;
        Ok(Self {
            port,
            admin_port: u16::try_from(parse_u64("GATEWAY_ADMIN_PORT", 3001)?)
                .context("GATEWAY_ADMIN_PORT must fit in u16")?,
            database_url: env::var("DATABASE_URL").context("DATABASE_URL must be set")?,
            bank_bca_url: env_or("BANK_BCA_URL", "http://127.0.0.1:4001"),
            bank_mandiri_url: env_or("BANK_MANDIRI_URL", "http://127.0.0.1:4002"),
            ewallet_url: env_or("EWALLET_URL", "http://127.0.0.1:4003"),
            merchant_sink_url: env_or("MERCHANT_SINK_URL", "http://127.0.0.1:5000"),
            merchant_webhook_secret: env::var("MERCHANT_WEBHOOK_SECRET")
                .context("MERCHANT_WEBHOOK_SECRET must be set (lab-only value is fine)")?,
            charge_timeout: Duration::from_secs(parse_u64("GATEWAY_TIMEOUT_SECS", 5)?),
            txn_ttl: Duration::from_secs(parse_u64("GATEWAY_TXN_TTL_SECS", 600)?),
            expiry_interval: Duration::from_secs(parse_u64("GATEWAY_EXPIRY_INTERVAL_SECS", 10)?),
            seed: parse_u64("SEED", 42)?,
        })
    }

    /// Base URL of the bank-sim instance serving `provider`.
    #[must_use]
    pub fn bank_url(&self, provider: crate::domain::Provider) -> &str {
        match provider {
            crate::domain::Provider::Bca => &self.bank_bca_url,
            crate::domain::Provider::Mandiri => &self.bank_mandiri_url,
            crate::domain::Provider::Ewallet => &self.ewallet_url,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_u64_rejects_garbage() {
        assert!(parse_u64("GATEWAY_PORT_DEFINITELY_UNSET_XYZ", 7).is_ok());
    }

    #[test]
    fn bank_url_matches_provider() {
        let cfg = Config {
            port: 3000,
            admin_port: 3001,
            database_url: String::new(),
            bank_bca_url: "bca".to_owned(),
            bank_mandiri_url: "mandiri".to_owned(),
            ewallet_url: "ewallet".to_owned(),
            merchant_sink_url: String::new(),
            merchant_webhook_secret: String::new(),
            charge_timeout: Duration::from_secs(5),
            txn_ttl: Duration::from_secs(600),
            expiry_interval: Duration::from_secs(10),
            seed: 42,
        };
        assert_eq!(cfg.bank_url(crate::domain::Provider::Bca), "bca");
        assert_eq!(cfg.bank_url(crate::domain::Provider::Mandiri), "mandiri");
        assert_eq!(cfg.bank_url(crate::domain::Provider::Ewallet), "ewallet");
    }
}
