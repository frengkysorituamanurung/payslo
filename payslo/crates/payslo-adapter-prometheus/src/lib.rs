#![forbid(unsafe_code)]

//! Prometheus backend: Google-SRE multi-window multi-burn-rate alerts +
//! error-ratio recording rules. Output is byte-deterministic: groups sorted by
//! name, recording rules sorted by record name, alerts in document order,
//! explicit field order, block scalars for PromQL (no HashMap anywhere).

use std::collections::{BTreeMap, BTreeSet};

use payslo_adapter::{AdapterError, Capabilities, PlannedFile, SloBackend};
use payslo_model::{SloDocument, parse_duration};

/// Prometheus backend. `prometheus_url` (e.g. `http://127.0.0.1:9090`) enables
/// `apply` reloads and `status` rule inspection; `None` = render-only.
pub struct PrometheusBackend {
    pub prometheus_url: Option<String>,
}

impl PrometheusBackend {
    #[must_use]
    pub fn new(prometheus_url: Option<String>) -> Self {
        Self { prometheus_url }
    }

    fn file_name(slo: &SloDocument) -> String {
        format!("{}.yml", slo.name)
    }
}

/// Format a threshold with at most 10 decimals, trimmed (deterministic).
fn fmt_num(v: f64) -> String {
    let s = format!("{v:.10}");
    let s = s.trim_end_matches('0').trim_end_matches('.');
    if s.is_empty() {
        "0".to_owned()
    } else {
        s.to_owned()
    }
}

fn error_budget(objective: f64) -> f64 {
    1.0 - objective
}

/// Metric-name-safe SLO slug: PromQL identifiers forbid `-`.
fn prom_slug(name: &str) -> String {
    name.replace('-', "_")
}

/// Indent every line of `text` by `n` spaces (for YAML block scalars).
fn indent(text: &str, n: usize) -> String {
    let pad = " ".repeat(n);
    text.lines()
        .map(|line| {
            if line.trim().is_empty() {
                String::new()
            } else {
                format!("{pad}{line}")
            }
        })
        .collect::<Vec<_>>()
        .join("\n")
}

struct Compiled {
    group_name: String,
    recordings: Vec<(String, String)>,
    alerts: Vec<AlertRule>,
}

struct AlertRule {
    severity: String,
    long_window: String,
    short_window: String,
    threshold: String,
    error_expr_long: String,
    error_expr_short: String,
}

fn compile(slo: &SloDocument) -> Result<Compiled, AdapterError> {
    let eb = error_budget(slo.objective);
    let mut windows: BTreeSet<String> = BTreeSet::new();
    for burn in &slo.alerting.burn_rates {
        if parse_duration(&burn.long_window).is_none() {
            return Err(AdapterError::Invalid(format!(
                "bad longWindow {:?}",
                burn.long_window
            )));
        }
        if parse_duration(&burn.short_window).is_none() {
            return Err(AdapterError::Invalid(format!(
                "bad shortWindow {:?}",
                burn.short_window
            )));
        }
        windows.insert(burn.long_window.clone());
        windows.insert(burn.short_window.clone());
    }
    let mut recordings = Vec::new();
    for window in &windows {
        let record = format!("payslo:slo_error_ratio:{}:{window}", prom_slug(&slo.name));
        let expr = format!(
            "1 - (\n  sum(rate({good}[{window}]))\n  /\n  sum(rate({total}[{window}]))\n)",
            good = slo.sli.good,
            total = slo.sli.total,
        );
        recordings.push((record, expr));
    }
    let mut alerts = Vec::new();
    for burn in &slo.alerting.burn_rates {
        let threshold = fmt_num(eb * burn.burn_rate);
        let slug = prom_slug(&slo.name);
        alerts.push(AlertRule {
            severity: burn.severity.clone(),
            long_window: burn.long_window.clone(),
            short_window: burn.short_window.clone(),
            threshold,
            error_expr_long: format!("payslo:slo_error_ratio:{slug}:{}", burn.long_window),
            error_expr_short: format!("payslo:slo_error_ratio:{slug}:{}", burn.short_window),
        });
    }
    Ok(Compiled {
        group_name: format!("payslo-{}", slo.name),
        recordings,
        alerts,
    })
}

