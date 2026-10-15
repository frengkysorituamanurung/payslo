#![forbid(unsafe_code)]

//! Session state (JSON file): one active run per scenario with start time,
//! hints used, answers, and trainee actions recorded by lab commands.

use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};

/// One trainee action recorded by lab.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SessionAction {
    /// Unix seconds.
    pub ts: i64,
    /// Short command tag, e.g. `routing`, `chaos`, `answer`, `deploy`.
    pub cmd: String,
    /// Free detail (provider, target...).
    #[serde(default)]
    pub detail: String,
}

/// Active scenario run.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Session {
    pub scenario_id: String,
    pub started_at: i64,
    #[serde(default)]
    pub ended_at: Option<i64>,
    #[serde(default)]
    pub hints_used: Vec<String>,
    #[serde(default)]
    pub answers: BTreeMap<String, String>,
    #[serde(default)]
    pub actions: Vec<SessionAction>,
}

impl Session {
    #[must_use]
    pub fn new(scenario_id: &str, now: i64) -> Self {
        Self {
            scenario_id: scenario_id.to_owned(),
            started_at: now,
            ended_at: None,
            hints_used: Vec::new(),
            answers: BTreeMap::new(),
            actions: Vec::new(),
        }
    }

    pub fn record(&mut self, now: i64, cmd: &str, detail: &str) {
        self.actions.push(SessionAction {
            ts: now,
            cmd: cmd.to_owned(),
            detail: detail.to_owned(),
        });
    }

    /// Elapsed seconds from start to first action (None = no action yet).
    #[must_use]
    pub fn first_action_after(&self) -> Option<i64> {
        self.actions
            .iter()
            .map(|a| a.ts)
            .min()
            .map(|t| t - self.started_at)
    }
}

/// Session file location: `<root>/payslo-workspace/lab-sessions/<id>.json`.
pub fn session_path(root: &std::path::Path, id: &str) -> std::path::PathBuf {
    root.join("payslo-workspace")
        .join("lab-sessions")
        .join(format!("{id}.json"))
}

pub fn load(root: &std::path::Path, id: &str) -> anyhow::Result<Session> {
    let path = session_path(root, id);
    let text = std::fs::read_to_string(&path).map_err(|e| {
        anyhow::anyhow!(
            "no active session for {id} ({}); run `lab scenario start {id}` first",
            e
        )
    })?;
    Ok(serde_json::from_str(&text)?)
}

pub fn save(root: &std::path::Path, session: &Session) -> anyhow::Result<()> {
    let path = session_path(root, &session.scenario_id);
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent)?;
    }
    // Atomic write: temp + rename.
    let tmp = path.with_extension("json.tmp");
    std::fs::write(&tmp, serde_json::to_string_pretty(session)?)?;
    std::fs::rename(&tmp, &path)?;
    Ok(())
}

pub fn now_unix() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}
