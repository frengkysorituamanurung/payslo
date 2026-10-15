#![forbid(unsafe_code)]

//! Webhook normalization: Alertmanager + Datadog payloads -> CanonicalAlert.
//!
//! Fingerprint (idempotency): sha256 over source, alert name, status, and
//! sorted stable labels. Volatile fields (timestamps, values) are excluded so
//! re-deliveries of the same alert collapse to one row.

use std::collections::BTreeMap;

use payslo_model::{AlertStatus, CanonicalAlert};
use sha2::{Digest, Sha256};
use thiserror::Error;

#[derive(Debug, Error)]
pub enum NormalizeError {
    #[error("bad {origin} payload: {reason}")]
    BadPayload { origin: String, reason: String },
}

/// Compute the idempotency fingerprint.
/// The same logical alert keeps one fingerprint across firing->resolved
/// (status excluded); re-deliveries upsert instead of duplicating.
#[must_use]
pub fn fingerprint(source: &str, name: &str, labels: &BTreeMap<String, String>) -> String {
    // Volatile keys that must not break dedup.
    let skip = ["value", "alertstate"];
    let mut hasher = Sha256::new();
    hasher.update(source.as_bytes());
    hasher.update([0]);
    hasher.update(name.as_bytes());
    for (k, v) in labels {
        if skip.contains(&k.as_str()) {
            continue;
        }
        hasher.update([0]);
        hasher.update(k.as_bytes());
        hasher.update([0]);
        hasher.update(v.as_bytes());
    }
    hex::encode(hasher.finalize())
}

fn status_of_firing(firing: bool) -> AlertStatus {
    if firing {
        AlertStatus::Firing
    } else {
        AlertStatus::Resolved
    }
}

fn pick<'a>(labels: &'a BTreeMap<String, String>, keys: &[&str], default: &'a str) -> &'a str {
    keys.iter()
        .find_map(|k| labels.get(*k).map(String::as_str))
        .unwrap_or(default)
}

/// Normalize an Alertmanager webhook body (`{"alerts": [...]}`).
/// Returns (alert, fingerprint) pairs.
pub fn from_alertmanager(
    body: &serde_json::Value,
) -> Result<Vec<(CanonicalAlert, String)>, NormalizeError> {
    let bad = |reason: &str| NormalizeError::BadPayload {
        origin: "alertmanager".to_owned(),
        reason: reason.to_owned(),
    };
    let alerts = body
        .pointer("/alerts")
        .and_then(serde_json::Value::as_array)
        .ok_or_else(|| bad("missing alerts[]"))?;
    let mut out = Vec::with_capacity(alerts.len());
    for alert in alerts {
        let labels: BTreeMap<String, String> = alert
            .pointer("/labels")
            .and_then(serde_json::Value::as_object)
            .map(|m| {
                m.iter()
                    .filter_map(|(k, v)| v.as_str().map(|s| (k.clone(), s.to_owned())))
                    .collect()
            })
            .unwrap_or_default();
        let status_str = alert
            .pointer("/status")
            .and_then(serde_json::Value::as_str)
            .unwrap_or("firing");
        let status = status_of_firing(status_str != "resolved");
        let name = pick(&labels, &["alertname"], "unknown").to_owned();
        let slo = pick(&labels, &["slo"], "unknown").to_owned();
        let service = pick(&labels, &["service"], "unknown").to_owned();
        let severity = pick(&labels, &["severity"], "ticket").to_owned();
        let mut full_labels = labels.clone();
        full_labels.insert("alertname".to_owned(), name.clone());
        let fingerprint = fingerprint("alertmanager", &name, &full_labels);
        out.push((
            CanonicalAlert {
                slo,
                service,
                severity,
                status,
                starts_at: alert
                    .pointer("/startsAt")
                    .and_then(serde_json::Value::as_str)
                    .map(str::to_owned),
                ends_at: alert
                    .pointer("/endsAt")
                    .and_then(serde_json::Value::as_str)
                    .map(str::to_owned),
                labels: full_labels,
                runbook_url: alert
                    .pointer("/annotations/runbook_url")
                    .and_then(serde_json::Value::as_str)
                    .map(str::to_owned),
            },
            fingerprint,
        ));
    }
    Ok(out)
}

