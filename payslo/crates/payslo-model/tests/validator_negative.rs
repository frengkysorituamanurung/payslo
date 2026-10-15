//! Negative validator tests: each invalid document is rejected with an
//! actionable message (file + field path). Counts toward the ">= 6 invalid
//! cases" acceptance.

use std::path::PathBuf;

use payslo_model::ModelError;

fn workspace_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../../payslo-workspace")
}

fn write_case(dir: &std::path::Path, name: &str, body: &str) -> Result<PathBuf, String> {
    let path = dir.join(name);
    std::fs::write(&path, body).map_err(|e| e.to_string())?;
    Ok(path)
}

fn base() -> String {
    r#"
apiVersion: payslo/v1
kind: SLO
name: case-x
service: gateway-api
objective: 0.99
window: 1h
sli:
  type: success-ratio
  source: metric
  good: up
  total: up
alerting:
  runbook: runbooks/payment-success.md
  burnRates:
    - longWindow: 15m
      shortWindow: 5m
      burnRate: 6
      severity: page
targets:
  - backend: prometheus
"#
    .to_owned()
}

fn load(case: &str, body: &str) -> Result<payslo_model::SloDocument, ModelError> {
    let dir = std::env::temp_dir().join(format!("payslo-neg-{}", std::process::id()));
    std::fs::create_dir_all(&dir).map_err(|e| ModelError::Io {
        file: dir.display().to_string(),
        reason: e.to_string(),
    })?;
    let path = write_case(&dir, &format!("{case}.yml"), body).map_err(|e| ModelError::Io {
        file: case.to_owned(),
        reason: e,
    })?;
    payslo_model::load_file(&path, &workspace_root())
}

fn expect_reject(case: &str, body: &str) -> Result<String, String> {
    match load(case, body) {
        Ok(_) => Err(format!("{case}: document must be rejected")),
        Err(e) => Ok(e.to_string()),
    }
}

#[test]
fn rejects_wrong_api_version() -> Result<(), String> {
    let msg = expect_reject("api-version", &base().replace("payslo/v1", "payslo/v9"))?;
    assert!(msg.contains("apiVersion"), "{msg}");
    Ok(())
}

#[test]
fn rejects_objective_out_of_range() -> Result<(), String> {
    let msg = expect_reject(
        "objective",
        &base().replace("objective: 0.99", "objective: 1.5"),
    )?;
    assert!(msg.contains("objective"), "{msg}");
    Ok(())
}

#[test]
fn rejects_short_not_less_than_long() -> Result<(), String> {
    let msg = expect_reject(
        "short-long",
        &base().replace("shortWindow: 5m", "shortWindow: 15m"),
    )?;
    assert!(msg.contains("shortWindow"), "{msg}");
    Ok(())
}

#[test]
fn rejects_long_window_beyond_objective() -> Result<(), String> {
    let msg = expect_reject(
        "long-window",
        &base().replace("longWindow: 15m", "longWindow: 6h"),
    )?;
    assert!(msg.contains("longWindow"), "{msg}");
    Ok(())
}

#[test]
fn rejects_missing_runbook() -> Result<(), String> {
    let msg = expect_reject(
        "runbook",
        &base().replace("runbooks/payment-success.md", "runbooks/nope.md"),
    )?;
    assert!(msg.contains("runbook"), "{msg}");
    Ok(())
}

#[test]
fn rejects_unknown_field() -> Result<(), String> {
    // Top-level unknown field trips `deny_unknown_fields`.
    let msg = expect_reject(
        "unknown",
        &base().replace("kind: SLO", "kind: SLO\nfrobnicate: true"),
    )?;
    assert!(!msg.is_empty());
    Ok(())
}

#[test]
fn rejects_unsupported_backend_and_bad_severity() -> Result<(), String> {
    let msg = expect_reject(
        "backend",
        &base().replace("backend: prometheus", "backend: nagios"),
    )?;
    assert!(msg.contains("nagios"), "{msg}");
    Ok(())
}

#[test]
fn rejects_duplicate_names_in_dir() -> Result<(), String> {
    let dir = std::env::temp_dir().join(format!("payslo-dup-{}", std::process::id()));
    std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
    write_case(&dir, "a.yml", &base())?;
    write_case(&dir, "b.yml", &base())?;
    match payslo_model::load_dir(&dir, &workspace_root()) {
        Ok(_) => Err("duplicate names must be rejected".to_owned()),
        Err(ModelError::DuplicateName { name, .. }) => {
            assert_eq!(name, "case-x");
            Ok(())
        }
        Err(other) => Err(format!("wrong error: {other}")),
    }
}
