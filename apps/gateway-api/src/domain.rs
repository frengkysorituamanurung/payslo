#![forbid(unsafe_code)]

//! Domain types for gateway-api.
//!
//! P7: money is always `Amount(i64)` (minor IDR units). `f32`/`f64` for money
//! is forbidden by code review (no float appears in this module).
//! P8: transaction lifecycle is a Rust enum; illegal transitions are rejected
//! by [`TxnStatus::transition`] and covered by tests.

use std::fmt::Formatter;
use std::str::FromStr;

use serde::{Deserialize, Serialize};
use thiserror::Error;

/// Money in minor IDR units. Never float.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Serialize, Deserialize)]
pub struct Amount(pub i64);

/// Payment channel requested by the merchant.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "UPPERCASE")]
pub enum Channel {
    Va,
    Ewallet,
    Qris,
}

/// Downstream provider chosen by the router.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Provider {
    Bca,
    Mandiri,
    Ewallet,
}

/// Transaction lifecycle state machine (P8).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "UPPERCASE")]
pub enum TxnStatus {
    Created,
    Pending,
    Success,
    Failed,
    Expired,
}

/// Event that drives a status transition.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TxnEvent {
    /// Gateway accepted the payment and sent a charge to the bank.
    ChargeAccepted,
    /// Bank reported success via callback.
    BankSuccess,
    /// Bank reported failure via callback, or the charge call failed fast.
    BankFailure,
    /// Background job found a PENDING transaction past `expires_at`.
    Expired,
}

/// Illegal state transition (P8: must be unrepresentable via this error).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Error)]
#[error("illegal transaction transition from {from:?} on event {event:?}")]
pub struct IllegalTransition {
    pub from: TxnStatus,
    pub event: TxnEvent,
}

impl TxnStatus {
    /// Legal transitions:
    /// CREATED -> PENDING (ChargeAccepted)
    /// PENDING -> SUCCESS (BankSuccess) | FAILED (BankFailure) | EXPIRED (Expired)
    /// Terminal states (SUCCESS/FAILED/EXPIRED) have no outgoing transitions.
    pub fn transition(self, event: TxnEvent) -> Result<Self, IllegalTransition> {
        match (self, event) {
            (Self::Created, TxnEvent::ChargeAccepted) => Ok(Self::Pending),
            (Self::Pending, TxnEvent::BankSuccess) => Ok(Self::Success),
            (Self::Pending, TxnEvent::BankFailure) => Ok(Self::Failed),
            (Self::Pending, TxnEvent::Expired) => Ok(Self::Expired),
            _ => Err(IllegalTransition { from: self, event }),
        }
    }

    /// Whether the status is terminal (no further transitions legal).
    #[must_use]
    pub fn is_terminal(self) -> bool {
        matches!(self, Self::Success | Self::Failed | Self::Expired)
    }
}

/// Parse error for [`TxnStatus`], [`Channel`] and [`Provider`].
#[derive(Debug, Clone, Copy, PartialEq, Eq, Error)]
#[error("unknown enum value")]
pub struct ParseEnumError;

impl FromStr for TxnStatus {
    type Err = ParseEnumError;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s {
            "CREATED" => Ok(Self::Created),
            "PENDING" => Ok(Self::Pending),
            "SUCCESS" => Ok(Self::Success),
            "FAILED" => Ok(Self::Failed),
            "EXPIRED" => Ok(Self::Expired),
            _ => Err(ParseEnumError),
        }
    }
}

impl std::fmt::Display for TxnStatus {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        let s = match self {
            Self::Created => "CREATED",
            Self::Pending => "PENDING",
            Self::Success => "SUCCESS",
            Self::Failed => "FAILED",
            Self::Expired => "EXPIRED",
        };
        f.write_str(s)
    }
}

impl FromStr for Channel {
    type Err = ParseEnumError;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s {
            "VA" => Ok(Self::Va),
            "EWALLET" => Ok(Self::Ewallet),
            "QRIS" => Ok(Self::Qris),
            _ => Err(ParseEnumError),
        }
    }
}

impl std::fmt::Display for Channel {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        let s = match self {
            Self::Va => "VA",
            Self::Ewallet => "EWALLET",
            Self::Qris => "QRIS",
        };
        f.write_str(s)
    }
}

impl FromStr for Provider {
    type Err = ParseEnumError;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s {
            "bca" => Ok(Self::Bca),
            "mandiri" => Ok(Self::Mandiri),
            "ewallet" => Ok(Self::Ewallet),
            _ => Err(ParseEnumError),
        }
    }
}

impl std::fmt::Display for Provider {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        let s = match self {
            Self::Bca => "bca",
            Self::Mandiri => "mandiri",
            Self::Ewallet => "ewallet",
        };
        f.write_str(s)
    }
}

/// Per-provider routing entry (admin-controlled).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub struct ProviderRoute {
    pub enabled: bool,
    pub weight: u32,
}

/// Full routing table (admin-controlled).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub struct RoutingConfig {
    pub bca: ProviderRoute,
    pub mandiri: ProviderRoute,
    pub ewallet: ProviderRoute,
}

impl Default for RoutingConfig {
    /// Spec defaults: VA bca 60 / mandiri 40; EWALLET & QRIS ewallet 100.
    fn default() -> Self {
        Self {
            bca: ProviderRoute {
                enabled: true,
                weight: 60,
            },
            mandiri: ProviderRoute {
                enabled: true,
                weight: 40,
            },
            ewallet: ProviderRoute {
                enabled: true,
                weight: 100,
            },
        }
    }
}

