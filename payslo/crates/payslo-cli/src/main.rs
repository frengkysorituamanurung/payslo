#![forbid(unsafe_code)]

//! `payslo` CLI: validate | render | plan | apply | drift | capabilities.
//!
//! plan/apply/drift work per connection (connections.yml) with an
//! apply-state store (rusqlite): second apply is a no-op, manual backend
//! edits surface as drift.

use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

use clap::{Parser, Subcommand, ValueEnum};
use payslo_adapter::SloBackend;
use payslo_adapter::connections::{Connection, resolve_api_key};
use payslo_adapter::store::{StateStore, content_hash};

#[derive(Debug, Parser)]
#[command(name = "payslo", about = "PaySLO: SLO-as-code for the lab")]
struct Cli {
    #[command(subcommand)]
    command: Command,
}

#[derive(Debug, Subcommand)]
enum Command {
    /// Validate SLO file(s) against the schema + code rules.
    Validate {
        /// SLO file or directory of *.yml.
        path: PathBuf,
        /// Workspace root used to resolve runbook paths.
        #[arg(long, default_value = "payslo-workspace")]
        workspace: PathBuf,
    },
    /// Render backend rules to stdout or a directory.
    Render {
        /// SLO file or directory of *.yml.
        path: PathBuf,
        #[arg(long, default_value = "payslo-workspace")]
        workspace: PathBuf,
        #[arg(long, value_enum, default_value = "prometheus")]
        backend: Backend,
        /// Output directory (default: stdout, first SLO only).
        #[arg(long)]
        out: Option<PathBuf>,
        #[arg(long, value_enum, default_value = "native")]
        format: RenderFormat,
    },
    /// Show what would change on each connection (diff vs actual state).
    Plan {
        path: PathBuf,
        #[arg(long, default_value = "payslo-workspace")]
        workspace: PathBuf,
        #[arg(long)]
        connection: Vec<String>,
        #[arg(long, default_value = "payslo-workspace/connections.yml")]
        connections: PathBuf,
        #[arg(long, default_value = "payslo-workspace/state.db")]
        state: PathBuf,
    },
    /// Apply SLOs to each connection (idempotent; second apply = no-op).
    Apply {
        path: PathBuf,
        #[arg(long, default_value = "payslo-workspace")]
        workspace: PathBuf,
        #[arg(long)]
        connection: Vec<String>,
        #[arg(long, default_value = "payslo-workspace/connections.yml")]
        connections: PathBuf,
        #[arg(long, default_value = "payslo-workspace/state.db")]
        state: PathBuf,
    },
    /// Compare stored apply hashes with live backend state.
    Drift {
        #[arg(long, default_value = "payslo-workspace")]
        workspace: PathBuf,
        #[arg(long)]
        connection: Vec<String>,
        #[arg(long, default_value = "payslo-workspace/connections.yml")]
        connections: PathBuf,
        #[arg(long, default_value = "payslo-workspace/state.db")]
        state: PathBuf,
    },
    /// List backend capabilities (+ matrix).
    Capabilities {
        #[arg(long, value_enum)]
        backend: Option<Backend>,
    },
    /// Run the incident server (webhooks + responders; UI in FASE 5b).
    Serve {},
}

#[derive(Debug, Clone, Copy, ValueEnum)]
enum Backend {
    Prometheus,
    Datadog,
}

#[derive(Debug, Clone, Copy, ValueEnum)]
enum RenderFormat {
    Native,
    Crd,
}

fn load_one(
    path: &std::path::Path,
    workspace: &std::path::Path,
) -> anyhow::Result<payslo_model::SloDocument> {
    Ok(payslo_model::load_file(path, workspace)?)
}

fn load_all(
    path: &std::path::Path,
    workspace: &std::path::Path,
) -> anyhow::Result<Vec<(PathBuf, payslo_model::SloDocument)>> {
    if path.is_dir() {
        let map = payslo_model::load_dir(path, workspace)?;
        Ok(map.into_values().collect())
    } else {
        let doc = load_one(path, workspace)?;
        Ok(vec![(path.to_path_buf(), doc)])
    }
}

