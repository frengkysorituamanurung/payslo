#![forbid(unsafe_code)]

//! datadog-mock: subset tiruan Datadog API v1 (SLO + monitor), BUKAN Datadog asli.
//! Field payload mengikuti dokumentasi resmi (lihat docs/ASSUMPTIONS.md);
//! yang disederhanakan (evaluasi query, history) didokumentasikan sebagai mock.

use std::collections::BTreeMap;
use std::sync::Arc;

use axum::{
    Json,
    body::Bytes,
    extract::{Path, Query, State},
    http::{HeaderMap, StatusCode},
    response::IntoResponse,
};
use serde_json::Value;
use tokio::sync::RwLock;
use tracing::info;

#[derive(Debug, Clone)]
pub struct MockConfig {
    pub api_key: String,
    pub db_file: Option<String>,
}

#[derive(Debug, Default, serde::Serialize, serde::Deserialize)]
struct Db {
    slos: BTreeMap<String, Value>,
    monitors: BTreeMap<String, Value>,
    slo_seq: u64,
    monitor_seq: u64,
}

pub struct MockState {
    pub config: MockConfig,
    inner: RwLock<Db>,
}

impl MockState {
    pub fn new(config: MockConfig) -> Arc<Self> {
        let db = config
            .db_file
            .as_deref()
            .and_then(|f| std::fs::read_to_string(f).ok())
            .and_then(|t| serde_json::from_str(&t).ok())
            .unwrap_or_default();
        Arc::new(Self {
            config,
            inner: RwLock::new(db),
        })
    }

    async fn persist(&self) {
        let Some(file) = self.config.db_file.clone() else {
            return;
        };
        let text = serde_json::to_string(&*self.inner.read().await).unwrap_or_default();
        let tmp = format!("{file}.tmp");
        if std::fs::write(&tmp, text).is_ok() {
            let _ = std::fs::rename(&tmp, &file);
        }
    }
}

fn now_secs() -> i64 {
    time::OffsetDateTime::now_utc().unix_timestamp()
}

fn unauthorized() -> (StatusCode, Json<Value>) {
    (
        StatusCode::FORBIDDEN,
        Json(serde_json::json!({"errors": ["Invalid API key (lab mock: set DD_API_KEY)"]})),
    )
}

fn check_auth(
    state: &Arc<MockState>,
    headers: &HeaderMap,
) -> Result<(), (StatusCode, Json<Value>)> {
    let key = headers
        .get("dd-api-key")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("");
    if key == state.config.api_key {
        Ok(())
    } else {
        Err(unauthorized())
    }
}

fn parse_tags(query: &Query<BTreeMap<String, String>>) -> Vec<String> {
    query
        .get("tags")
        .map(|s| {
            s.split(',')
                .map(str::trim)
                .filter(|t| !t.is_empty())
                .map(str::to_owned)
                .collect()
        })
        .unwrap_or_default()
}

fn has_all_tags(obj: &Value, tags: &[String]) -> bool {
    let owned: Vec<String> = obj
        .pointer("/tags")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default()
        .iter()
        .filter_map(Value::as_str)
        .map(str::to_owned)
        .collect();
    tags.iter().all(|t| owned.contains(t))
}

// ------------------------------------------------------------------ SLO

