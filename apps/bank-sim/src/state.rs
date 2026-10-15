#![forbid(unsafe_code)]

//! Runtime state: admin-overridable profile knobs + charge registry.

use std::collections::HashMap;
use std::sync::Arc;

use serde::{Deserialize, Serialize};
use tokio::sync::Mutex;

use crate::config::Config;
use crate::profile::{Profile, rng_for};

/// Admin-overridable knobs (PUT /admin/profile, partial).
#[derive(Debug, Clone, Default, Deserialize)]
pub struct ProfilePatch {
    pub profile: Option<Profile>,
    pub error_rate: Option<f64>,
    pub latency_ms_min: Option<u64>,
    pub latency_ms_max: Option<u64>,
    pub callback_delay_s: Option<u64>,
    pub duplicate_callbacks: Option<u32>,
}

#[derive(Debug, Clone, Serialize)]
pub struct ProfileView {
    pub provider: String,
    pub profile: Profile,
    pub error_rate: f64,
    pub latency_ms_min: u64,
    pub latency_ms_max: u64,
    pub callback_delay_s: u64,
    pub duplicate_callbacks: u32,
}

/// Status of a charge as known by the bank.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
pub enum BankTxnStatus {
    Charged,
    Success,
    Failed,
}

pub struct Shared {
    pub provider_name: String,
    pub gateway_url: String,
    pub inner: Mutex<StateInner>,
    pub http: reqwest::Client,
}

pub struct StateInner {
    pub profile: Profile,
    pub error_rate: f64,
    pub latency_ms_min: u64,
    pub latency_ms_max: u64,
    pub callback_delay_s: u64,
    pub duplicate_callbacks: u32,
    pub rng: rand_chacha::ChaCha8Rng,
    pub charges: HashMap<String, BankTxnStatus>,
    pub counter: u64,
    defaults: Defaults,
}

#[derive(Debug, Clone)]
struct Defaults {
    profile: Profile,
    error_rate: f64,
    latency_ms_min: u64,
    latency_ms_max: u64,
    callback_delay_s: u64,
    duplicate_callbacks: u32,
    seed: u64,
    provider_name: String,
}

impl Shared {
    pub fn new(config: &Config) -> Arc<Self> {
        let defaults = Defaults {
            profile: config.profile,
            error_rate: config.error_rate,
            latency_ms_min: config.latency_ms_min,
            latency_ms_max: config.latency_ms_max,
            callback_delay_s: config.callback_delay_s,
            duplicate_callbacks: config.duplicate_callbacks,
            seed: config.seed,
            provider_name: config.provider_name.clone(),
        };
        Arc::new(Self {
            provider_name: config.provider_name.clone(),
            gateway_url: config.gateway_url.clone(),
            inner: Mutex::new(StateInner {
                profile: config.profile,
                error_rate: config.error_rate,
                latency_ms_min: config.latency_ms_min,
                latency_ms_max: config.latency_ms_max,
                callback_delay_s: config.callback_delay_s,
                duplicate_callbacks: config.duplicate_callbacks,
                rng: rng_for(config.seed, &config.provider_name),
                charges: HashMap::new(),
                counter: 0,
                defaults,
            }),
            http: reqwest::Client::new(),
        })
    }

    pub async fn view(&self) -> ProfileView {
        let s = self.inner.lock().await;
        ProfileView {
            provider: self.provider_name.clone(),
            profile: s.profile,
            error_rate: s.error_rate,
            latency_ms_min: s.latency_ms_min,
            latency_ms_max: s.latency_ms_max,
            callback_delay_s: s.callback_delay_s,
            duplicate_callbacks: s.duplicate_callbacks,
        }
    }

    /// Restore env defaults (POST /admin/reset), including RNG re-seed.
    pub async fn reset(&self) {
        let mut s = self.inner.lock().await;
        let d = s.defaults.clone();
        s.profile = d.profile;
        s.error_rate = d.error_rate;
        s.latency_ms_min = d.latency_ms_min;
        s.latency_ms_max = d.latency_ms_max;
        s.callback_delay_s = d.callback_delay_s;
        s.duplicate_callbacks = d.duplicate_callbacks;
        s.rng = rng_for(d.seed, &d.provider_name);
    }
}
