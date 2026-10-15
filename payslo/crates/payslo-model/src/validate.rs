#![forbid(unsafe_code)]

//! Code-level validation on top of the JSON Schema: duration semantics
//! (short < long, backend window limits), runbook existence, name uniqueness.
//! Durations: `<int><s|m|h|d|w>`, e.g. `1m`, `15m`, `1h`, `30d`.

use std::path::Path;

use crate::error::ModelError;
use crate::model::SloDocument;

/// Seconds per unit.
fn unit_seconds(unit: char) -> Option<u64> {
    match unit {
        's' => Some(1),
        'm' => Some(60),
        'h' => Some(3600),
        'd' => Some(86_400),
        'w' => Some(604_800),
        _ => None,
    }
}

/// Parse `1m`/`15m`/`1h`/`30d` into seconds.
pub fn parse_duration(raw: &str) -> Option<u64> {
    let (num, unit) = raw.split_at(raw.len().checked_sub(1)?);
    let n: u64 = num.parse().ok()?;
    if n == 0 {
        return None;
    }
    n.checked_mul(unit_seconds(unit.chars().next()?)?)
}

/// Backend window limits (seconds). Prometheus range selectors cap out;
/// keep lab windows far below backend limits.
const PROMETHEUS_MIN_WINDOW: u64 = 60;
const PROMETHEUS_MAX_WINDOW: u64 = 90 * 86_400;

/// Validate one parsed document. `workspace_root` resolves `runbook`.
pub fn validate_document(
    file: &str,
    doc: &SloDocument,
    workspace_root: &Path,
) -> Result<(), ModelError> {
    let v = |path: &str, reason: String| ModelError::validation(file, path, reason);

    if doc.api_version != "payslo/v1" {
        return Err(v(
            "apiVersion",
            format!("must be \"payslo/v1\", got {:?}", doc.api_version),
        ));
    }
    if doc.kind != "SLO" {
        return Err(v("kind", format!("must be \"SLO\", got {:?}", doc.kind)));
    }
    if !(0.0 < doc.objective && doc.objective < 1.0) {
        return Err(v(
            "objective",
            format!("must be in (0, 1), got {}", doc.objective),
        ));
    }
    let window_secs = parse_duration(&doc.window).ok_or_else(|| {
        v(
            "window",
            format!("bad duration {:?}, want e.g. 1h or 30d", doc.window),
        )
    })?;
    if doc.targets.is_empty() {
        return Err(v(
            "targets",
            "at least one backend target is required".to_owned(),
        ));
    }
    for (i, target) in doc.targets.iter().enumerate() {
        match target.backend.as_str() {
            "prometheus" => {
                if !(PROMETHEUS_MIN_WINDOW..=PROMETHEUS_MAX_WINDOW).contains(&window_secs) {
                    return Err(v(
                        &format!("targets[{i}].backend"),
                        format!("window {} out of prometheus range 1m..90d", doc.window),
                    ));
                }
            }
            "datadog" => {}
            other => {
                return Err(v(
                    &format!("targets[{i}].backend"),
                    format!("unsupported backend {other:?}, want prometheus|datadog"),
                ));
            }
        }
    }

    for (i, burn) in doc.alerting.burn_rates.iter().enumerate() {
        let base = format!("alerting.burnRates[{i}]");
        let long = parse_duration(&burn.long_window).ok_or_else(|| {
            v(
                &format!("{base}.longWindow"),
                format!("bad duration {:?}", burn.long_window),
            )
        })?;
        let short = parse_duration(&burn.short_window).ok_or_else(|| {
            v(
                &format!("{base}.shortWindow"),
                format!("bad duration {:?}", burn.short_window),
            )
        })?;
        if short >= long {
            return Err(v(
                &format!("{base}.shortWindow"),
                format!(
                    "shortWindow ({}) must be < longWindow ({})",
                    burn.short_window, burn.long_window
                ),
            ));
        }
        if long > window_secs {
            return Err(v(
                &format!("{base}.longWindow"),
                format!(
                    "longWindow ({}) must be <= objective window ({})",
                    burn.long_window, doc.window
                ),
            ));
        }
        if !burn.burn_rate.is_finite() || burn.burn_rate <= 0.0 {
            return Err(v(
                &format!("{base}.burnRate"),
                format!("burnRate must be > 0, got {}", burn.burn_rate),
            ));
        }
    }

    let runbook_path = workspace_root.join(&doc.alerting.runbook);
    if !runbook_path.is_file() {
        return Err(v(
            "alerting.runbook",
            format!(
                "runbook {:?} not found (resolved to {})",
                doc.alerting.runbook,
                runbook_path.display()
            ),
        ));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn durations_parse() {
        assert_eq!(parse_duration("1m"), Some(60));
        assert_eq!(parse_duration("90d"), Some(90 * 86_400));
        assert_eq!(parse_duration("2w"), Some(2 * 604_800));
        assert_eq!(parse_duration("0m"), None);
        assert_eq!(parse_duration("m"), None);
        assert_eq!(parse_duration("10x"), None);
        assert_eq!(parse_duration(""), None);
    }
}
