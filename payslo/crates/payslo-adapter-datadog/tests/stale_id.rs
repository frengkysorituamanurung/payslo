//! Stale external-id must never overwrite a different object.

use payslo_adapter_datadog::{DatadogBackend, DatadogConfig};

async fn serve_mock() -> Result<(String, tokio::task::JoinHandle<()>), String> {
    let state = datadog_mock::MockState::new(datadog_mock::MockConfig {
        api_key: "k".to_owned(),
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
        {}
    });
    Ok((format!("http://{addr}"), handle))
}

fn keyed(req: reqwest::RequestBuilder) -> reqwest::RequestBuilder {
    req.header("DD-API-KEY", "k")
}

#[tokio::test]
async fn stale_id_does_not_clobber() -> Result<(), String> {
    let (endpoint, server) = serve_mock().await?;
    let http = reqwest::Client::new();
    // Pre-existing unrelated SLO occupying id slo_00000001.
    let created: serde_json::Value = keyed(http.post(format!("{endpoint}/api/v1/slo")))
        .json(&serde_json::json!({"type": "metric", "name": "other", "thresholds": [{"target": 99.0, "timeframe": "7d"}]}))
        .send()
        .await
        .map_err(|e| e.to_string())?
        .json()
        .await
        .map_err(|e| e.to_string())?;
    let other_id = created
        .pointer("/data/0/id")
        .and_then(serde_json::Value::as_str)
        .ok_or("no id")?
        .to_owned();

    let backend = DatadogBackend::new(DatadogConfig {
        endpoint: endpoint.clone(),
        api_key: "k".to_owned(),
    });
    let root =
        std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../../payslo-workspace");
    let slo = payslo_model::load_file(&root.join("slo/success-rate.yml"), &root)
        .map_err(|e| e.to_string())?;
    // Apply with a STALE external id pointing at the unrelated SLO.
    let (new_id, _) = backend
        .apply_remote(&slo, Some(&other_id))
        .await
        .map_err(|e| e.to_string())?;
    assert_ne!(new_id, other_id, "must create fresh, not overwrite");
    // The unrelated SLO is untouched.
    let kept: serde_json::Value = keyed(http.get(format!("{endpoint}/api/v1/slo/{other_id}")))
        .send()
        .await
        .map_err(|e| e.to_string())?
        .json()
        .await
        .map_err(|e| e.to_string())?;
    assert_eq!(
        kept.pointer("/data/name")
            .and_then(serde_json::Value::as_str),
        Some("other")
    );
    server.abort();
    Ok(())
}
