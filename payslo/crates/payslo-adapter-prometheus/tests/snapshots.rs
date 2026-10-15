//! Snapshot tests: every template renders byte-identically on every run.
//! Regenerate with `INSTA_UPDATE=always cargo test` + review (never hand-edit).

use std::path::PathBuf;

fn workspace_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../../payslo-workspace")
}

fn templates_dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../templates")
}

fn render_template(file: &str) -> Result<String, String> {
    let root = workspace_root();
    let doc =
        payslo_model::load_file(&templates_dir().join(file), &root).map_err(|e| e.to_string())?;
    payslo_adapter_prometheus::PrometheusBackend::new(None)
        .render_native_text(&doc)
        .map_err(|e| e.to_string())
}

#[test]
fn snapshot_success_rate() -> Result<(), String> {
    insta::assert_snapshot!(render_template("success-rate.yml")?);
    Ok(())
}

#[test]
fn snapshot_latency_p99() -> Result<(), String> {
    insta::assert_snapshot!(render_template("latency-p99.yml")?);
    Ok(())
}

#[test]
fn snapshot_callback_delivery() -> Result<(), String> {
    insta::assert_snapshot!(render_template("callback-delivery.yml")?);
    Ok(())
}

#[test]
fn snapshot_success_rate_prod() -> Result<(), String> {
    insta::assert_snapshot!(render_template("success-rate-prod.yml")?);
    Ok(())
}

#[test]
fn snapshot_crd_format() -> Result<(), String> {
    let root = workspace_root();
    let doc = payslo_model::load_file(&templates_dir().join("success-rate.yml"), &root)
        .map_err(|e| e.to_string())?;
    let text = payslo_adapter_prometheus::PrometheusBackend::new(None)
        .render_crd_text(&doc)
        .map_err(|e| e.to_string())?;
    insta::assert_snapshot!(text);
    Ok(())
}

#[test]
fn render_is_deterministic() -> Result<(), String> {
    // Same input rendered twice must be byte-identical (no HashMap in output).
    let a = render_template("success-rate.yml")?;
    let b = render_template("success-rate.yml")?;
    assert_eq!(a, b);
    Ok(())
}
