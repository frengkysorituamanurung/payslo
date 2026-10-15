#![forbid(unsafe_code)]

//! Generic backend contract suite. Every adapter test calls
//! [`run_contract_suite`] so all backends guarantee the same behavior:
//! deterministic render, idempotent apply, empty plan after apply, drift
//! detection after out-of-band modification, clean delete.
//!
//! Backend specifics live in [`ContractHarness`] (actual state lives in
//! files for Prometheus, behind HTTP for Datadog).

use std::collections::BTreeMap;
use std::path::Path;

use payslo_adapter::SloBackend;
use payslo_model::SloDocument;

/// Backend-specific test hooks (implemented per adapter test).
#[async_trait::async_trait]
pub trait ContractHarness: Send + Sync {
    /// Current backend state as name->content map.
    async fn read_actual(&self) -> Result<BTreeMap<String, String>, String>;
    /// Modify backend state out-of-band (simulates manual edits).
    async fn tamper(&self) -> Result<(), String>;
    /// True when the SLO has no trace left in the backend.
    async fn is_absent(&self) -> Result<bool, String>;
}

/// Run the full contract against `backend` + `slo`, using `workdir` as the
/// apply output directory (the harness should observe the same directory
/// for file-based backends).
pub async fn run_contract_suite<B>(
    backend: &B,
    slo: &SloDocument,
    harness: &dyn ContractHarness,
    workdir: &Path,
) -> Result<(), String>
where
    B: SloBackend,
{
    // 1. Render is deterministic.
    let first = backend
        .render(slo)
        .map_err(|e| format!("render failed: {e}"))?;
    let second = backend
        .render(slo)
        .map_err(|e| format!("second render failed: {e}"))?;
    if first != second {
        return Err("render is not deterministic".to_owned());
    }
    if first.is_empty() {
        return Err("render produced no files".to_owned());
    }

    // 2. Apply, then plan must be empty (up-to-date).
    std::fs::create_dir_all(workdir).map_err(|e| e.to_string())?;
    backend
        .apply(slo, workdir)
        .await
        .map_err(|e| format!("apply failed: {e}"))?;
    assert_plan_empty(backend, slo, harness, "after apply").await?;

    // 3. Second apply is a no-op (still empty plan, still success).
    backend
        .apply(slo, workdir)
        .await
        .map_err(|e| format!("second apply failed: {e}"))?;
    assert_plan_empty(backend, slo, harness, "after second apply").await?;

    // 4. Out-of-band modification is detected as drift (non-empty plan).
    harness.tamper().await?;
    let planned = backend
        .plan(slo, &harness.read_actual().await?)
        .map_err(|e| format!("plan after tamper failed: {e}"))?;
    if planned.is_empty() {
        return Err("drift after manual modification was NOT detected".to_owned());
    }

    // 5. Delete cleans up.
    backend
        .delete(slo, workdir)
        .await
        .map_err(|e| format!("delete failed: {e}"))?;
    if !harness.is_absent().await? {
        return Err("delete left objects behind".to_owned());
    }
    Ok(())
}

async fn assert_plan_empty<B>(
    backend: &B,
    slo: &SloDocument,
    harness: &dyn ContractHarness,
    stage: &str,
) -> Result<(), String>
where
    B: SloBackend,
{
    let planned = backend
        .plan(slo, &harness.read_actual().await?)
        .map_err(|e| format!("plan {stage} failed: {e}"))?;
    if planned.is_empty() {
        Ok(())
    } else {
        Err(format!(
            "plan {stage} should be empty, has {} file(s)",
            planned.len()
        ))
    }
}

/// Load the shared lab SLO fixture used by contract tests.
pub fn fixture_slo(name: &str) -> Result<SloDocument, String> {
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("../../../payslo-workspace");
    payslo_model::load_file(&root.join("slo").join(name), &root).map_err(|e| e.to_string())
}

#[cfg(test)]
mod tests {
    #[test]
    fn fixture_loads() -> Result<(), String> {
        let slo = super::fixture_slo("success-rate.yml")?;
        assert_eq!(slo.name, "payment-success-rate");
        Ok(())
    }
}
