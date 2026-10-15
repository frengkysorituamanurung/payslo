#![forbid(unsafe_code)]

//! Datadog backend: metric SLOs + `slo alert` burn-rate monitors.
//! Field shapes follow the official Datadog v1 API docs (endpoints,
//! `DD-API-KEY` auth, SLO `type/metric/thresholds/timeframe`, monitor
//! `type/query/name/message/tags/options`) — see docs/ASSUMPTIONS.md for the
//! verified list. The mock stores SLI queries opaquely (it does not evaluate
//! Datadog metric queries). Order: SLO first, then monitors.

use std::collections::BTreeMap;
use std::time::Duration;

use payslo_adapter::{AdapterError, Capabilities, PlannedFile, SloBackend};
use payslo_model::{SloDocument, parse_duration};
use rand::RngExt;

/// Placeholder used in rendered payloads where the server-generated SLO id
/// goes. Substituted with the real id on apply.
pub const SLO_ID_PLACEHOLDER: &str = "__PAYSLO_SLO_ID__";

/// Tag marking every object PaySLO manages.
pub const MANAGED_TAG: &str = "managed-by:payslo";

/// Datadog connection config (built by the CLI from connections.yml).
#[derive(Debug, Clone)]
pub struct DatadogConfig {
    pub endpoint: String,
    pub api_key: String,
}

/// Datadog backend (HTTP to Datadog or datadog-mock).
#[derive(Debug, Clone)]
pub struct DatadogBackend {
    config: DatadogConfig,
    http: reqwest::Client,
}

impl DatadogBackend {
    #[must_use]
    pub fn new(config: DatadogConfig) -> Self {
        Self {
            config,
            http: reqwest::Client::new(),
        }
    }

    /// POST/PUT/GET with exponential backoff + jitter on 429/5xx.
    /// Auth (`DD-API-KEY`) is attached by each call site because request
    /// builders are consumed on send and rebuilt per attempt.
    async fn send_retry(
        &self,
        build: impl Fn() -> reqwest::RequestBuilder,
    ) -> Result<reqwest::Response, AdapterError> {
        let mut attempt: u32 = 0;
        loop {
            let resp = build()
                .send()
                .await
                .map_err(|e| AdapterError::Backend(e.to_string()))?;
            let status = resp.status();
            if status.as_u16() == 429 || status.is_server_error() {
                attempt += 1;
                if attempt > 4 {
                    return Err(AdapterError::Backend(format!(
                        "datadog kept failing with {status} after retries"
                    )));
                }
                let backoff_ms = 200_u64.saturating_mul(2_u64.saturating_pow(attempt));
                let jitter: u64 = rand::rng().random_range(0..100);
                tokio::time::sleep(Duration::from_millis(backoff_ms + jitter)).await;
                continue;
            }
            if !status.is_success() {
                let body = resp.text().await.unwrap_or_default();
                return Err(AdapterError::Backend(format!(
                    "datadog {status}: {body:.300}"
                )));
            }
            return Ok(resp);
        }
    }

    async fn get_json(&self, path: &str) -> Result<serde_json::Value, AdapterError> {
        let url = format!("{}{path}", self.config.endpoint);
        let http = self.http.clone();
        let key = self.config.api_key.clone();
        self.send_retry(move || http.get(&url).header("DD-API-KEY", &key))
            .await?
            .json()
            .await
            .map_err(|e| AdapterError::Backend(e.to_string()))
    }

    async fn post_json(
        &self,
        path: &str,
        body: &serde_json::Value,
    ) -> Result<serde_json::Value, AdapterError> {
        let url = format!("{}{path}", self.config.endpoint);
        let http = self.http.clone();
        let key = self.config.api_key.clone();
        let body = body.clone();
        self.send_retry(move || http.post(&url).header("DD-API-KEY", &key).json(&body))
            .await?
            .json()
            .await
            .map_err(|e| AdapterError::Backend(e.to_string()))
    }

