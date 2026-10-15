#![forbid(unsafe_code)]

//! Scenario definition: typed YAML (no free-form eval). `Check` is a tagged
//! enum; setup/teardown are typed action lists.

use serde::Deserialize;

/// One scenario definition (`scenarios/<id>/scenario.yml`).
#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Scenario {
    pub id: String,
    pub title: String,
    pub brief: String,
    #[serde(default)]
    pub questions: Vec<Question>,
    #[serde(default)]
    pub setup: Vec<SetupAction>,
    #[serde(default)]
    pub teardown: Vec<SetupAction>,
    #[serde(default)]
    pub checks: Vec<ScoredCheck>,
    #[serde(default)]
    pub scoring: Scoring,
    /// Unsafe trainee actions that zero the safety score when recorded.
    #[serde(default)]
    pub unsafe_actions: Vec<String>,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Question {
    pub id: String,
    pub prompt: String,
}

/// Setup/teardown step. All actions are idempotent where possible.
#[derive(Debug, Clone, Deserialize)]
#[serde(tag = "action", deny_unknown_fields)]
pub enum SetupAction {
    /// Set a bank-sim profile: {provider, profile} (+ optional knobs).
    #[serde(rename = "bank_profile")]
    BankProfile {
        provider: String,
        profile: String,
        #[serde(default)]
        error_rate: Option<f64>,
        #[serde(default)]
        latency_ms_min: Option<u64>,
        #[serde(default)]
        latency_ms_max: Option<u64>,
        #[serde(default)]
        callback_delay_s: Option<u64>,
    },
    /// Reset a bank-sim to env defaults.
    #[serde(rename = "bank_reset")]
    BankReset { provider: String },
    /// Set merchant-sink failure mode.
    #[serde(rename = "merchant_mode")]
    MerchantMode {
        status: u16,
        #[serde(default)]
        latency_ms: Option<u64>,
    },
    /// Gateway routing for one provider.
    #[serde(rename = "routing")]
    Routing { provider: String, enabled: bool },
    /// Recreate gateway with a transaction TTL (seconds).
    #[serde(rename = "gateway_ttl")]
    GatewayTtl { secs: u64 },
    /// Inject a Postgres ACCESS EXCLUSIVE lock (chaos db), auto-release secs.
    #[serde(rename = "db_lock")]
    DbLock { secs: u64 },
    /// Release any lab-held DB lock.
    #[serde(rename = "db_unlock")]
    DbUnlock {},
    /// Deploy gateway release version (good|bad).
    #[serde(rename = "deploy_gateway")]
    DeployGateway { version: String },
    /// Reset datadog-mock objects (test isolation).
    #[serde(rename = "dd_reset")]
    DdReset {},
    /// Sleep N seconds (let metrics/alerts catch up).
    #[serde(rename = "sleep")]
    Sleep { secs: u64 },
}

/// Typed check DSL.
#[derive(Debug, Clone, Deserialize)]
#[serde(tag = "type", deny_unknown_fields)]
pub enum Check {
    /// Prometheus instant query compared with operator to value.
    #[serde(rename = "prometheus_query")]
    PrometheusQuery {
        query: String,
        operator: String,
        value: f64,
    },
    /// A payslo incident row exists (any status unless stated).
    #[serde(rename = "incident_exists")]
    IncidentExists {
        #[serde(default)]
        slo: Option<String>,
        #[serde(default)]
        service: Option<String>,
    },
    /// Gateway routing state for a provider.
    #[serde(rename = "routing_state")]
    RoutingState { provider: String, enabled: bool },
    /// A submitted answer contains text (case-insensitive).
    #[serde(rename = "answer_contains")]
    AnswerContains { question: String, contains: String },
    /// SLI query value above threshold (sugar over prometheus_query).
    #[serde(rename = "sli_above")]
    SliAbove { query: String, threshold: f64 },
    /// Session duration below max (soft: display only, never fails).
    #[serde(rename = "duration_lt")]
    DurationLt { max_secs: u64 },
    /// datadog-mock object counts via a named connection.
    #[serde(rename = "datadog_objects")]
    DatadogObjects {
        connection: String,
        min_slos: u32,
        min_monitors: u32,
    },
}

/// A check with points and soft flag.
/// NOTE: no `deny_unknown_fields` here — it conflicts with `flatten` (the
/// inner `Check` still denies unknown fields per variant).
#[derive(Debug, Clone, Deserialize)]
pub struct ScoredCheck {
    #[serde(flatten)]
    pub check: Check,
    #[serde(default = "default_points")]
    pub points: u32,
    #[serde(default)]
    pub soft: bool,
}

fn default_points() -> u32 {
    10
}

/// Scoring windows (seconds).
#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Scoring {
    #[serde(default = "default_ttd_full")]
    pub ttd_full_secs: u64,
    #[serde(default = "default_ttd_zero")]
    pub ttd_zero_secs: u64,
    #[serde(default = "default_ttm_full")]
    pub ttm_full_secs: u64,
    #[serde(default = "default_ttm_zero")]
    pub ttm_zero_secs: u64,
}

impl Default for Scoring {
    fn default() -> Self {
        Self {
            ttd_full_secs: default_ttd_full(),
            ttd_zero_secs: default_ttd_zero(),
            ttm_full_secs: default_ttm_full(),
            ttm_zero_secs: default_ttm_zero(),
        }
    }
}

