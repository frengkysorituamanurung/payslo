#![forbid(unsafe_code)]

//! Bank behavior profiles. Randomness uses `ChaCha8Rng` seeded from `SEED`
//! so every profile is reproducible (same seed => same outcome sequence).
//! Pure decision logic lives in [`decide`] and is unit-tested for determinism.

use rand::{RngExt, SeedableRng};
use rand_chacha::ChaCha8Rng;
use serde::{Deserialize, Serialize};

/// Behavior profile (admin-switchable at runtime).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum Profile {
    /// 100-300ms, 99.5% success.
    Normal,
    /// 5-15s latency (exceeds gateway timeout).
    Slow,
    /// Random 5xx/timeout at `error_rate`.
    Flaky,
    /// Never responds (gateway times out).
    Down,
    /// Processes fine but responds after the gateway timeout (7s).
    Ambiguous,
    /// Responds fast, callback arrives late.
    LateCallback,
    /// Responds fast, sends the callback 2-3x.
    DupCallback,
}

/// Outcome of one charge attempt (before latency is applied).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Decision {
    /// Respond 200 + send callback with this finality.
    Accept(bool),
    /// Respond 500 fast.
    Http500,
    /// Never respond (caller times out).
    Hang,
}

/// Pure decision function: same `(profile, seed, error_rate, call_index)`
/// always yields the same decision. Tested below.
pub fn decide(profile: Profile, rng: &mut ChaCha8Rng, error_rate: f64) -> Decision {
    match profile {
        Profile::Normal => Decision::Accept(rng.random_range(0.0..1.0) >= 0.005),
        Profile::Slow | Profile::LateCallback | Profile::DupCallback | Profile::Ambiguous => {
            Decision::Accept(true)
        }
        Profile::Flaky => {
            if rng.random_range(0.0..1.0) < error_rate {
                if rng.random_bool(0.5) {
                    Decision::Http500
                } else {
                    Decision::Hang
                }
            } else {
                Decision::Accept(true)
            }
        }
        Profile::Down => Decision::Hang,
    }
}

/// Latency range (ms) applied before responding.
pub fn latency_range_ms(profile: Profile, cfg_min: u64, cfg_max: u64) -> (u64, u64) {
    match profile {
        Profile::Normal => (100, 300),
        Profile::Slow => (5000, 15000),
        Profile::Ambiguous => (7000, 7000),
        Profile::Down => (3_600_000, 3_600_000),
        Profile::Flaky | Profile::LateCallback | Profile::DupCallback => (cfg_min, cfg_max),
    }
}

/// Mix a provider name into the base seed so the three instances reproduce
/// independently while staying deterministic.
#[must_use]
pub fn seed_for(base_seed: u64, provider_name: &str) -> u64 {
    let mut hash: u64 = 0xcbf2_9ce4_8422_2325;
    for b in provider_name.bytes() {
        hash ^= u64::from(b);
        hash = hash.wrapping_mul(0x0100_0000_01b3);
    }
    base_seed ^ hash
}

pub fn rng_for(base_seed: u64, provider_name: &str) -> ChaCha8Rng {
    ChaCha8Rng::seed_from_u64(seed_for(base_seed, provider_name))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn decisions(profile: Profile, seed: u64, n: usize) -> Vec<Decision> {
        let mut rng = ChaCha8Rng::seed_from_u64(seed);
        (0..n).map(|_| decide(profile, &mut rng, 0.3)).collect()
    }

    #[test]
    fn same_seed_same_sequence() {
        assert_eq!(
            decisions(Profile::Normal, 42, 50),
            decisions(Profile::Normal, 42, 50)
        );
        assert_eq!(
            decisions(Profile::Flaky, 42, 50),
            decisions(Profile::Flaky, 42, 50)
        );
    }

    #[test]
    fn different_seeds_diverge() {
        assert_ne!(
            decisions(Profile::Flaky, 42, 50),
            decisions(Profile::Flaky, 43, 50)
        );
    }

    #[test]
    fn normal_is_mostly_success() {
        let ok = decisions(Profile::Normal, 7, 1000)
            .iter()
            .filter(|d| **d == Decision::Accept(true))
            .count();
        assert!(ok >= 985, "expected ~99.5% success, got {ok}/1000");
    }

    #[test]
    fn flaky_error_rate_respected() {
        let bad = decisions(Profile::Flaky, 9, 1000)
            .iter()
            .filter(|d| !matches!(d, Decision::Accept(_)))
            .count();
        // error_rate 0.3 over 1000 draws: generous bounds, still deterministic.
        assert!((200..400).contains(&bad), "got {bad}/1000 errors");
    }

    #[test]
    fn zero_error_rate_never_fails() {
        let mut rng = ChaCha8Rng::seed_from_u64(1);
        for _ in 0..100 {
            assert_eq!(
                decide(Profile::Flaky, &mut rng, 0.0),
                Decision::Accept(true)
            );
        }
    }

    #[test]
    fn down_always_hangs() {
        let mut rng = ChaCha8Rng::seed_from_u64(1);
        for _ in 0..10 {
            assert_eq!(decide(Profile::Down, &mut rng, 0.0), Decision::Hang);
        }
    }

    #[test]
    fn provider_seeds_differ_but_stable() {
        assert_eq!(seed_for(42, "bca"), seed_for(42, "bca"));
        assert_ne!(seed_for(42, "bca"), seed_for(42, "mandiri"));
    }
}