    async fn put_json(
        &self,
        path: &str,
        body: &serde_json::Value,
    ) -> Result<serde_json::Value, AdapterError> {
        let url = format!("{}{path}", self.config.endpoint);
        let http = self.http.clone();
        let key = self.config.api_key.clone();
        let body = body.clone();
        self.send_retry(move || http.put(&url).header("DD-API-KEY", &key).json(&body))
            .await?
            .json()
            .await
            .map_err(|e| AdapterError::Backend(e.to_string()))
    }

    async fn delete(&self, path: &str) -> Result<(), AdapterError> {
        let url = format!("{}{path}", self.config.endpoint);
        let http = self.http.clone();
        let key = self.config.api_key.clone();
        self.send_retry(move || http.delete(&url).header("DD-API-KEY", &key))
            .await?;
        Ok(())
    }
}

/// Map a lab window to the closest Datadog SLO timeframe (7d/30d/90d).
/// Returns (timeframe, mapped: whether it differs from requested).
fn map_timeframe(window: &str) -> (String, bool) {
    let secs = parse_duration(window).unwrap_or(7 * 86_400);
    let tf = if secs <= 7 * 86_400 {
        "7d"
    } else if secs <= 30 * 86_400 {
        "30d"
    } else {
        "90d"
    };
    (tf.to_owned(), tf != window)
}

/// Build SLO + monitor payloads with `slo_id` substituted.
/// Monitors are sorted by name so render/fetch/delete all see one stable order.
fn build_payloads(slo: &SloDocument, slo_id: &str) -> (serde_json::Value, Vec<serde_json::Value>) {
    let target_pct = (slo.objective * 100.0 * 10000.0).round() / 10000.0;
    let warning_pct = ((target_pct + (100.0 - target_pct) / 2.0) * 10000.0).round() / 10000.0;
    let (timeframe, _) = map_timeframe(&slo.window);
    let slo_payload = serde_json::json!({
        "type": "metric",
        "name": slo.name,
        "description": slo.description.clone().unwrap_or_default(),
        "tags": ["managed-by:payslo", format!("service:{}", slo.service)],
        "thresholds": [{
            "target": target_pct,
            "timeframe": timeframe,
            "warning": warning_pct,
        }],
        "timeframe": timeframe,
        "target_threshold": target_pct,
        "warning_threshold": warning_pct,
        "query": {
            "numerator": slo.sli.good,
            "denominator": slo.sli.total,
        },
    });
    let mut monitors = Vec::new();
    for burn in &slo.alerting.burn_rates {
        let (mon_tf, _) = map_timeframe(&slo.window);
        monitors.push(serde_json::json!({
            "type": "slo alert",
            "name": format!("[payslo] {} {} {}/{}", slo.name, burn.severity, burn.long_window, burn.short_window),
            "query": format!("error_budget(\"{slo_id}\").over(\"{mon_tf}\") >= {}", burn.burn_rate),
            "message": format!("SLO {} burn rate high. Runbook: {}", slo.name, slo.alerting.runbook),
            "tags": ["managed-by:payslo", format!("slo:{}", slo.name), format!("service:{}", slo.service), format!("severity:{}", burn.severity), format!("window:{}", burn.long_window)],
            "options": {
                "thresholds": {"critical": burn.burn_rate},
                "include_tags": true,
            },
        }));
    }
    monitors.sort_by(|a, b| {
        let an = a
            .pointer("/name")
            .and_then(serde_json::Value::as_str)
            .unwrap_or("");
        let bn = b
            .pointer("/name")
            .and_then(serde_json::Value::as_str)
            .unwrap_or("");
        an.cmp(bn)
    });
    (slo_payload, monitors)
}

/// Canonical JSON for drift comparison (server fields stripped, keys sorted).
fn canonical(payloads: &(serde_json::Value, Vec<serde_json::Value>)) -> String {
    let doc = serde_json::json!({"slo": payloads.0, "monitors": payloads.1});
    serde_json::to_string(&doc).unwrap_or_default()
}

impl DatadogBackend {
    /// Rendered (placeholder) content for plan files.
    pub fn render_canonical(&self, slo: &SloDocument) -> Result<String, AdapterError> {
        self.validate(slo)?;
        Ok(canonical(&build_payloads(slo, SLO_ID_PLACEHOLDER)))
    }