impl RoutingConfig {
    fn route_of(self, provider: Provider) -> ProviderRoute {
        match provider {
            Provider::Bca => self.bca,
            Provider::Mandiri => self.mandiri,
            Provider::Ewallet => self.ewallet,
        }
    }
}

/// Weighted provider pick. `roll` must be in `[0.0, 1.0)`.
/// Returns `None` when no provider is eligible for the channel.
pub fn pick_provider(channel: Channel, routing: RoutingConfig, roll: f64) -> Option<Provider> {
    let candidates: &[(Provider, ProviderRoute)] = match channel {
        Channel::Va => &[
            (Provider::Bca, routing.route_of(Provider::Bca)),
            (Provider::Mandiri, routing.route_of(Provider::Mandiri)),
        ],
        Channel::Ewallet | Channel::Qris => {
            &[(Provider::Ewallet, routing.route_of(Provider::Ewallet))]
        }
    };
    let total: u32 = candidates
        .iter()
        .filter(|(_, r)| r.enabled && r.weight > 0)
        .map(|(_, r)| r.weight)
        .sum();
    if total == 0 {
        return None;
    }
    let mut cursor = roll * f64::from(total);
    for (provider, route) in candidates {
        if !route.enabled || route.weight == 0 {
            continue;
        }
        cursor -= f64::from(route.weight);
        if cursor < 0.0 {
            return Some(*provider);
        }
    }
    // Float rounding fallback: last eligible provider.
    candidates
        .iter()
        .rev()
        .find(|(_, r)| r.enabled && r.weight > 0)
        .map(|(p, _)| *p)
}

/// Release version for deploy-regression scenario.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum ReleaseVersion {
    Good,
    Bad,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn legal_transitions() {
        assert_eq!(
            TxnStatus::Created.transition(TxnEvent::ChargeAccepted),
            Ok(TxnStatus::Pending)
        );
        assert_eq!(
            TxnStatus::Pending.transition(TxnEvent::BankSuccess),
            Ok(TxnStatus::Success)
        );
        assert_eq!(
            TxnStatus::Pending.transition(TxnEvent::BankFailure),
            Ok(TxnStatus::Failed)
        );
        assert_eq!(
            TxnStatus::Pending.transition(TxnEvent::Expired),
            Ok(TxnStatus::Expired)
        );
    }

    #[test]
    fn illegal_transitions_rejected() {
        // Terminal states have no outgoing transitions.
        for from in [TxnStatus::Success, TxnStatus::Failed, TxnStatus::Expired] {
            for event in [
                TxnEvent::ChargeAccepted,
                TxnEvent::BankSuccess,
                TxnEvent::BankFailure,
                TxnEvent::Expired,
            ] {
                assert!(from.transition(event).is_err(), "{from:?} + {event:?}");
            }
        }
        // Skipping PENDING is illegal.
        assert!(
            TxnStatus::Created
                .transition(TxnEvent::BankSuccess)
                .is_err()
        );
        assert!(
            TxnStatus::Created
                .transition(TxnEvent::BankFailure)
                .is_err()
        );
        assert!(TxnStatus::Created.transition(TxnEvent::Expired).is_err());
        // Re-entering PENDING is illegal.
        assert!(
            TxnStatus::Pending
                .transition(TxnEvent::ChargeAccepted)
                .is_err()
        );
    }

    #[test]
    fn status_db_roundtrip() {
        for s in [
            TxnStatus::Created,
            TxnStatus::Pending,
            TxnStatus::Success,
            TxnStatus::Failed,
            TxnStatus::Expired,
        ] {
            assert_eq!(s.to_string().parse(), Ok(s));
        }
        assert!("BOGUS".parse::<TxnStatus>().is_err());
    }

    #[test]
    fn router_default_weights() {
        let routing = RoutingConfig::default();
        // VA: bca 60 / mandiri 40.
        assert_eq!(
            pick_provider(Channel::Va, routing, 0.0),
            Some(Provider::Bca)
        );
        assert_eq!(
            pick_provider(Channel::Va, routing, 0.59),
            Some(Provider::Bca)
        );
        assert_eq!(
            pick_provider(Channel::Va, routing, 0.6),
            Some(Provider::Mandiri)
        );
        assert_eq!(
            pick_provider(Channel::Va, routing, 0.99),
            Some(Provider::Mandiri)
        );
        // EWALLET & QRIS always ewallet.
        assert_eq!(
            pick_provider(Channel::Ewallet, routing, 0.99),
            Some(Provider::Ewallet)
        );
        assert_eq!(
            pick_provider(Channel::Qris, routing, 0.0),
            Some(Provider::Ewallet)
        );
    }

    #[test]
    fn router_disabled_provider_excluded() {
        let mut routing = RoutingConfig::default();
        routing.bca.enabled = false;
        assert_eq!(
            pick_provider(Channel::Va, routing, 0.0),
            Some(Provider::Mandiri)
        );
        routing.mandiri.enabled = false;
        assert_eq!(pick_provider(Channel::Va, routing, 0.5), None);
        routing.ewallet.enabled = false;
        assert_eq!(pick_provider(Channel::Qris, routing, 0.5), None);
    }
}