fn default_ttd_full() -> u64 {
    120
}
fn default_ttd_zero() -> u64 {
    600
}
fn default_ttm_full() -> u64 {
    300
}
fn default_ttm_zero() -> u64 {
    1500
}

/// Load + basic-validate a scenario file.
pub fn load(path: &std::path::Path) -> anyhow::Result<Scenario> {
    let text = std::fs::read_to_string(path)
        .map_err(|e| anyhow::anyhow!("cannot read {}: {e}", path.display()))?;
    let scenario: Scenario = serde_yaml_ng::from_str(&text)
        .map_err(|e| anyhow::anyhow!("{}: invalid scenario YAML: {e}", path.display()))?;
    let stem = path
        .parent()
        .and_then(|p| p.file_name())
        .and_then(|n| n.to_str())
        .unwrap_or("");
    if scenario.id != stem {
        anyhow::bail!(
            "{}: id {:?} must match directory name {stem:?}",
            path.display(),
            scenario.id
        );
    }
    Ok(scenario)
}

/// Human-readable check description (for verify output).
pub fn describe(check: &Check) -> String {
    match check {
        Check::PrometheusQuery {
            query,
            operator,
            value,
        } => {
            format!("prometheus: ({query}) {operator} {value}")
        }
        Check::IncidentExists { slo, service } => format!(
            "incident exists slo={} service={}",
            slo.as_deref().unwrap_or("*"),
            service.as_deref().unwrap_or("*")
        ),
        Check::RoutingState { provider, enabled } => {
            format!("routing {provider} enabled={enabled}")
        }
        Check::AnswerContains { question, contains } => {
            format!("answer[{question}] contains {contains:?}")
        }
        Check::SliAbove { query, threshold } => format!("sli ({query}) > {threshold}"),
        Check::DurationLt { max_secs } => format!("duration < {max_secs}s (soft)"),
        Check::DatadogObjects {
            connection,
            min_slos,
            min_monitors,
        } => {
            format!("datadog-mock[{connection}] slos>={min_slos} monitors>={min_monitors}")
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const SAMPLE: &str = r#"
id: "99"
title: Sample
brief: Briefing.
questions:
  - id: diagnosis
    prompt: What broke?
setup:
  - action: bank_profile
    provider: bca
    profile: down
  - action: sleep
    secs: 5
checks:
  - type: incident_exists
    slo: payment-success-rate
    points: 10
  - type: routing_state
    provider: bca
    enabled: false
    points: 15
  - type: prometheus_query
    query: sum(rate(x[1m]))
    operator: ">"
    value: 1.0
    points: 10
  - type: answer_contains
    question: diagnosis
    contains: bca
    points: 25
  - type: sli_above
    query: sum(rate(y[1m]))
    threshold: 0.9
    points: 10
  - type: duration_lt
    max_secs: 900
    soft: true
scoring:
  ttd_full_secs: 60
unsafe_actions: [chaos, deploy-bad]
"#;

    #[test]
    fn parses_all_check_types() -> Result<(), String> {
        let scenario: Scenario = serde_yaml_ng::from_str(SAMPLE).map_err(|e| e.to_string())?;
        assert_eq!(scenario.checks.len(), 6);
        assert!(matches!(
            scenario.checks[0].check,
            Check::IncidentExists { .. }
        ));
        assert!(matches!(
            scenario.checks[1].check,
            Check::RoutingState { .. }
        ));
        assert!(matches!(
            scenario.checks[2].check,
            Check::PrometheusQuery { .. }
        ));
        assert!(matches!(
            scenario.checks[3].check,
            Check::AnswerContains { .. }
        ));
        assert!(matches!(scenario.checks[4].check, Check::SliAbove { .. }));
        assert!(matches!(scenario.checks[5].check, Check::DurationLt { .. }));
        assert_eq!(scenario.setup.len(), 2);
        assert_eq!(scenario.scoring.ttd_full_secs, 60);
        Ok(())
    }

    #[test]
    fn rejects_unknown_check_type() {
        let bad = SAMPLE.replace("type: incident_exists", "type: teleport");
        assert!(serde_yaml_ng::from_str::<Scenario>(&bad).is_err());
    }

    #[test]
    fn rejects_unknown_action() {
        let bad = SAMPLE.replace("action: bank_profile", "action: rm_rf");
        assert!(serde_yaml_ng::from_str::<Scenario>(&bad).is_err());
    }

    #[test]
    fn parses_new_action_and_check_types() -> Result<(), String> {
        let doc = r#"
id: "98"
title: T
brief: B
setup:
  - action: deploy_gateway
    version: bad
  - action: dd_reset
checks:
  - type: datadog_objects
    connection: datadog-mock
    min_slos: 3
    min_monitors: 1
"#;
        let scenario: Scenario = serde_yaml_ng::from_str(doc).map_err(|e| e.to_string())?;
        assert!(matches!(
            scenario.setup[0],
            SetupAction::DeployGateway { .. }
        ));
        assert!(matches!(scenario.setup[1], SetupAction::DdReset {}));
        assert!(matches!(
            scenario.checks[0].check,
            Check::DatadogObjects { .. }
        ));
        Ok(())
    }
}