    /// Canonical content with the real SLO id substituted.
    pub fn resolved_canonical(
        &self,
        slo: &SloDocument,
        slo_id: &str,
    ) -> Result<String, AdapterError> {
        self.validate(slo)?;
        Ok(canonical(&build_payloads(slo, slo_id)))
    }

    /// Fetch live objects, canonicalized with REAL ids (for CLI plan/drift
    /// against resolved content). Returns None when the SLO is absent.
    /// Use [`DatadogBackend::fetch_canonical`] for the placeholder form.
    pub async fn fetch_resolved(
        &self,
        slo: &SloDocument,
        external_id: Option<&str>,
    ) -> Result<Option<String>, AdapterError> {
        let (live_id, live_slo, live_monitors) = match self.fetch_live(slo, external_id).await? {
            Some(t) => t,
            None => return Ok(None),
        };
        let _ = live_id;
        Ok(Some(canonical(&(live_slo, live_monitors))))
    }

    /// Fetch live objects and canonicalize (ids normalized to placeholder).
    /// Returns None when the SLO does not exist remotely.
    pub async fn fetch_canonical(
        &self,
        slo: &SloDocument,
        external_id: Option<&str>,
    ) -> Result<Option<String>, AdapterError> {
        let (live_id, live_slo, mut live_monitors) = match self.fetch_live(slo, external_id).await?
        {
            Some(t) => t,
            None => return Ok(None),
        };
        for mon in live_monitors.iter_mut() {
            for key in [
                "id",
                "created",
                "modified",
                "creator",
                "overall_state",
                "matching_downtimes",
            ] {
                mon.as_object_mut().map(|m| m.remove(key));
            }
        }
        let mut slo_value = live_slo;
        for key in [
            "id",
            "created_at",
            "modified_at",
            "creator",
            "monitor_ids",
            "monitor_tags",
        ] {
            slo_value.as_object_mut().map(|m| m.remove(key));
        }
        // Normalize the concrete SLO id back to the placeholder.
        let mut text = serde_json::to_string(
            &serde_json::json!({"slo": slo_value, "monitors": live_monitors}),
        )
        .unwrap_or_default();
        if !live_id.is_empty() {
            text = text.replace(&live_id, SLO_ID_PLACEHOLDER);
        }
        Ok(Some(text))
    }

    /// Raw live fetch: (slo_id, slo stripped of server fields, monitors
    /// stripped of server fields, unsorted).
    async fn fetch_live(
        &self,
        slo: &SloDocument,
        external_id: Option<&str>,
    ) -> Result<Option<(String, serde_json::Value, Vec<serde_json::Value>)>, AdapterError> {
        let live_slo = match external_id {
            Some(id) => {
                let resp = self.get_json(&format!("/api/v1/slo/{id}")).await;
                match resp {
                    Ok(v) => v.pointer("/data").cloned(),
                    Err(_) => None,
                }
            }
            None => self.find_slo_by_name(&slo.name).await?,
        };
        let Some(mut live_slo) = live_slo else {
            return Ok(None);
        };
        let live_id = live_slo
            .pointer("/id")
            .and_then(serde_json::Value::as_str)
            .unwrap_or("")
            .to_owned();
        // Strip server-managed fields.
        for key in [
            "id",
            "created_at",
            "modified_at",
            "creator",
            "monitor_ids",
            "monitor_tags",
        ] {
            live_slo.as_object_mut().map(|m| m.remove(key));
        }
        let live_monitors = self.list_monitors_for_slo(&slo.name).await?;
        let mut normalized_monitors = Vec::new();
        for mut mon in live_monitors {
            for key in [
                "id",
                "created",
                "modified",
                "creator",
                "overall_state",
                "matching_downtimes",
            ] {
                mon.as_object_mut().map(|m| m.remove(key));
            }
            normalized_monitors.push(mon);
        }
        normalized_monitors.sort_by(|a: &serde_json::Value, b: &serde_json::Value| {
            let an = a
                .pointer("/name")
                .and_then(serde_json::Value::as_str)
                .unwrap_or("");
            let bn = b
                .pointer("/name")
                .and_then(serde_json::Value::as_str)
                .unwrap_or("");
            an.cmp(bn)
        });
        Ok(Some((live_id, live_slo, normalized_monitors)))
    }