/// Native Prometheus rules file (for `rule_files:`).
fn render_native(slo: &SloDocument, compiled: &Compiled) -> String {
    let mut out = String::from("groups:\n");
    out.push_str(&format!("  - name: {}\n", compiled.group_name));
    out.push_str("    rules:\n");
    for (record, expr) in &compiled.recordings {
        out.push_str(&format!("      - record: {record}\n"));
        out.push_str("        expr: |\n");
        out.push_str(&indent(expr, 10));
        out.push('\n');
    }
    for alert in &compiled.alerts {
        let expr = format!(
            "(\n  {long} > {threshold}\n  and\n  {short} > {threshold}\n)",
            long = alert.error_expr_long,
            short = alert.error_expr_short,
            threshold = alert.threshold,
        );
        out.push_str("      - alert: PaySLOBurnRateHigh\n");
        out.push_str("        expr: |\n");
        out.push_str(&indent(&expr, 10));
        out.push('\n');
        out.push_str(&format!("        for: {}\n", alert.short_window));
        out.push_str("        labels:\n");
        out.push_str(&format!("          slo: {}\n", slo.name));
        out.push_str(&format!("          service: {}\n", slo.service));
        out.push_str(&format!("          severity: {}\n", alert.severity));
        // Distinguishes the fast-burn and slow-burn alerts sharing
        // slo/service/severity (else Prometheus sees duplicate alerts).
        out.push_str(&format!("          window: \"{}\"\n", alert.long_window));
        out.push_str("        annotations:\n");
        out.push_str(&format!(
            "          summary: \"SLO {} burn rate high ({} over {})\"\n",
            slo.name, alert.long_window, alert.short_window
        ));
        out.push_str(&format!(
            "          runbook_url: \"{}\"\n",
            slo.alerting.runbook
        ));
        out.push_str(&format!("          window: \"{}\"\n", alert.long_window));
    }
    out
}

/// PrometheusRule CRD (monitoring.coreos.com/v1) for GitOps pipelines.
fn render_crd(slo: &SloDocument, compiled: &Compiled) -> String {
    let mut out = String::from("apiVersion: monitoring.coreos.com/v1\n");
    out.push_str("kind: PrometheusRule\n");
    out.push_str("metadata:\n");
    out.push_str(&format!("  name: {}\n", compiled.group_name));
    out.push_str("  labels:\n");
    out.push_str("    app.kubernetes.io/managed-by: payslo\n");
    out.push_str(&format!("    payslo-slo: {}\n", slo.name));
    out.push_str("spec:\n");
    out.push_str(&format!("  - name: {}\n", compiled.group_name));
    out.push_str("    rules:\n");
    for (record, expr) in &compiled.recordings {
        out.push_str(&format!("      - record: {record}\n"));
        out.push_str("        expr: |\n");
        out.push_str(&indent(expr, 10));
        out.push('\n');
    }
    for alert in &compiled.alerts {
        let expr = format!(
            "(\n  {long} > {threshold}\n  and\n  {short} > {threshold}\n)",
            long = alert.error_expr_long,
            short = alert.error_expr_short,
            threshold = alert.threshold,
        );
        out.push_str("      - alert: PaySLOBurnRateHigh\n");
        out.push_str("        expr: |\n");
        out.push_str(&indent(&expr, 10));
        out.push('\n');
        out.push_str(&format!("        for: {}\n", alert.short_window));
        out.push_str("        labels:\n");
        out.push_str(&format!("          slo: {}\n", slo.name));
        out.push_str(&format!("          service: {}\n", slo.service));
        out.push_str(&format!("          severity: {}\n", alert.severity));
        // Distinguishes the fast-burn and slow-burn alerts sharing
        // slo/service/severity (else Prometheus sees duplicate alerts).
        out.push_str(&format!("          window: \"{}\"\n", alert.long_window));
        out.push_str("        annotations:\n");
        out.push_str(&format!(
            "          summary: \"SLO {} burn rate high ({} over {})\"\n",
            slo.name, alert.long_window, alert.short_window
        ));
        out.push_str(&format!(
            "          runbook_url: \"{}\"\n",
            slo.alerting.runbook
        ));
    }
    out
}

impl PrometheusBackend {
    /// Render native rules file content (used by CLI `--format native`).
    pub fn render_native_text(&self, slo: &SloDocument) -> Result<String, AdapterError> {
        self.validate(slo)?;
        Ok(render_native(slo, &compile(slo)?))
    }

    /// Render PrometheusRule CRD content (used by CLI `--format crd`).
    pub fn render_crd_text(&self, slo: &SloDocument) -> Result<String, AdapterError> {
        self.validate(slo)?;
        Ok(render_crd(slo, &compile(slo)?))
    }
}

