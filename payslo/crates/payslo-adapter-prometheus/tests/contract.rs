//! Contract suite for the Prometheus backend (temp dir as backend state).

use std::collections::BTreeMap;
use std::path::PathBuf;

use payslo_testkit::ContractHarness;

struct FileHarness {
    dir: PathBuf,
}

fn read_dir(dir: &PathBuf) -> Result<BTreeMap<String, String>, String> {
    let mut out = BTreeMap::new();
    let entries = std::fs::read_dir(dir).map_err(|e| e.to_string())?;
    for entry in entries {
        let entry = entry.map_err(|e| e.to_string())?;
        let path = entry.path();
        if path.extension().is_some_and(|e| e == "yml")
            && let Some(name) = path.file_name().and_then(|n| n.to_str())
            && let Ok(text) = std::fs::read_to_string(&path)
        {
            out.insert(name.to_owned(), text);
        }
    }
    Ok(out)
}

#[async_trait::async_trait]
impl ContractHarness for FileHarness {
    async fn read_actual(&self) -> Result<BTreeMap<String, String>, String> {
        read_dir(&self.dir)
    }

    async fn tamper(&self) -> Result<(), String> {
        // Simulate a manual edit: append a comment to the generated file.
        let path = self.dir.join("payment-success-rate.yml");
        let mut text = std::fs::read_to_string(&path).map_err(|e| e.to_string())?;
        text.push_str("# manual edit by trainee\n");
        std::fs::write(&path, text).map_err(|e| e.to_string())?;
        Ok(())
    }

    async fn is_absent(&self) -> Result<bool, String> {
        Ok(read_dir(&self.dir)?.is_empty())
    }
}

#[tokio::test]
async fn contract_suite() -> Result<(), String> {
    let dir = std::env::temp_dir().join(format!("payslo-contract-prom-{}", std::process::id()));
    std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
    let backend = payslo_adapter_prometheus::PrometheusBackend::new(None);
    let slo = payslo_testkit::fixture_slo("success-rate.yml")?;
    payslo_testkit::run_contract_suite(&backend, &slo, &FileHarness { dir: dir.clone() }, &dir)
        .await
}