async fn create_slo(
    State(state): State<Arc<MockState>>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Json<Value>, (StatusCode, Json<Value>)> {
    check_auth(&state, &headers)?;
    let mut slo: Value = serde_json::from_slice(&body).map_err(|_| {
        (
            StatusCode::BAD_REQUEST,
            Json(serde_json::json!({"errors": ["invalid JSON"]})),
        )
    })?;
    if slo
        .pointer("/name")
        .and_then(Value::as_str)
        .unwrap_or("")
        .is_empty()
    {
        return Err((
            StatusCode::BAD_REQUEST,
            Json(serde_json::json!({"errors": ["name is required"]})),
        ));
    }
    let mut db = state.inner.write().await;
    db.slo_seq += 1;
    let id = format!("slo_{:08x}", db.slo_seq);
    if let Some(map) = slo.as_object_mut() {
        map.insert("id".to_owned(), Value::String(id.clone()));
        map.insert("created_at".to_owned(), Value::from(now_secs()));
        map.insert("modified_at".to_owned(), Value::from(now_secs()));
    }
    db.slos.insert(id, slo.clone());
    drop(db);
    state.persist().await;
    info!("mock: SLO created");
    Ok(Json(serde_json::json!({"data": [slo], "errors": []})))
}

async fn list_slos(
    State(state): State<Arc<MockState>>,
    headers: HeaderMap,
    query: Query<BTreeMap<String, String>>,
) -> Result<Json<Value>, (StatusCode, Json<Value>)> {
    check_auth(&state, &headers)?;
    let tags = parse_tags(&query);
    let db = state.inner.read().await;
    let items: Vec<Value> = db
        .slos
        .values()
        .filter(|s| has_all_tags(s, &tags))
        .cloned()
        .collect();
    Ok(Json(serde_json::json!({"data": items})))
}

async fn get_slo(
    State(state): State<Arc<MockState>>,
    headers: HeaderMap,
    Path(id): Path<String>,
) -> Result<Json<Value>, (StatusCode, Json<Value>)> {
    check_auth(&state, &headers)?;
    match state.inner.read().await.slos.get(&id) {
        Some(slo) => Ok(Json(serde_json::json!({"data": slo}))),
        None => Err((
            StatusCode::NOT_FOUND,
            Json(serde_json::json!({"errors": ["SLO not found"]})),
        )),
    }
}

async fn update_slo(
    State(state): State<Arc<MockState>>,
    headers: HeaderMap,
    Path(id): Path<String>,
    body: Bytes,
) -> Result<Json<Value>, (StatusCode, Json<Value>)> {
    check_auth(&state, &headers)?;
    let patch: Value = serde_json::from_slice(&body).map_err(|_| {
        (
            StatusCode::BAD_REQUEST,
            Json(serde_json::json!({"errors": ["invalid JSON"]})),
        )
    })?;
    let mut db = state.inner.write().await;
    let Some(existing) = db.slos.get(&id).cloned() else {
        return Err((
            StatusCode::NOT_FOUND,
            Json(serde_json::json!({"errors": ["SLO not found"]})),
        ));
    };
    let mut merged = existing;
    if let (Some(dst), Some(src)) = (merged.as_object_mut(), patch.as_object()) {
        for (k, v) in src {
            if k != "id" && k != "created_at" {
                dst.insert(k.clone(), v.clone());
            }
        }
        dst.insert("modified_at".to_owned(), Value::from(now_secs()));
    }
    db.slos.insert(id, merged.clone());
    drop(db);
    state.persist().await;
    Ok(Json(serde_json::json!({"data": [merged]})))
}

async fn delete_slo(
    State(state): State<Arc<MockState>>,
    headers: HeaderMap,
    Path(id): Path<String>,
) -> Result<Json<Value>, (StatusCode, Json<Value>)> {
    check_auth(&state, &headers)?;
    let mut db = state.inner.write().await;
    if db.slos.remove(&id).is_none() {
        return Err((
            StatusCode::NOT_FOUND,
            Json(serde_json::json!({"errors": ["SLO not found"]})),
        ));
    }
    drop(db);
    state.persist().await;
    Ok(Json(serde_json::json!({"data": {"deleted_id": id}})))
}

/// Simplified history: shape mirrors the real endpoint; values are mock
/// constants (the mock does not evaluate queries).
async fn slo_history(
    State(state): State<Arc<MockState>>,
    headers: HeaderMap,
    Path(id): Path<String>,
) -> Result<Json<Value>, (StatusCode, Json<Value>)> {
    check_auth(&state, &headers)?;
    let db = state.inner.read().await;
    let Some(slo) = db.slos.get(&id) else {
        return Err((
            StatusCode::NOT_FOUND,
            Json(serde_json::json!({"errors": ["SLO not found"]})),
        ));
    };
    let name = slo.pointer("/name").and_then(Value::as_str).unwrap_or("");
    Ok(Json(serde_json::json!({"data": {
        "id": id,
        "overall": {"name": name, "sli_value": 99.9, "precision": {"unit": 2}},
        "thresholds": slo.pointer("/thresholds").cloned().unwrap_or(Value::Null),
        "history": [],
    }})))
}

// ------------------------------------------------------------------ monitor

async fn create_monitor(
    State(state): State<Arc<MockState>>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Json<Value>, (StatusCode, Json<Value>)> {
    check_auth(&state, &headers)?;
    let mut mon: Value = serde_json::from_slice(&body).map_err(|_| {
        (
            StatusCode::BAD_REQUEST,
            Json(serde_json::json!({"errors": ["invalid JSON"]})),
        )
    })?;
    if mon
        .pointer("/query")
        .and_then(Value::as_str)
        .unwrap_or("")
        .is_empty()
    {
        return Err((
            StatusCode::BAD_REQUEST,
            Json(serde_json::json!({"errors": ["query is required"]})),
        ));
    }
    let mut db = state.inner.write().await;
    db.monitor_seq += 1;
    let id = 1000 + db.monitor_seq;
    if let Some(map) = mon.as_object_mut() {
        map.insert("id".to_owned(), Value::from(id));
        map.insert("created".to_owned(), Value::from(now_secs()));
        map.insert("modified".to_owned(), Value::from(now_secs()));
        map.insert("overall_state".to_owned(), Value::String("OK".to_owned()));
    }
    db.monitors.insert(id.to_string(), mon.clone());
    drop(db);
    state.persist().await;
    info!("mock: monitor created");
    Ok(Json(mon))
}

async fn list_monitors(
    State(state): State<Arc<MockState>>,
    headers: HeaderMap,
    query: Query<BTreeMap<String, String>>,
) -> Result<Json<Value>, (StatusCode, Json<Value>)> {
    check_auth(&state, &headers)?;
    let tags = parse_tags(&query);
    let db = state.inner.read().await;
    let items: Vec<Value> = db
        .monitors
        .values()
        .filter(|m| has_all_tags(m, &tags))
        .cloned()
        .collect();
    Ok(Json(Value::Array(items)))
}

async fn get_monitor(
    State(state): State<Arc<MockState>>,
    headers: HeaderMap,
    Path(id): Path<i64>,
) -> Result<Json<Value>, (StatusCode, Json<Value>)> {
    check_auth(&state, &headers)?;
    match state.inner.read().await.monitors.get(&id.to_string()) {
        Some(mon) => Ok(Json(mon.clone())),
        None => Err((
            StatusCode::NOT_FOUND,
            Json(serde_json::json!({"errors": ["Monitor not found"]})),
        )),
    }
}

async fn update_monitor(
    State(state): State<Arc<MockState>>,
    headers: HeaderMap,
    Path(id): Path<i64>,
    body: Bytes,
) -> Result<Json<Value>, (StatusCode, Json<Value>)> {
    check_auth(&state, &headers)?;
    let patch: Value = serde_json::from_slice(&body).map_err(|_| {
        (
            StatusCode::BAD_REQUEST,
            Json(serde_json::json!({"errors": ["invalid JSON"]})),
        )
    })?;
    let mut db = state.inner.write().await;
    let Some(existing) = db.monitors.get(&id.to_string()).cloned() else {
        return Err((
            StatusCode::NOT_FOUND,
            Json(serde_json::json!({"errors": ["Monitor not found"]})),
        ));
    };
    let mut merged = existing;
    if let (Some(dst), Some(src)) = (merged.as_object_mut(), patch.as_object()) {
        for (k, v) in src {
            if k != "id" && k != "created" {
                dst.insert(k.clone(), v.clone());
            }
        }
        dst.insert("modified".to_owned(), Value::from(now_secs()));
    }
    db.monitors.insert(id.to_string(), merged.clone());
    drop(db);
    state.persist().await;
    Ok(Json(merged))
}

async fn delete_monitor(
    State(state): State<Arc<MockState>>,
    headers: HeaderMap,
    Path(id): Path<i64>,
) -> Result<Json<Value>, (StatusCode, Json<Value>)> {
    check_auth(&state, &headers)?;
    let mut db = state.inner.write().await;
    if db.monitors.remove(&id.to_string()).is_none() {
        return Err((
            StatusCode::NOT_FOUND,
            Json(serde_json::json!({"errors": ["Monitor not found"]})),
        ));
    }
    drop(db);
    state.persist().await;
    Ok(Json(serde_json::json!({"deleted_monitor_id": id})))
}

async fn healthz() -> Json<Value> {
    Json(serde_json::json!({"ok": true, "mock": true}))
}

/// Build the full route table (shared by `main` and tests).
pub fn router(state: Arc<MockState>) -> axum::Router {
    use axum::routing::{get, post};
    axum::Router::new()
        .route("/healthz", get(healthz))
        .route("/api/v1/slo", post(create_slo).get(list_slos))
        .route(
            "/api/v1/slo/{id}",
            get(get_slo).put(update_slo).delete(delete_slo),
        )
        .route("/api/v1/slo/{id}/history", get(slo_history))
        .route("/api/v1/monitor", post(create_monitor).get(list_monitors))
        .route(
            "/api/v1/monitor/{id}",
            get(get_monitor).put(update_monitor).delete(delete_monitor),
        )
        .with_state(state)
}

pub fn not_found_fallback() -> (StatusCode, Json<Value>) {
    (
        StatusCode::NOT_FOUND,
        Json(serde_json::json!({"errors": ["not found (lab mock)"]})),
    )
}

pub async fn fallback() -> impl IntoResponse {
    not_found_fallback()
}

#[cfg(test)]
mod tests {
    use super::*;

    async fn serve() -> Result<(String, reqwest::Client, tokio::task::JoinHandle<()>), String> {
        let state = MockState::new(MockConfig {
            api_key: "test-key".to_owned(),
            db_file: None,
        });
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
            .await
            .map_err(|e| e.to_string())?;
        let addr = listener.local_addr().map_err(|e| e.to_string())?;
        let handle = tokio::spawn(async move {
            if axum::serve(listener, router(state).fallback(fallback))
                .await
                .is_err()
            {
                // Listener closed; test over.
            }
        });
        Ok((format!("http://{addr}"), reqwest::Client::new(), handle))
    }

    fn keyed(req: reqwest::RequestBuilder) -> reqwest::RequestBuilder {
        req.header("DD-API-KEY", "test-key")
    }

    #[tokio::test]
    async fn slo_crud_roundtrip() -> Result<(), String> {
        let (base, http, server) = serve().await?;
        // Auth enforced.
        let resp = http
            .post(format!("{base}/api/v1/slo"))
            .json(&serde_json::json!({}))
            .send()
            .await
            .map_err(|e| e.to_string())?;
        assert_eq!(resp.status(), 403);
        // Create.
        let created: Value = keyed(http.post(format!("{base}/api/v1/slo")))
            .json(&serde_json::json!({"type": "metric", "name": "s1", "thresholds": [{"target": 99.0, "timeframe": "7d"}], "tags": ["managed-by:payslo"]}))
            .send().await.map_err(|e| e.to_string())?
            .json().await.map_err(|e| e.to_string())?;
        let id = created
            .pointer("/data/0/id")
            .and_then(Value::as_str)
            .ok_or("no id")?
            .to_owned();
        // Get + list with tag filter.
        let got: Value = keyed(http.get(format!("{base}/api/v1/slo/{id}")))
            .send()
            .await
            .map_err(|e| e.to_string())?
            .json()
            .await
            .map_err(|e| e.to_string())?;
        assert_eq!(
            got.pointer("/data/name").and_then(Value::as_str),
            Some("s1")
        );
        let listed: Value = keyed(http.get(format!("{base}/api/v1/slo?tags=managed-by:payslo")))
            .send()
            .await
            .map_err(|e| e.to_string())?
            .json()
            .await
            .map_err(|e| e.to_string())?;
        assert_eq!(
            listed
                .pointer("/data")
                .and_then(Value::as_array)
                .map(Vec::len),
            Some(1)
        );
        let listed_none: Value = keyed(http.get(format!("{base}/api/v1/slo?tags=nope:x")))
            .send()
            .await
            .map_err(|e| e.to_string())?
            .json()
            .await
            .map_err(|e| e.to_string())?;
        assert_eq!(
            listed_none
                .pointer("/data")
                .and_then(Value::as_array)
                .map(Vec::len),
            Some(0)
        );
        // History + update + delete.
        let hist: Value = keyed(http.get(format!("{base}/api/v1/slo/{id}/history")))
            .send()
            .await
            .map_err(|e| e.to_string())?
            .json()
            .await
            .map_err(|e| e.to_string())?;
        assert_eq!(
            hist.pointer("/data/id").and_then(Value::as_str),
            Some(id.as_str())
        );
        keyed(http.put(format!("{base}/api/v1/slo/{id}")))
            .json(&serde_json::json!({"description": "d"}))
            .send()
            .await
            .map_err(|e| e.to_string())?;
        let resp = keyed(http.delete(format!("{base}/api/v1/slo/{id}")))
            .send()
            .await
            .map_err(|e| e.to_string())?;
        assert_eq!(resp.status(), 200);
        let resp = keyed(http.get(format!("{base}/api/v1/slo/{id}")))
            .send()
            .await
            .map_err(|e| e.to_string())?;
        assert_eq!(resp.status(), 404);
        server.abort();
        Ok(())
    }

    #[tokio::test]
    async fn monitor_crud_roundtrip() -> Result<(), String> {
        let (base, http, server) = serve().await?;
        let created: Value = keyed(http.post(format!("{base}/api/v1/monitor")))
            .json(&serde_json::json!({"type": "slo alert", "query": "error_budget(\"slo_1\").over(\"7d\") >= 14.4", "name": "m1", "tags": ["managed-by:payslo"]}))
            .send().await.map_err(|e| e.to_string())?
            .json().await.map_err(|e| e.to_string())?;
        let id = created
            .pointer("/id")
            .and_then(Value::as_i64)
            .ok_or("no id")?;
        assert!(id > 0);
        let listed: Value =
            keyed(http.get(format!("{base}/api/v1/monitor?tags=managed-by:payslo")))
                .send()
                .await
                .map_err(|e| e.to_string())?
                .json()
                .await
                .map_err(|e| e.to_string())?;
        assert_eq!(listed.as_array().map(Vec::len), Some(1));
        keyed(http.put(format!("{base}/api/v1/monitor/{id}")))
            .json(&serde_json::json!({"message": "hi"}))
            .send()
            .await
            .map_err(|e| e.to_string())?;
        let resp = keyed(http.delete(format!("{base}/api/v1/monitor/{id}")))
            .send()
            .await
            .map_err(|e| e.to_string())?;
        assert_eq!(resp.status(), 200);
        server.abort();
        Ok(())
    }
}