fn select_connections(all: &[Connection], wanted: &[String]) -> anyhow::Result<Vec<Connection>> {
    if wanted.is_empty() {
        return Ok(all.to_vec());
    }
    let mut out = Vec::new();
    for name in wanted {
        match all.iter().find(|c| &c.name == name) {
            Some(conn) => out.push(conn.clone()),
            None => anyhow::bail!("unknown connection {name:?}"),
        }
    }
    Ok(out)
}

enum ActiveBackend {
    Prometheus(payslo_adapter_prometheus::PrometheusBackend),
    Datadog(payslo_adapter_datadog::DatadogBackend),
}

impl ActiveBackend {
    fn name(&self) -> &'static str {
        match self {
            Self::Prometheus(_) => "prometheus",
            Self::Datadog(_) => "datadog",
        }
    }

    fn warnings(&self, slo: &payslo_model::SloDocument) -> Vec<String> {
        match self {
            Self::Prometheus(b) => b.warnings(slo),
            Self::Datadog(b) => b.warnings(slo),
        }
    }
}

fn build_backend(conn: &Connection) -> anyhow::Result<ActiveBackend> {
    match conn.backend.as_str() {
        "prometheus" => {
            let out = conn.out_dir.clone().unwrap_or_default();
            let reload = conn.reload_url.clone();
            // out_dir is resolved by the caller for file IO; the backend only
            // needs it for trait apply/delete signatures.
            let _ = out;
            Ok(ActiveBackend::Prometheus(
                payslo_adapter_prometheus::PrometheusBackend::new(reload),
            ))
        }
        "datadog" => {
            let endpoint = conn.endpoint.clone().unwrap_or_default();
            let api_key = resolve_api_key(
                conn.api_key
                    .as_ref()
                    .ok_or_else(|| anyhow::anyhow!("datadog connection needs api_key"))?,
            )?;
            Ok(ActiveBackend::Datadog(
                payslo_adapter_datadog::DatadogBackend::new(
                    payslo_adapter_datadog::DatadogConfig { endpoint, api_key },
                ),
            ))
        }
        other => anyhow::bail!("unknown backend {other:?}"),
    }
}

/// Desired content hash + external id for (slo, connection).
/// For Prometheus the unit is the rendered file; for Datadog the resolved
/// canonical JSON (real SLO id substituted when known from the store).
async fn desired_state(
    backend: &ActiveBackend,
    conn: &Connection,
    slo: &payslo_model::SloDocument,
    store: &StateStore,
) -> anyhow::Result<(String, Option<String>)> {
    match backend {
        ActiveBackend::Prometheus(b) => {
            let files = b.render(slo)?;
            let content = files.first().map(|f| f.content.clone()).unwrap_or_default();
            let file_name = format!("{}.yml", slo.name);
            Ok((content_hash(&content), Some(file_name)))
        }
        ActiveBackend::Datadog(b) => {
            let external = store
                .get(&slo.name, &conn.name)
                .map_err(|e| anyhow::anyhow!("{e}"))?
                .map(|r| r.external_id);
            let content = match external.as_deref() {
                Some(id) => b.resolved_canonical(slo, id)?,
                None => b.render_canonical(slo)?,
            };
            Ok((content_hash(&content), external))
        }
    }
}

/// Actual backend content hash (None = absent). Compares like-for-like with
/// [`desired_state`] (resolved form, real ids).
async fn actual_state(
    backend: &ActiveBackend,
    conn: &Connection,
    slo: &payslo_model::SloDocument,
    store: &StateStore,
) -> anyhow::Result<Option<String>> {
    match backend {
        ActiveBackend::Prometheus(_) => {
            let out = conn.out_dir.clone().unwrap_or_default();
            let path = Path::new(&out).join(format!("{}.yml", slo.name));
            match std::fs::read_to_string(&path) {
                Ok(text) => Ok(Some(content_hash(&text))),
                Err(_) => Ok(None),
            }
        }
        ActiveBackend::Datadog(b) => {
            let external = store
                .get(&slo.name, &conn.name)
                .map_err(|e| anyhow::anyhow!("{e}"))?
                .map(|r| r.external_id);
            match b.fetch_resolved(slo, external.as_deref()).await? {
                Some(content) => Ok(Some(content_hash(&content))),
                None => Ok(None),
            }
        }
    }
}

