//! Contract suite for the Datadog backend against in-process datadog-mock.

use payslo_adapter_datadog::{DatadogBackend, DatadogConfig};
use payslo_testkit::ContractHarness;

struct MockHarness {
    backend: DatadogBackend,
    endpoint: String,
    api_key: String,
    file_name: String,
}

async fn serve_mock() -> Result<(String, tokio::task::JoinHandle<()>), String> {
    let state = datadog_mock::MockState::new(datadog_mock::MockConfig {
        api_key: "contract-key".to_owned(),
        db_file: None,
    });
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
        .await
        .map_err(|e| e.to_string())?;
    let addr = listener.local_addr().map_err(|e| e.to_string())?;
    let handle = tokio::spawn(async move {
        if axum::serve(listener, datadog_mock::router(state))
            .await
            .is_err()
        {
            // Listener closed; test over.
        }
    });
    Ok((format!("http://{addr}"), handle))
}

#[async_trait::async_trait]
impl ContractHarness for MockHarness {
    async fn read_actual(&self) -> Result<std::collections::BTreeMap<String, String>, String> {
        let slo = payslo_testkit::fixture_slo("success-rate.yml")?;
        let mut out = std::collections::BTreeMap::new();
        if let Some(content) = self
            .backend
            .fetch_canonical(&slo, None)
            .await
            .map_err(|e| e.to_string())?
        {
            out.insert(self.file_name.clone(), content);
        }
        Ok(out)
    }

    async fn tamper(&self) -> Result<(), String> {
        // Simulate a manual monitor edit straight through the mock API.
        let mons: serde_json::Value = reqwest::Client::new()
            .get(format!(
                "{}/api/v1/monitor?tags=managed-by:payslo",
                self.endpoint
            ))
            .header("DD-API-KEY", &self.api_key)
            .send()
            .await
            .map_err(|e| e.to_string())?
            .json()
            .await
            .map_err(|e| e.to_string())?;
        let id = mons
            .as_array()
            .and_then(|a| a.first())
            .and_then(|m| m.pointer("/id"))
            .and_then(serde_json::Value::as_i64)
            .ok_or("no monitors to tamper with")?;
        reqwest::Client::new()
            .put(format!("{}/api/v1/monitor/{id}", self.endpoint))
            .header("DD-API-KEY", &self.api_key)
            .json(&serde_json::json!({"message": "hand-edited by trainee"}))
            .send()
            .await
            .map_err(|e| e.to_string())?;
        Ok(())
    }

    async fn is_absent(&self) -> Result<bool, String> {
        let slo = payslo_testkit::fixture_slo("success-rate.yml")?;
        Ok(self
            .backend
            .fetch_canonical(&slo, None)
            .await
            .map_err(|e| e.to_string())?
            .is_none())
    }
}

#[tokio::test]
async fn contract_suite() -> Result<(), String> {
    let (endpoint, server) = serve_mock().await?;
    let backend = DatadogBackend::new(DatadogConfig {
        endpoint: endpoint.clone(),
        api_key: "contract-key".to_owned(),
    });
    let slo = payslo_testkit::fixture_slo("success-rate.yml")?;
    let harness = MockHarness {
        backend: backend.clone(),
        endpoint,
        api_key: "contract-key".to_owned(),
        file_name: format!("{}.datadog.json", slo.name),
    };
    let workdir = std::env::temp_dir().join(format!("payslo-contract-dd-{}", std::process::id()));
    let result = payslo_testkit::run_contract_suite(&backend, &slo, &harness, &workdir).await;
    server.abort();
    result
}
