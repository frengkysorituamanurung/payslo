#![forbid(unsafe_code)]

//! connections.yml loader: named backend connections (prometheus/datadog).

use serde::Deserialize;

/// Top-level connections file.
#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ConnectionsFile {
    #[serde(default)]
    pub connections: Vec<Connection>,
}

/// One named connection.
#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Connection {
    pub name: String,
    /// `prometheus` or `datadog`.
    pub backend: String,
    /// Prometheus: directory of generated rules files.
    #[serde(default)]
    pub out_dir: Option<String>,
    /// Prometheus: base URL for `/-/reload` (skip reload when absent).
    #[serde(default)]
    pub reload_url: Option<String>,
    /// Datadog: API base URL (real site or datadog-mock).
    #[serde(default)]
    pub endpoint: Option<String>,
    /// Datadog: API key reference (env var or file). Never inline secrets.
    #[serde(default)]
    pub api_key: Option<ApiKeyRef>,
}

/// API key reference: exactly one of `env` / `file`.
#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ApiKeyRef {
    #[serde(default)]
    pub env: Option<String>,
    #[serde(default)]
    pub file: Option<String>,
}

/// Load + minimally validate connections.
pub fn load(path: &std::path::Path) -> anyhow::Result<Vec<Connection>> {
    let text = std::fs::read_to_string(path)
        .map_err(|e| anyhow::anyhow!("cannot read {}: {e}", path.display()))?;
    let file: ConnectionsFile = serde_yaml_ng::from_str(&text)
        .map_err(|e| anyhow::anyhow!("{}: invalid YAML: {e}", path.display()))?;
    for conn in &file.connections {
        match conn.backend.as_str() {
            "prometheus" => {
                if conn.out_dir.as_deref().unwrap_or("").is_empty() {
                    anyhow::bail!("connection {:?}: prometheus needs out_dir", conn.name);
                }
            }
            "datadog" => {
                if conn.endpoint.as_deref().unwrap_or("").is_empty() {
                    anyhow::bail!("connection {:?}: datadog needs endpoint", conn.name);
                }
                match &conn.api_key {
                    Some(ApiKeyRef {
                        env: Some(_),
                        file: None,
                    })
                    | Some(ApiKeyRef {
                        env: None,
                        file: Some(_),
                    }) => {}
                    _ => anyhow::bail!(
                        "connection {:?}: datadog needs api_key with exactly one of env/file",
                        conn.name
                    ),
                }
            }
            other => anyhow::bail!("connection {:?}: unknown backend {other:?}", conn.name),
        }
    }
    Ok(file.connections)
}

/// Resolve an API key reference (env var value or trimmed file content).
pub fn resolve_api_key(reference: &ApiKeyRef) -> anyhow::Result<String> {
    if let Some(name) = reference.env.as_deref() {
        return std::env::var(name)
            .map_err(|_| anyhow::anyhow!("env {name} is not set (secret_ref)"));
    }
    if let Some(path) = reference.file.as_deref() {
        return Ok(std::fs::read_to_string(path)
            .map_err(|e| anyhow::anyhow!("cannot read secret file {path}: {e}"))?
            .trim()
            .to_owned());
    }
    anyhow::bail!("api_key needs exactly one of env/file")
}