fn print_warnings(backend: &ActiveBackend, slo: &payslo_model::SloDocument) {
    for warning in backend.warnings(slo) {
        println!("warning [{}:{}]: {warning}", slo.name, backend.name());
    }
}

#[tokio::main]
async fn main() {
    if let Err(e) = run().await {
        eprintln!("payslo: error: {e:#}");
        std::process::exit(1);
    }
}

async fn run() -> anyhow::Result<()> {
    let cli = Cli::parse();
    match cli.command {
        Command::Validate { path, workspace } => {
            let slos = load_all(&path, &workspace)?;
            let backend = payslo_adapter_prometheus::PrometheusBackend::new(None);
            for (file, doc) in &slos {
                backend
                    .validate(doc)
                    .map_err(|e| anyhow::anyhow!("{}: {e}", doc.name))?;
                println!("ok: {} ({})", doc.name, file.display());
            }
            println!("{} SLO(s) valid", slos.len());
        }
        Command::Render {
            path,
            workspace,
            backend,
            out,
            format,
        } => {
            if !matches!(backend, Backend::Prometheus) {
                anyhow::bail!(
                    "render --backend datadog is not file-based; use plan/apply with a connection"
                );
            }
            let slos = load_all(&path, &workspace)?;
            let be = payslo_adapter_prometheus::PrometheusBackend::new(None);
            match out {
                Some(dir) => {
                    std::fs::create_dir_all(&dir)?;
                    for (_, doc) in &slos {
                        let name = &doc.name;
                        let content = match format {
                            RenderFormat::Native => be.render_native_text(doc)?,
                            RenderFormat::Crd => be.render_crd_text(doc)?,
                        };
                        let file_name = match format {
                            RenderFormat::Native => format!("{name}.yml"),
                            RenderFormat::Crd => format!("{name}-crd.yml"),
                        };
                        std::fs::write(dir.join(&file_name), &content)?;
                        println!("wrote {}", dir.join(&file_name).display());
                    }
                }
                None => {
                    let (_, doc) = slos
                        .first()
                        .ok_or_else(|| anyhow::anyhow!("no SLOs found"))?;
                    let content = match format {
                        RenderFormat::Native => be.render_native_text(doc)?,
                        RenderFormat::Crd => be.render_crd_text(doc)?,
                    };
                    print!("{content}");
                }
            }
        }
        Command::Plan {
            path,
            workspace,
            connection,
            connections,
            state,
        } => {
            let slos = load_all(&path, &workspace)?;
            let conns = select_connections(
                &payslo_adapter::connections::load(&connections)?,
                &connection,
            )?;
            let store = StateStore::open(&state).map_err(|e| anyhow::anyhow!("{e}"))?;
            let mut changes = 0;
            for (_, doc) in &slos {
                for conn in &conns {
                    let backend = build_backend(conn)?;
                    print_warnings(&backend, doc);
                    let (desired, _) = desired_state(&backend, conn, doc, &store).await?;
                    let actual = actual_state(&backend, conn, doc, &store).await?;
                    if actual.as_deref() != Some(desired.as_str()) {
                        println!("would update {} on {}", doc.name, conn.name);
                        changes += 1;
                    }
                }
            }
            println!("{changes} change(s) planned");
        }
        Command::Apply {
            path,
            workspace,
            connection,
            connections,
            state,
        } => {
            let slos = load_all(&path, &workspace)?;
            let conns = select_connections(
                &payslo_adapter::connections::load(&connections)?,
                &connection,
            )?;
            let store = StateStore::open(&state).map_err(|e| anyhow::anyhow!("{e}"))?;
            for (_, doc) in &slos {
                for conn in &conns {
                    let backend = build_backend(conn)?;
                    print_warnings(&backend, doc);
                    let (desired, _) = desired_state(&backend, conn, doc, &store).await?;
                    let actual = actual_state(&backend, conn, doc, &store).await?;
                    if actual.as_deref() == Some(desired.as_str()) {
                        println!("up-to-date {} on {} (no changes)", doc.name, conn.name);
                        continue;
                    }
                    match &backend {
                        ActiveBackend::Prometheus(b) => {
                            let out = conn.out_dir.clone().unwrap_or_default();
                            std::fs::create_dir_all(&out)?;
                            b.apply(doc, Path::new(&out)).await?;
                            let file_name = format!("{}.yml", doc.name);
                            store
                                .record(&doc.name, &conn.name, &desired, &file_name)
                                .map_err(|e| anyhow::anyhow!("{e}"))?;
                        }
                        ActiveBackend::Datadog(b) => {
                            let external = store
                                .get(&doc.name, &conn.name)
                                .map_err(|e| anyhow::anyhow!("{e}"))?
                                .map(|r| r.external_id);
                            let (slo_id, resolved) =
                                b.apply_remote(doc, external.as_deref()).await?;
                            store
                                .record(&doc.name, &conn.name, &content_hash(&resolved), &slo_id)
                                .map_err(|e| anyhow::anyhow!("{e}"))?;
                        }
                    }
                    println!("applied {} on {}", doc.name, conn.name);
                }
            }
        }
        Command::Drift {
            workspace,
            connection,
            connections,
            state,
        } => {
            let conns = select_connections(
                &payslo_adapter::connections::load(&connections)?,
                &connection,
            )?;
            let store = StateStore::open(&state).map_err(|e| anyhow::anyhow!("{e}"))?;
            let slo_dir = workspace.join("slo");
            let slos = load_all(&slo_dir, &workspace)?;
            let docs: BTreeMap<&str, &payslo_model::SloDocument> =
                slos.iter().map(|(_, d)| (d.name.as_str(), d)).collect();
            let mut drifted = 0;
            for record in store.list().map_err(|e| anyhow::anyhow!("{e}"))? {
                let Some(conn) = conns.iter().find(|c| c.name == record.connection) else {
                    continue;
                };
                let Some(doc) = docs.get(record.slo.as_str()) else {
                    println!(
                        "drift: {} on {} (SLO file removed)",
                        record.slo, record.connection
                    );
                    drifted += 1;
                    continue;
                };
                let backend = build_backend(conn)?;
                let actual = actual_state(&backend, conn, doc, &store).await?;
                match actual {
                    Some(hash) if hash == record.content_hash => {}
                    Some(_) => {
                        println!(
                            "drift: {} on {} (backend differs from last apply)",
                            record.slo, record.connection
                        );
                        drifted += 1;
                    }
                    None => {
                        println!(
                            "drift: {} on {} (object missing in backend)",
                            record.slo, record.connection
                        );
                        drifted += 1;
                    }
                }
            }
            if drifted == 0 {
                println!("no drift");
            } else {
                println!("{drifted} drifted record(s)");
            }
        }
        Command::Capabilities { backend } => {
            print_matrix(backend);
        }
        Command::Serve {} => {
            payslo_server::serve::serve().await?;
        }
    }
    Ok(())
}

fn print_matrix(only: Option<Backend>) {
    println!("backend capabilities (see payslo/CAPABILITIES.md):");
    println!("| feature | prometheus | datadog |");
    println!("|---|---|---|");
    for row in payslo_adapter::CAPABILITY_MATRIX {
        if let Some(Backend::Prometheus) = only {
            println!("| {} | {} | n/a |", row.feature, row.prometheus);
        } else if let Some(Backend::Datadog) = only {
            println!("| {} | n/a | {} |", row.feature, row.datadog);
        } else {
            println!("| {} | {} | {} |", row.feature, row.prometheus, row.datadog);
        }
    }
}