/// Normalize a Datadog monitor webhook body.
///
/// Accepted (mock-documented) shape:
/// `{"title": ..., "text": ..., "alert_type": "error|warning|success",
///   "priority": "P1".."P5", "id": ..., "tags": ["k:v", ...]}`.
/// Severity mapping: P1/P2 -> page, else ticket; `success` -> resolved.
pub fn from_datadog(
    body: &serde_json::Value,
) -> Result<Vec<(CanonicalAlert, String)>, NormalizeError> {
    let bad = |reason: &str| NormalizeError::BadPayload {
        origin: "datadog".to_owned(),
        reason: reason.to_owned(),
    };
    if !body.is_object() {
        return Err(bad("body must be an object"));
    }
    let mut labels = BTreeMap::new();
    for tag in body
        .pointer("/tags")
        .and_then(serde_json::Value::as_array)
        .cloned()
        .unwrap_or_default()
    {
        if let Some(tag) = tag.as_str() {
            match tag.split_once(':') {
                Some((k, v)) => {
                    labels.insert(k.to_owned(), v.to_owned());
                }
                None => {
                    labels.insert(tag.to_owned(), "true".to_owned());
                }
            }
        }
    }
    let alert_type = body
        .pointer("/alert_type")
        .and_then(serde_json::Value::as_str)
        .unwrap_or("error");
    let status = status_of_firing(alert_type != "success");
    let priority = body
        .pointer("/priority")
        .and_then(serde_json::Value::as_str)
        .unwrap_or("P3");
    let severity = match priority {
        "P1" | "P2" => "page",
        _ => "ticket",
    }
    .to_owned();
    let name = body
        .pointer("/title")
        .and_then(serde_json::Value::as_str)
        .unwrap_or("datadog-alert")
        .to_owned();
    let slo = pick(&labels, &["slo"], "unknown").to_owned();
    let service = pick(&labels, &["service"], "unknown").to_owned();
    let mut full_labels = labels;
    full_labels.insert("alertname".to_owned(), name.clone());
    let fingerprint = fingerprint("datadog", &name, &full_labels);
    let alert = CanonicalAlert {
        slo,
        service,
        severity,
        status,
        starts_at: body
            .pointer("/date_happened")
            .and_then(serde_json::Value::as_i64)
            .map(|t| t.to_string()),
        ends_at: None,
        labels: full_labels,
        runbook_url: None,
    };
    Ok(vec![(alert, fingerprint)])
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn alertmanager_maps_labels() -> Result<(), String> {
        let body = serde_json::json!({
            "alerts": [{
                "status": "firing",
                "labels": {"alertname": "PaySLOBurnRateHigh", "slo": "payment-success-rate", "service": "gateway-api", "severity": "page"},
                "annotations": {"runbook_url": "runbooks/x.md"},
            }],
        });
        let alerts = from_alertmanager(&body).map_err(|e| e.to_string())?;
        assert_eq!(alerts.len(), 1);
        assert_eq!(alerts[0].0.slo, "payment-success-rate");
        assert_eq!(alerts[0].0.severity, "page");
        assert_eq!(alerts[0].0.runbook_url.as_deref(), Some("runbooks/x.md"));
        assert_eq!(alerts[0].1.len(), 64);
        Ok(())
    }

    #[test]
    fn datadog_maps_priority() -> Result<(), String> {
        let body = serde_json::json!({
            "title": "burn", "alert_type": "error", "priority": "P1",
            "tags": ["slo:payment-success-rate", "service:gateway-api"],
        });
        let alerts = from_datadog(&body).map_err(|e| e.to_string())?;
        assert_eq!(alerts[0].0.severity, "page");
        assert_eq!(alerts[0].0.status, AlertStatus::Firing);
        let ok = serde_json::json!({"title": "burn", "alert_type": "success", "priority": "P1", "tags": []});
        let alerts = from_datadog(&ok).map_err(|e| e.to_string())?;
        assert_eq!(alerts[0].0.status, AlertStatus::Resolved);
        Ok(())
    }

    #[test]
    fn fingerprint_stable_and_volatile_insensitive() {
        let mut labels = BTreeMap::from([("slo".to_owned(), "x".to_owned())]);
        let a = fingerprint("am", "n", &labels);
        labels.insert("value".to_owned(), "0.5".to_owned());
        let b = fingerprint("am", "n", &labels);
        assert_eq!(a, b);
        labels.insert("slo".to_owned(), "y".to_owned());
        assert_ne!(a, fingerprint("am", "n", &labels));
    }
}