#[async_trait::async_trait]
impl SloBackend for PrometheusBackend {
    fn name(&self) -> &'static str {
        "prometheus"
    }

    fn capabilities(&self) -> Capabilities {
        Capabilities {
            backend: "prometheus".to_owned(),
            supports_recording_rules: true,
            supports_alerts: true,
            supports_drift_check: true,
        }
    }

    fn validate(&self, slo: &SloDocument) -> Result<(), AdapterError> {
        if slo.sli.good.trim().is_empty() || slo.sli.total.trim().is_empty() {
            return Err(AdapterError::Invalid(
                "sli.good and sli.total must be non-empty".to_owned(),
            ));
        }
        if parse_duration(&slo.window).is_none() {
            return Err(AdapterError::Invalid(format!(
                "bad window {:?}",
                slo.window
            )));
        }
        Ok(())
    }

    fn render(&self, slo: &SloDocument) -> Result<Vec<PlannedFile>, AdapterError> {
        Ok(vec![PlannedFile {
            name: Self::file_name(slo),
            content: self.render_native_text(slo)?,
        }])
    }

    fn plan(
        &self,
        slo: &SloDocument,
        existing: &BTreeMap<String, String>,
    ) -> Result<Vec<PlannedFile>, AdapterError> {
        let wanted = self.render(slo)?;
        Ok(wanted
            .into_iter()
            .filter(|f| existing.get(&f.name).is_none_or(|c| c != &f.content))
            .collect())
    }

    async fn apply(
        &self,
        slo: &SloDocument,
        out_dir: &std::path::Path,
    ) -> Result<(), AdapterError> {
        let files = self.render(slo)?;
        let mut changed = false;
        for file in &files {
            let path = out_dir.join(&file.name);
            let current = std::fs::read_to_string(&path).unwrap_or_default();
            if current == file.content {
                continue;
            }
            // Atomic write: temp file in the same directory, then rename, so
            // Prometheus never reads a half-written rules file.
            let tmp = path.with_extension("yml.tmp");
            std::fs::write(&tmp, &file.content).map_err(|e| {
                AdapterError::Backend(format!("cannot write {}: {e}", tmp.display()))
            })?;
            std::fs::rename(&tmp, &path).map_err(|e| {
                AdapterError::Backend(format!("cannot rename {}: {e}", tmp.display()))
            })?;
            changed = true;
        }
        // Idempotent: skip the reload when nothing changed.
        if changed && let Some(base) = self.prometheus_url.as_deref() {
            let url = format!("{base}/-/reload");
            reqwest::Client::new()
                .post(&url)
                .send()
                .await
                .map_err(|e| AdapterError::Backend(format!("reload {url}: {e}")))?;
        }
        Ok(())
    }

    async fn delete(
        &self,
        slo: &SloDocument,
        out_dir: &std::path::Path,
    ) -> Result<(), AdapterError> {
        let path = out_dir.join(Self::file_name(slo));
        if path.exists() {
            std::fs::remove_file(&path).map_err(|e| {
                AdapterError::Backend(format!("cannot remove {}: {e}", path.display()))
            })?;
        }
        Ok(())
    }

    async fn status(&self, slo: &SloDocument) -> Result<Vec<String>, AdapterError> {
        let Some(base) = self.prometheus_url.as_deref() else {
            return Ok(vec!["prometheus_url unset: render-only mode".to_owned()]);
        };
        let url = format!("{base}/api/v1/rules");
        let body: serde_json::Value = reqwest::Client::new()
            .get(&url)
            .send()
            .await
            .map_err(|e| AdapterError::Backend(e.to_string()))?
            .json()
            .await
            .map_err(|e| AdapterError::Backend(e.to_string()))?;
        let mut lines = Vec::new();
        let groups = body
            .pointer("/data/groups")
            .and_then(serde_json::Value::as_array)
            .cloned()
            .unwrap_or_default();
        for group in &groups {
            if group.pointer("/name").and_then(serde_json::Value::as_str)
                == Some(format!("payslo-{}", slo.name).as_str())
            {
                let rules = group
                    .pointer("/rules")
                    .and_then(serde_json::Value::as_array)
                    .cloned()
                    .unwrap_or_default();
                lines.push(format!("group payslo-{}: {} rules", slo.name, rules.len()));
                for rule in &rules {
                    if let (Some(name), Some(state)) = (
                        rule.pointer("/name").and_then(serde_json::Value::as_str),
                        rule.pointer("/state").and_then(serde_json::Value::as_str),
                    ) {
                        lines.push(format!("  {name}: {state}"));
                    }
                }
            }
        }
        if lines.is_empty() {
            lines.push(format!("group payslo-{} not loaded", slo.name));
        }
        Ok(lines)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn thresholds_format_cleanly() {
        assert_eq!(fmt_num(0.005 * 14.4), "0.072");
        assert_eq!(fmt_num(0.005 * 6.0), "0.03");
        assert_eq!(fmt_num(0.001 * 2.0), "0.002");
        assert_eq!(fmt_num(0.0), "0");
    }
}
