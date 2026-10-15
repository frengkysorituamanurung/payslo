#![forbid(unsafe_code)]

//! Canonical PaySLO v1 model: `payslo/v1` kind `SLO`.
//! All mappings use `BTreeMap` and all sequences are emitted in stable order
//! so rendering is byte-deterministic.

use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};

/// Top-level SLO document.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct SloDocument {
    #[serde(rename = "apiVersion")]
    pub api_version: String,
    pub kind: String,
    pub name: String,
    pub service: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    /// Success objective in (0, 1), e.g. 0.995.
    pub objective: f64,
    /// Objective window, e.g. `1h`, `30d`.
    pub window: String,
    pub sli: Sli,
    pub alerting: Alerting,
    pub targets: Vec<SloTarget>,
}

/// Service-level indicator definition.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Sli {
    /// `success-ratio` or `latency` (both evaluated as good/total ratios).
    #[serde(rename = "type")]
    pub sli_type: String,
    /// SLI data source; only `metric` is supported in the lab.
    pub source: String,
    /// PromQL (or backend query) for "good" events.
    pub good: String,
    /// PromQL (or backend query) for "total" events.
    pub total: String,
}

/// Alerting policy: multi-window multi-burn-rate entries + runbook.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Alerting {
    #[serde(rename = "burnRates")]
    pub burn_rates: Vec<BurnRatePolicy>,
    /// Path to the runbook, relative to the workspace root.
    pub runbook: String,
}

/// One burn-rate alert: `longWindow`/`shortWindow` must satisfy short < long.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct BurnRatePolicy {
    #[serde(rename = "longWindow")]
    pub long_window: String,
    #[serde(rename = "shortWindow")]
    pub short_window: String,
    /// Error-budget burn multiplier, e.g. 14.4 (page) or 2 (ticket).
    #[serde(rename = "burnRate")]
    pub burn_rate: f64,
    /// `page` or `ticket`.
    pub severity: String,
}

/// Backend target with optional per-target overrides.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct SloTarget {
    /// `prometheus` or `datadog`.
    pub backend: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub overrides: Option<BTreeMap<String, String>>,
}

/// Alert normalized from any backend webhook (Alertmanager or Datadog).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct CanonicalAlert {
    pub slo: String,
    pub service: String,
    pub severity: String,
    pub status: AlertStatus,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub starts_at: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub ends_at: Option<String>,
    #[serde(default)]
    pub labels: BTreeMap<String, String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub runbook_url: Option<String>,
}

/// Firing or resolved.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum AlertStatus {
    Firing,
    Resolved,
}

/// Incident aggregated from correlated alerts (same slo+service).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Incident {
    pub id: String,
    pub title: String,
    pub slo: String,
    pub service: String,
    pub status: IncidentStatus,
    #[serde(default)]
    pub alerts: Vec<CanonicalAlert>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum IncidentStatus {
    Open,
    Acknowledged,
    Resolved,
}
