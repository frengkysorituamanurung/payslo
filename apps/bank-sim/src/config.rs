#![forbid(unsafe_code)]

//! Env-driven configuration for bank-sim. One binary, three instances via
//! `PROVIDER_NAME` (bank-bca, bank-mandiri, ewallet-sim).

use anyhow::{Context, Result};

use crate::profile::Profile;

#[derive(Debug, Clone)]
pub struct Config {
    pub provider_name: String,
    pub port: u16,
    pub profile: Profile,
    pub error_rate: f64,
    pub latency_ms_min: u64,
    pub latency_ms_max: u64,
    pub callback_delay_s: u64,
    pub duplicate_callbacks: u32,
    pub gateway_url: String,
    pub seed: u64,
}

fn env_or(key: &str, default: &str) -> String {
    std::env::var(key).unwrap_or_else(|_| default.to_owned())
}

fn parse_u64(key: &str, default: u64) -> Result<u64> {
    std::env::var(key).ok().map_or(Ok(default), |raw| {
        raw.parse::<u64>()
            .with_context(|| format!("{key} must be a positive integer, got {raw:?}"))
    })
}

fn parse_profile(raw: &str) -> Result<Profile> {
    match raw {
        "normal" => Ok(Profile::Normal),
        "slow" => Ok(Profile::Slow),
        "flaky" => Ok(Profile::Flaky),
        "down" => Ok(Profile::Down),
        "ambiguous" => Ok(Profile::Ambiguous),
        "late-callback" => Ok(Profile::LateCallback),
        "dup-callback" => Ok(Profile::DupCallback),
        _ => anyhow::bail!(
            "PROFILE must be one of normal|slow|flaky|down|ambiguous|late-callback|dup-callback, got {raw:?}"
        ),
    }
}

impl Config {
    pub fn from_env() -> Result<Self> {
        let port =
            u16::try_from(parse_u64("BANK_PORT", 4000)?).context("BANK_PORT must fit in u16")?;
        let error_rate = env_or("ERROR_RATE", "0.2")
            .parse::<f64>()
            .context("ERROR_RATE must be a float in [0,1]")?;
        if !(0.0..=1.0).contains(&error_rate) {
            anyhow::bail!("ERROR_RATE must be in [0,1]");
        }
        Ok(Self {
            provider_name: env_or("PROVIDER_NAME", "bca"),
            port,
            profile: parse_profile(&env_or("PROFILE", "normal"))?,
            error_rate,
            latency_ms_min: parse_u64("LATENCY_MS_MIN", 100)?,
            latency_ms_max: parse_u64("LATENCY_MS_MAX", 300)?,
            callback_delay_s: parse_u64("CALLBACK_DELAY_S", 1)?,
            duplicate_callbacks: u32::try_from(parse_u64("DUPLICATE_CALLBACKS", 1)?)
                .context("DUPLICATE_CALLBACKS must fit in u32")?,
            gateway_url: env_or("GATEWAY_URL", "http://127.0.0.1:3000"),
            seed: parse_u64("SEED", 42)?,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn profile_names_parse() {
        assert!(matches!(parse_profile("normal"), Ok(Profile::Normal)));
        assert!(matches!(
            parse_profile("dup-callback"),
            Ok(Profile::DupCallback)
        ));
        assert!(parse_profile("bogus").is_err());
    }
}