    async fn find_slo_by_name(
        &self,
        name: &str,
    ) -> Result<Option<serde_json::Value>, AdapterError> {
        let list = self.get_json("/api/v1/slo?tags=managed-by:payslo").await?;
        let items = list
            .pointer("/data")
            .and_then(serde_json::Value::as_array)
            .cloned()
            .unwrap_or_default();
        for item in items {
            if item.pointer("/name").and_then(serde_json::Value::as_str) == Some(name) {
                return Ok(Some(item));
            }
        }
        Ok(None)
    }

    async fn list_monitors_for_slo(
        &self,
        slo_name: &str,
    ) -> Result<Vec<serde_json::Value>, AdapterError> {
        let list = self
            .get_json("/api/v1/monitor?tags=managed-by:payslo")
            .await?;
        let items = list.as_array().cloned().unwrap_or_default();
        let prefix = format!("[payslo] {slo_name} ");
        Ok(items
            .into_iter()
            .filter(|m| {
                m.pointer("/name")
                    .and_then(serde_json::Value::as_str)
                    .is_some_and(|n| n.starts_with(&prefix))
            })
            .collect())
    }

    /// Create-or-update SLO (by name+tag), then monitors in order.
    /// Returns (slo_id, resolved canonical content).
    pub async fn apply_remote(
        &self,
        slo: &SloDocument,
        external_id: Option<&str>,
    ) -> Result<(String, String), AdapterError> {
        self.validate(slo)?;
        let (slo_payload, _) = build_payloads(slo, SLO_ID_PLACEHOLDER);
        // Resolve existing SLO: stored id first (verified by name — a stale
        // id must never overwrite a different object), else name lookup.
        let existing = match external_id {
            Some(id) => {
                let by_id = match self.get_json(&format!("/api/v1/slo/{id}")).await {
                    Ok(v) => v.pointer("/data").cloned(),
                    Err(_) => None,
                };
                match by_id {
                    Some(obj)
                        if obj.pointer("/name").and_then(serde_json::Value::as_str)
                            == Some(slo.name.as_str()) =>
                    {
                        Some(obj)
                    }
                    _ => self.find_slo_by_name(&slo.name).await?,
                }
            }
            None => self.find_slo_by_name(&slo.name).await?,
        };
        let slo_id = match existing {
            Some(obj) => {
                let id = obj
                    .pointer("/id")
                    .and_then(serde_json::Value::as_str)
                    .unwrap_or("")
                    .to_owned();
                if id.is_empty() {
                    return Err(AdapterError::Backend("remote SLO has no id".to_owned()));
                }
                self.put_json(&format!("/api/v1/slo/{id}"), &slo_payload)
                    .await?;
                id
            }
            None => {
                let created = self.post_json("/api/v1/slo", &slo_payload).await?;
                created
                    .pointer("/data/0/id")
                    .and_then(serde_json::Value::as_str)
                    .unwrap_or("")
                    .to_owned()
            }
        };
        if slo_id.is_empty() {
            return Err(AdapterError::Backend(
                "datadog did not return an SLO id".to_owned(),
            ));
        }
        // Monitors second (they reference the SLO id).
        let (_, monitors) = build_payloads(slo, &slo_id);
        let live = self.list_monitors_for_slo(&slo.name).await?;
        for mon in &monitors {
            let name = mon
                .pointer("/name")
                .and_then(serde_json::Value::as_str)
                .unwrap_or("");
            match live
                .iter()
                .find(|m| m.pointer("/name").and_then(serde_json::Value::as_str) == Some(name))
            {
                Some(obj) => {
                    let id = obj
                        .pointer("/id")
                        .and_then(serde_json::Value::as_i64)
                        .unwrap_or(0);
                    self.put_json(&format!("/api/v1/monitor/{id}"), mon).await?;
                }
                None => {
                    self.post_json("/api/v1/monitor", mon).await?;
                }
            }
        }
        // Prune monitors that no longer render (e.g. burn entry removed).
        let wanted: std::collections::BTreeSet<&str> = monitors
            .iter()
            .filter_map(|m| m.pointer("/name").and_then(serde_json::Value::as_str))
            .collect();
        for obj in &live {
            let name = obj
                .pointer("/name")
                .and_then(serde_json::Value::as_str)
                .unwrap_or("");
            if !wanted.contains(name) {
                let id = obj
                    .pointer("/id")
                    .and_then(serde_json::Value::as_i64)
                    .unwrap_or(0);
                self.delete(&format!("/api/v1/monitor/{id}")).await?;
            }
        }
        let resolved = build_payloads(slo, &slo_id);
        Ok((slo_id, canonical(&resolved)))
    }
}

