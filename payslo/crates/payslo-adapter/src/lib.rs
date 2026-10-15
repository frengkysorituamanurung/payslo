#![forbid(unsafe_code)]

//! Shared SLO backend adapter trait + error type + state store.
//! (`async_trait` is used because native `async fn` in traits is not
//! object-safe for `dyn` — see docs/DECISIONS.md.)

pub mod connections;
pub mod store;

use payslo_model::SloDocument;
use thiserror::Error;

/// Backend-agnostic adapter error.
#[derive(Debug, Error)]
pub enum AdapterError {
    #[error("validation failed: {0}")]
    Invalid(String),
    #[error("render failed: {0}")]
    Render(String),
    #[error("backend call failed: {0}")]
    Backend(String),
    #[error("model error: {0}")]
    Model(#[from] payslo_model::ModelError),
}

/// One file the adapter wants to write.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PlannedFile {
    /// File name (deterministic, e.g. `<slo>.yml`).
    pub name: String,
    /// Full file content.
    pub content: String,
}

/// Capabilities advertised by a backend.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Capabilities {
    pub backend: String,
    pub supports_recording_rules: bool,
    pub supports_alerts: bool,
    pub supports_drift_check: bool,
}

/// One row of the backend capability matrix (shared by docs + CLI output).
pub struct MatrixRow {
    pub feature: &'static str,
    pub prometheus: &'static str,
    pub datadog: &'static str,
}

/// Backend capability matrix. Single source for `payslo capabilities` and
/// `payslo/CAPABILITIES.md` — keep them in sync when editing.
pub const CAPABILITY_MATRIX: &[MatrixRow] = &[
    MatrixRow {
        feature: "recording rules",
        prometheus: "yes (PromQL)",
        datadog: "no (Datadog evaluates SLI itself)",
    },
    MatrixRow {
        feature: "burn-rate alerts",
        prometheus: "yes (MWMB, any window ≥1m)",
        datadog: "yes (error_budget monitors, timeframe 7d/30d/90d only)",
    },
    MatrixRow {
        feature: "objective windows",
        prometheus: "1m..90d",
        datadog: "7d/30d/90d only (lab windows mapped to 7d + warning)",
    },
    MatrixRow {
        feature: "SLI source",
        prometheus: "metric (PromQL, native)",
        datadog: "metric (Datadog queries; PromQL stored opaquely, not evaluated)",
    },
    MatrixRow {
        feature: "drift check",
        prometheus: "yes (file hash)",
        datadog: "yes (API content hash)",
    },
    MatrixRow {
        feature: "atomic apply",
        prometheus: "yes (temp + rename + reload)",
        datadog: "yes (SLO first, then monitors; retry 429/5xx)",
    },
    MatrixRow {
        feature: "runbook link",
        prometheus: "yes (runbook_url annotation)",
        datadog: "yes (message + tag)",
    },
];

/// SLO backend trait (async, object-safe via `async_trait`).
#[async_trait::async_trait]
pub trait SloBackend: Send + Sync {
    /// Backend name (`prometheus`, `datadog`).
    fn name(&self) -> &'static str;
    fn capabilities(&self) -> Capabilities;
    /// Backend-specific validation (on top of model validation).
    fn validate(&self, slo: &SloDocument) -> Result<(), AdapterError>;
    /// Render one SLO into files (deterministic output).
    fn render(&self, slo: &SloDocument) -> Result<Vec<PlannedFile>, AdapterError>;
    /// Files that would change vs `existing` (name -> new content).
    fn plan(
        &self,
        slo: &SloDocument,
        existing: &std::collections::BTreeMap<String, String>,
    ) -> Result<Vec<PlannedFile>, AdapterError>;
    /// Write files to `out_dir` and notify the backend (e.g. reload).
    async fn apply(&self, slo: &SloDocument, out_dir: &std::path::Path)
    -> Result<(), AdapterError>;
    /// Remove this SLO's files from `out_dir`.
    async fn delete(
        &self,
        slo: &SloDocument,
        out_dir: &std::path::Path,
    ) -> Result<(), AdapterError>;
    /// Backend health relevant to this SLO (human-readable lines).
    async fn status(&self, slo: &SloDocument) -> Result<Vec<String>, AdapterError>;

    /// Non-fatal backend limitations for this SLO. Printed by the CLI as
    /// `warning:` lines — backends must NEVER silently drop features.
    fn warnings(&self, _slo: &SloDocument) -> Vec<String> {
        Vec::new()
    }
}