#[async_trait::async_trait]
impl SloBackend for DatadogBackend {
    fn name(&self) -> &'static str {
        "datadog"
    }

    fn capabilities(&self) -> Capabilities {
        Capabilities {
            backend: "datadog".to_owned(),
            supports_recording_rules: false,
            supports_alerts: true,
            supports_drift_check: true,
        }
    }

    fn validate(&self, slo: &SloDocument) -> Result<(), AdapterError> {
        if slo.sli.source != "metric" {
            return Err(AdapterError::Invalid(format!(
                "datadog backend supports only sli.source=metric, got {:?}",
                slo.sli.source
            )));
        }
        if slo.sli.good.trim().is_empty() || slo.sli.total.trim().is_empty() {
            return Err(AdapterError::Invalid(
                "sli.good and sli.total must be non-empty".to_owned(),
            ));
        }
        Ok(())
    }

    fn render(&self, slo: &SloDocument) -> Result<Vec<PlannedFile>, AdapterError> {
        Ok(vec![PlannedFile {
            name: format!("{}.datadog.json", slo.name),
            content: self.render_canonical(slo)?,
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
        _out_dir: &std::path::Path,
    ) -> Result<(), AdapterError> {
        let _ = self.apply_remote(slo, None).await?;
        Ok(())
    }

    async fn delete(
        &self,
        slo: &SloDocument,
        _out_dir: &std::path::Path,
    ) -> Result<(), AdapterError> {
        // Monitors first (reverse of apply order), then the SLO.
        for mon in self.list_monitors_for_slo(&slo.name).await? {
            let id = mon
                .pointer("/id")
                .and_then(serde_json::Value::as_i64)
                .unwrap_or(0);
            self.delete(&format!("/api/v1/monitor/{id}")).await?;
        }
        if let Some(obj) = self.find_slo_by_name(&slo.name).await? {
            let id = obj
                .pointer("/id")
                .and_then(serde_json::Value::as_str)
                .unwrap_or("")
                .to_owned();
            if !id.is_empty() {
                self.delete(&format!("/api/v1/slo/{id}")).await?;
            }
        }
        Ok(())
    }

    async fn status(&self, slo: &SloDocument) -> Result<Vec<String>, AdapterError> {
        match self.find_slo_by_name(&slo.name).await? {
            Some(obj) => {
                let id = obj
                    .pointer("/id")
                    .and_then(serde_json::Value::as_str)
                    .unwrap_or("?");
                let mons = self.list_monitors_for_slo(&slo.name).await?;
                Ok(vec![format!(
                    "slo {} (id {id}): {} monitors",
                    slo.name,
                    mons.len()
                )])
            }
            None => Ok(vec![format!("slo {} not present", slo.name)]),
        }
    }

    fn warnings(&self, slo: &SloDocument) -> Vec<String> {
        let mut out = Vec::new();
        let (mapped, changed) = map_timeframe(&slo.window);
        if changed {
            out.push(format!(
                "datadog SLO timeframes are 7d/30d/90d only: window {} mapped to {mapped}",
                slo.window
            ));
        }
        out.push(
            "datadog evaluates Datadog-metric queries: PromQL SLI queries are stored opaquely and not evaluated (lab mock)".to_owned(),
        );
        out
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn timeframe_mapping() {
        assert_eq!(map_timeframe("1h"), ("7d".to_owned(), true));
        assert_eq!(map_timeframe("7d"), ("7d".to_owned(), false));
        assert_eq!(map_timeframe("30d"), ("30d".to_owned(), false));
        assert_eq!(map_timeframe("90d"), ("90d".to_owned(), false));
    }
}
