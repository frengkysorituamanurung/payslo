#![forbid(unsafe_code)]

//! `lab` CLI: compose lifecycle, chaos, routing, scenarios (FASE 6).
//! Docker/Compose accessed as subprocesses with clear errors when missing.

mod docker;
mod eval;
mod model;
mod session;

use std::path::PathBuf;

use clap::{Parser, Subcommand, ValueEnum};

use docker::{Endpoints, check_docker, compose, compose_exec};

#[derive(Debug, Parser)]
#[command(name = "lab", about = "PaySLO Lab CLI", arg_required_else_help = true)]
struct Cli {
    /// Repo root (must contain docker-compose.yml).
    #[arg(long, global = true)]
    root: Option<PathBuf>,
    #[command(subcommand)]
    command: Command,
}

#[derive(Debug, Subcommand)]
enum Command {
    /// Nyalakan stack (default+traffic+obs+payslo).
    Up {},
    /// Matikan stack.
    Down {},
    /// Reset total (down -v + up).
    Reset {},
    /// Status container.
    Status {},
    /// Tampilkan URL lab.
    Open {},
    /// Injeksikan kegagalan (atau status|reset).
    Chaos {
        target: ChaosTarget,
        /// Detik untuk chaos db (default 300).
        #[arg(long, default_value = "300")]
        secs: u64,
    },
    /// Atur laju loadgen.
    Traffic {
        #[command(subcommand)]
        action: TrafficAction,
    },
    /// Ubah routing provider.
    Routing {
        state: RoutingState,
        provider: String,
    },
    /// Ganti versi gateway (good|bad) + anotasi Grafana (best-effort).
    Deploy {
        #[arg(value_enum)]
        target: DeployTarget,
        #[arg(long, value_enum, default_value = "good")]
        version: ReleaseVersion,
    },
    /// Kelola skenario.
    Scenario {
        #[command(subcommand)]
        action: ScenarioAction,
    },
    /// Tampilkan hint berikutnya untuk skenario aktif.
    Hint { id: String },
    /// Jawab pertanyaan skenario.
    Answer { question: String, text: String },
    /// Nilai skenario (skor + PASS/FAIL).
    Verify { id: String },
    /// Cetak shell completion.
    Completion {
        #[arg(value_enum)]
        shell: CompletionShell,
    },
    /// Mode mentor: lihat timeline trainee, injeksikan gangguan.
    Mentor {
        #[command(subcommand)]
        action: MentorAction,
    },
    /// Snapshot/restore state lab (DB + profil chaos).
    Snapshot {
        #[command(subcommand)]
        action: SnapshotAction,
    },
}

#[derive(Debug, Clone, Copy, ValueEnum)]
enum ChaosTarget {
    Bca,
    Mandiri,
    Ewallet,
    Db,
    Merchant,
    Status,
    Reset,
}

#[derive(Debug, Subcommand)]
enum TrafficAction {
    /// Set laju req/detik loadgen.
    Set {
        #[arg(long)]
        rate: u64,
    },
}

#[derive(Debug, Clone, Copy, ValueEnum)]
enum RoutingState {
    Enable,
    Disable,
}

#[derive(Debug, Clone, Copy, ValueEnum)]
enum DeployTarget {
    Gateway,
}

#[derive(Debug, Clone, Copy, ValueEnum)]
enum ReleaseVersion {
    Good,
    Bad,
}

#[derive(Debug, Subcommand)]
enum ScenarioAction {
    Start {
        id: String,
        #[arg(long)]
        force: bool,
    },
    Stop {
        id: String,
    },
    List {},
    Solution {
        id: String,
    },
    Random {
        #[arg(long)]
        seed: Option<u64>,
    },
}

#[derive(Debug, Clone, Copy, ValueEnum)]
enum CompletionShell {
    Bash,
    Zsh,
    Fish,
}

#[derive(Debug, Subcommand)]
enum SnapshotAction {
    /// Simpan snapshot DB + profil chaos.
    Save { name: String },
    /// Muat snapshot (timpa DB + profil).
    Load { name: String },
}

#[derive(Debug, Subcommand)]
enum MentorAction {
    /// Tampilkan timeline aksi trainee (sesi + audit payslo).
    Timeline {
        #[arg(default_value = "")]
        id: String,
    },
    /// Injeksikan gangguan di tengah sesi (traffic/chaos).
    Inject {
        #[command(subcommand)]
        what: InjectWhat,
    },
}

#[derive(Debug, Subcommand)]
enum InjectWhat {
    /// Ubah laju loadgen, opsional kembali normal setelah N detik.
    Traffic {
        #[arg(long)]
        rate: u64,
        #[arg(long)]
        secs: Option<u64>,
    },
}

fn repo_root(explicit: Option<PathBuf>) -> anyhow::Result<PathBuf> {
    let root = explicit
        .or_else(|| std::env::var("LAB_ROOT").ok().map(PathBuf::from))
        .unwrap_or_else(|| std::env::current_dir().unwrap_or_else(|_| PathBuf::from(".")));
    if !root.join("docker-compose.yml").exists() {
        anyhow::bail!(
            "docker-compose.yml tidak ditemukan di {} (jalankan dari repo root atau --root)",
            root.display()
        );
    }
    Ok(root)
}

fn http_client() -> reqwest::Client {
    reqwest::Client::new()
}

#[tokio::main]
async fn main() {
    if let Err(e) = run().await {
        eprintln!("lab: error: {e:#}");
        std::process::exit(1);
    }
}

async fn run() -> anyhow::Result<()> {
    let cli = Cli::parse();
    // Completion works without Docker/repo checks.
    if let Command::Completion { shell } = cli.command {
        return print_completion(shell);
    }
    let root = repo_root(cli.root)?;
    let endpoints = Endpoints::from_env(root.clone());
    let http = http_client();
    match cli.command {
        Command::Up {} => {
            check_docker()?;
            let out = compose(
                &root,
                &[
                    "--profile",
                    "traffic",
                    "--profile",
                    "obs",
                    "--profile",
                    "payslo",
                    "up",
                    "-d",
                    "--build",
                ],
                &[],
            )?;
            if !out.status.success() {
                anyhow::bail!("{}", String::from_utf8_lossy(&out.stderr));
            }
            println!("stack up (default+traffic+obs+payslo)");
            Ok(())
        }
        Command::Down {} => {
            check_docker()?;
            let out = compose(
                &root,
                &[
                    "--profile",
                    "traffic",
                    "--profile",
                    "obs",
                    "--profile",
                    "obs-full",
                    "--profile",
                    "payslo",
                    "down",
                ],
                &[],
            )?;
            if !out.status.success() {
                anyhow::bail!("{}", String::from_utf8_lossy(&out.stderr));
            }
            println!("stack down");
            Ok(())
        }
        Command::Reset {} => {
            check_docker()?;
            for args in [
                vec![
                    "--profile",
                    "traffic",
                    "--profile",
                    "obs",
                    "--profile",
                    "obs-full",
                    "--profile",
                    "payslo",
                    "down",
                    "-v",
                ],
                vec![
                    "--profile",
                    "traffic",
                    "--profile",
                    "obs",
                    "--profile",
                    "payslo",
                    "up",
                    "-d",
                    "--build",
                ],
            ] {
                let out = compose(&root, &args, &[])?;
                if !out.status.success() {
                    anyhow::bail!("{}", String::from_utf8_lossy(&out.stderr));
                }
            }
            println!("stack reset");
            Ok(())
        }
        Command::Status {} => {
            check_docker()?;
            let out = compose(
                &root,
                &[
                    "--profile",
                    "traffic",
                    "--profile",
                    "obs",
                    "--profile",
                    "obs-full",
                    "--profile",
                    "payslo",
                    "ps",
                ],
                &[],
            )?;
            print!("{}", String::from_utf8_lossy(&out.stdout));
            Ok(())
        }
        Command::Open {} => {
            println!(
                "gateway:  http://127.0.0.1:3000/healthz   admin: http://127.0.0.1:3001 (loopback-only)"
            );
            println!("envoy:    http://127.0.0.1:8080   admin: http://127.0.0.1:9901");
            println!("merchant: http://127.0.0.1:5000/state");
            println!("prom:     http://127.0.0.1:9090   alertmanager: http://127.0.0.1:9093");
            println!("grafana:  http://127.0.0.1:3002");
            println!("payslo:   http://127.0.0.1:8081/incidents   ddmock: http://127.0.0.1:8126");
            println!("ntfy:     http://127.0.0.1:8022/payslo-lab");
            Ok(())
        }
        Command::Chaos { target, secs } => chaos(&root, &endpoints, &http, target, secs).await,
        Command::Traffic { action } => match action {
            TrafficAction::Set { rate } => {
                check_docker()?;
                let rate_str = rate.to_string();
                let out = compose(
                    &root,
                    &["--profile", "traffic", "up", "-d", "loadgen"],
                    &[("RATE_PER_SEC", rate_str.as_str())],
                )?;
                if !out.status.success() {
                    anyhow::bail!("{}", String::from_utf8_lossy(&out.stderr));
                }
                println!("traffic rate={rate}/s");
                record_session_action(&root, "traffic", &format!("rate={rate}"));
                Ok(())
            }
        },
        Command::Routing { state, provider } => {
            let enabled = matches!(state, RoutingState::Enable);
            set_routing(&http, &endpoints, &provider, enabled).await?;
            println!("routing {provider} enabled={enabled}");
            record_session_action(&root, "routing", &format!("{provider}={enabled}"));
            Ok(())
        }
        Command::Deploy { target, version } => {
            let DeployTarget::Gateway = target;
            let version_str = match version {
                ReleaseVersion::Good => "good",
                ReleaseVersion::Bad => "bad",
            };
            http.put(format!("{}/admin/release", endpoints.gateway_admin))
                .json(&serde_json::json!({"version": version_str}))
                .send()
                .await?
                .error_for_status()?;
            println!("gateway version={version_str}");
            annotate_deploy(&http, version_str).await;
            record_session_action(&root, "deploy", &format!("gateway={version_str}"));
            Ok(())
        }
        Command::Scenario { action } => scenario_cmd(&root, &endpoints, &http, action).await,
        Command::Mentor { action } => mentor_cmd(&root, &endpoints, &http, action).await,
        Command::Snapshot { action } => snapshot_cmd(&root, &endpoints, &http, action).await,
        Command::Hint { id } => hint(&root, &id),
        Command::Answer { question, text } => answer(&root, &question, &text),
        Command::Verify { id } => verify(&root, &endpoints, &http, &id).await,
        Command::Completion { .. } => unreachable!(),
    }
}

async fn mentor_cmd(
    root: &std::path::Path,
    endpoints: &Endpoints,
    http: &reqwest::Client,
    action: MentorAction,
) -> anyhow::Result<()> {
    match action {
        MentorAction::Timeline { id } => {
            if id.is_empty() {
                // All sessions, newest first.
                let dir = root.join("payslo-workspace").join("lab-sessions");
                let mut files: Vec<String> = std::fs::read_dir(&dir)
                    .map(|rd| {
                        rd.filter_map(|e| {
                            e.ok().and_then(|e| {
                                e.path()
                                    .file_stem()
                                    .and_then(|s| s.to_str())
                                    .map(str::to_owned)
                            })
                        })
                        .collect()
                    })
                    .unwrap_or_default();
                files.sort();
                for sid in files {
                    print_session_timeline(root, &sid)?;
                }
                return Ok(());
            }
            print_session_timeline(root, &id)?;
            // Plus payslo incidents for the scenario (server-side view).
            let scenario = model::load(&scenario_dir(root, &id).join("scenario.yml"))?;
            let slo = scenario.checks.iter().find_map(|c| match &c.check {
                model::Check::IncidentExists { slo, .. } => slo.clone(),
                _ => None,
            });
            let mut url = format!("{}/incidents", endpoints.payslo);
            if let Some(slo) = slo {
                url.push_str(&format!("?slo={slo}"));
            }
            match http.get(url).send().await {
                Ok(resp) => match resp.json::<serde_json::Value>().await {
                    Ok(body) => {
                        let items = body
                            .pointer("/incidents")
                            .and_then(serde_json::Value::as_array)
                            .cloned()
                            .unwrap_or_default();
                        println!("--- payslo incidents ({}) ---", items.len());
                        for item in items {
                            println!(
                                "{} | {} | {} | opened {} | resolved {}",
                                item.pointer("/id")
                                    .and_then(serde_json::Value::as_str)
                                    .unwrap_or("?"),
                                item.pointer("/status")
                                    .and_then(serde_json::Value::as_str)
                                    .unwrap_or("?"),
                                item.pointer("/title")
                                    .and_then(serde_json::Value::as_str)
                                    .unwrap_or("?"),
                                item.pointer("/created_at")
                                    .and_then(serde_json::Value::as_str)
                                    .unwrap_or("?"),
                                item.pointer("/resolved_at")
                                    .and_then(serde_json::Value::as_str)
                                    .unwrap_or("-"),
                            );
                        }
                    }
                    Err(e) => println!("warning: cannot parse payslo incidents: {e}"),
                },
                Err(e) => println!("warning: payslo unreachable ({e}); is the payslo profile up?"),
            }
            Ok(())
        }
        MentorAction::Inject { what } => match what {
            InjectWhat::Traffic { rate, secs } => {
                check_docker()?;
                let rate_str = rate.to_string();
                let out = compose(
                    root,
                    &["--profile", "traffic", "up", "-d", "loadgen"],
                    &[("RATE_PER_SEC", rate_str.as_str())],
                )?;
                if !out.status.success() {
                    anyhow::bail!("{}", String::from_utf8_lossy(&out.stderr));
                }
                println!("mentor inject: traffic rate={rate}/s");
                if let Some(secs) = secs {
                    println!("reverting to 2/s in {secs}s ...");
                    tokio::time::sleep(std::time::Duration::from_secs(secs)).await;
                    let out = compose(
                        root,
                        &["--profile", "traffic", "up", "-d", "loadgen"],
                        &[("RATE_PER_SEC", "2")],
                    )?;
                    if !out.status.success() {
                        anyhow::bail!("{}", String::from_utf8_lossy(&out.stderr));
                    }
                    println!("mentor inject: traffic back to 2/s");
                }
                Ok(())
            }
        },
    }
}

fn print_session_timeline(root: &std::path::Path, id: &str) -> anyhow::Result<()> {
    let session = session::load(root, id)?;
    println!("=== timeline {id} (started {}) ===", session.started_at);
    if !session.hints_used.is_empty() {
        println!("hints: {}", session.hints_used.join(", "));
    }
    for (qid, text) in &session.answers {
        let preview: String = text.chars().take(80).collect();
        println!("answer[{qid}]: {preview}");
    }
    for action in &session.actions {
        println!(
            "t+{}s {} {}",
            action.ts - session.started_at,
            action.cmd,
            action.detail
        );
    }
    if let Some(ended) = session.ended_at {
        println!("ended after {}s", ended - session.started_at);
    }
    Ok(())
}

fn snapshot_dir(root: &std::path::Path, name: &str) -> PathBuf {
    root.join("snapshots").join(name)
}

async fn snapshot_cmd(
    root: &std::path::Path,
    endpoints: &Endpoints,
    http: &reqwest::Client,
    action: SnapshotAction,
) -> anyhow::Result<()> {
    match action {
        SnapshotAction::Save { name } => {
            check_docker()?;
            let dir = snapshot_dir(root, &name);
            std::fs::create_dir_all(&dir)?;
            // Postgres dump (data only, minus migration bookkeeping).
            let dump = compose_exec(
                root,
                "postgres",
                &[
                    "pg_dump",
                    "-U",
                    "lab_app",
                    "-d",
                    "payslo_lab",
                    "--data-only",
                    "-T",
                    "_sqlx_migrations",
                ],
            )?;
            std::fs::write(dir.join("db.sql"), &dump)?;
            // Chaos profiles (banks, merchant, routing, release).
            let mut chaos = serde_json::json!({});
            for (provider, base) in [
                ("bca", &endpoints.bank_bca),
                ("mandiri", &endpoints.bank_mandiri),
                ("ewallet", &endpoints.bank_ewallet),
            ] {
                let profile: serde_json::Value = http
                    .get(format!("{base}/admin/profile"))
                    .send()
                    .await?
                    .json()
                    .await?;
                chaos[provider] = profile;
            }
            chaos["merchant_mode"] = http
                .get(format!("{}/admin/mode", endpoints.merchant))
                .send()
                .await?
                .json()
                .await?;
            chaos["routing"] = http
                .get(format!("{}/admin/routing", endpoints.gateway_admin))
                .send()
                .await?
                .json()
                .await?;
            std::fs::write(
                dir.join("chaos.json"),
                serde_json::to_string_pretty(&chaos)?,
            )?;
            let tables = dump
                .lines()
                .filter(|l| l.starts_with("COPY ") && l.ends_with(" FROM stdin;"))
                .count();
            println!("snapshot {name} saved (data tables: {tables})");
            Ok(())
        }
        SnapshotAction::Load { name } => {
            check_docker()?;
            let dir = snapshot_dir(root, &name);
            if !dir.join("db.sql").exists() {
                anyhow::bail!("snapshot {name:?} not found");
            }
            // Restore DB via psql stdin. Truncate lab tables first for a clean
            // slate (dump is data-only INSERTs).
            let sql = std::fs::read_to_string(dir.join("db.sql"))?;
            let mut child = std::process::Command::new("docker")
                .args([
                    "compose",
                    "exec",
                    "-T",
                    "postgres",
                    "psql",
                    "-U",
                    "lab_app",
                    "-d",
                    "payslo_lab",
                    "-v",
                    "ON_ERROR_STOP=1",
                    "-q",
                ])
                .current_dir(root)
                .stdin(std::process::Stdio::piped())
                .spawn()?;
            // Wipe lab tables, then replay (CASCADE covers FKs).
            let wipe = "TRUNCATE webhook_delivery, callback_log, transactions CASCADE;";
            let full = format!("{wipe}\n{sql}");
            std::io::Write::write_all(
                child
                    .stdin
                    .as_mut()
                    .ok_or_else(|| anyhow::anyhow!("no stdin"))?,
                full.as_bytes(),
            )?;
            let status = child.wait()?;
            if !status.success() {
                anyhow::bail!("snapshot restore failed");
            }
            // Restore chaos profiles.
            let chaos: serde_json::Value =
                serde_json::from_str(&std::fs::read_to_string(dir.join("chaos.json"))?)?;
            for (provider, base) in [
                ("bca", &endpoints.bank_bca),
                ("mandiri", &endpoints.bank_mandiri),
                ("ewallet", &endpoints.bank_ewallet),
            ] {
                if let Some(profile) = chaos.get(provider) {
                    http.put(format!("{base}/admin/profile"))
                        .json(profile)
                        .send()
                        .await?;
                }
            }
            if let Some(mode) = chaos.get("merchant_mode") {
                http.put(format!("{}/admin/mode", endpoints.merchant))
                    .json(mode)
                    .send()
                    .await?;
            }
            if let Some(routing) = chaos.get("routing") {
                http.put(format!("{}/admin/routing", endpoints.gateway_admin))
                    .json(routing)
                    .send()
                    .await?;
            }
            // Release any lab-held DB locks (snapshot is data+profiles only).
            let _ = chaos_reset_db_only(root);
            println!("snapshot {name} loaded");
            Ok(())
        }
    }
}

fn print_completion(shell: CompletionShell) -> anyhow::Result<()> {
    use clap::CommandFactory;
    use clap_complete::{generate, shells};
    let mut cmd = Cli::command();
    match shell {
        CompletionShell::Bash => generate(shells::Bash, &mut cmd, "lab", &mut std::io::stdout()),
        CompletionShell::Zsh => generate(shells::Zsh, &mut cmd, "lab", &mut std::io::stdout()),
        CompletionShell::Fish => generate(shells::Fish, &mut cmd, "lab", &mut std::io::stdout()),
    }
    Ok(())
}

/// Record an action into the active session, if any (drives TTD/safety).
fn record_session_action(root: &std::path::Path, cmd: &str, detail: &str) {
    if let Some(mut session) = active_session_mut(root) {
        let now = session::now_unix();
        session.record(now, cmd, detail);
        let _ = session::save(root, &session);
    }
}

/// Find the single active (non-ended) session, if any.
fn active_session_mut(root: &std::path::Path) -> Option<session::Session> {
    let dir = root.join("payslo-workspace").join("lab-sessions");
    let entries = std::fs::read_dir(dir).ok()?;
    for entry in entries.flatten() {
        let text = std::fs::read_to_string(entry.path()).ok()?;
        let session: session::Session = serde_json::from_str(&text).ok()?;
        if session.ended_at.is_none() {
            return Some(session);
        }
    }
    None
}

async fn set_routing(
    http: &reqwest::Client,
    endpoints: &Endpoints,
    provider: &str,
    enabled: bool,
) -> anyhow::Result<()> {
    if !["bca", "mandiri", "ewallet"].contains(&provider) {
        anyhow::bail!("provider harus salah satu: bca|mandiri|ewallet");
    }
    let routing: serde_json::Value = http
        .get(format!("{}/admin/routing", endpoints.gateway_admin))
        .send()
        .await?
        .json()
        .await?;
    let mut map = routing.as_object().cloned().unwrap_or_default();
    let weight = map
        .get(provider)
        .and_then(|p| p.get("weight"))
        .and_then(serde_json::Value::as_u64)
        .unwrap_or(50);
    map.insert(
        provider.to_owned(),
        serde_json::json!({"enabled": enabled, "weight": weight}),
    );
    http.put(format!("{}/admin/routing", endpoints.gateway_admin))
        .json(&map)
        .send()
        .await?
        .error_for_status()?;
    Ok(())
}

/// Best-effort Grafana deploy annotation (warn-only on failure).
async fn annotate_deploy(http: &reqwest::Client, version: &str) {
    let body = serde_json::json!({
        "tags": ["deploy", "gateway"],
        "text": format!("gateway deploy {version} via lab"),
    });
    match http
        .post("http://127.0.0.1:3002/api/annotations")
        .basic_auth("admin", Some("lab-only-grafana"))
        .json(&body)
        .send()
        .await
    {
        Ok(resp) if resp.status().is_success() => println!("annotated deploy on Grafana"),
        Ok(resp) => println!("warning: Grafana annotation failed: {}", resp.status()),
        Err(e) => println!("warning: Grafana annotation failed: {e}"),
    }
}

async fn chaos(
    root: &std::path::Path,
    endpoints: &Endpoints,
    http: &reqwest::Client,
    target: ChaosTarget,
    secs: u64,
) -> anyhow::Result<()> {
    match target {
        ChaosTarget::Bca | ChaosTarget::Mandiri | ChaosTarget::Ewallet => {
            let name = format!("{target:?}").to_lowercase();
            let provider = endpoints
                .bank(&name)
                .ok_or_else(|| anyhow::anyhow!("unknown bank"))?;
            http.put(format!("{provider}/admin/profile"))
                .json(&serde_json::json!({"profile": "down"}))
                .send()
                .await?
                .error_for_status()?;
            println!("chaos: bank {name} -> down");
            record_session_action(root, "chaos", &format!("bank-{name}-down"));
            Ok(())
        }
        ChaosTarget::Db => {
            check_docker()?;
            // ACCESS EXCLUSIVE lock held in background; self-releases after secs.
            let sql = format!(
                "BEGIN; LOCK TABLE transactions IN ACCESS EXCLUSIVE MODE; SELECT pg_sleep({secs}); COMMIT;"
            );
            compose_exec(
                root,
                "postgres",
                &["psql", "-U", "lab_app", "-d", "payslo_lab", "-c", &sql],
            )?;
            println!("chaos: db lock held ({secs}s, `lab chaos reset` to release early)");
            record_session_action(root, "chaos", "db-lock");
            Ok(())
        }
        ChaosTarget::Merchant => {
            http.put(format!("{}/admin/mode", endpoints.merchant))
                .json(&serde_json::json!({"status": 500}))
                .send()
                .await?
                .error_for_status()?;
            println!("chaos: merchant -> 500");
            record_session_action(root, "chaos", "merchant-500");
            Ok(())
        }
        ChaosTarget::Status => chaos_status(endpoints, http, root).await,
        ChaosTarget::Reset => chaos_reset(root, endpoints, http).await,
    }
}

async fn chaos_status(
    endpoints: &Endpoints,
    http: &reqwest::Client,
    root: &std::path::Path,
) -> anyhow::Result<()> {
    for (name, url) in [
        ("bca", &endpoints.bank_bca),
        ("mandiri", &endpoints.bank_mandiri),
        ("ewallet", &endpoints.bank_ewallet),
    ] {
        match http.get(format!("{url}/admin/profile")).send().await {
            Ok(resp) => match resp.json::<serde_json::Value>().await {
                Ok(profile) => println!(
                    "bank {name}: {}",
                    profile.get("profile").unwrap_or(&serde_json::Value::Null)
                ),
                Err(e) => println!("bank {name}: unreachable ({e})"),
            },
            Err(e) => println!("bank {name}: unreachable ({e})"),
        }
    }
    // DB lock holder present?
    match compose_exec(
        root,
        "postgres",
        &[
            "psql",
            "-U",
            "lab_app",
            "-d",
            "payslo_lab",
            "-t",
            "-c",
            "SELECT count(*) FROM pg_locks l JOIN pg_stat_activity a ON a.pid = l.pid WHERE l.locktype = 'relation' AND l.relation::regclass::text = 'transactions' AND l.mode = 'AccessExclusiveLock' AND a.query LIKE '%ACCESS EXCLUSIVE%';",
        ],
    ) {
        Ok(out) => println!("db locks held: {}", out.trim()),
        Err(e) => println!("db: unreachable ({e:#})"),
    }
    println!(
        "merchant mode: {}",
        match http
            .get(format!("{}/admin/mode", endpoints.merchant))
            .send()
            .await
        {
            Ok(resp) => match resp.json::<serde_json::Value>().await {
                Ok(mode) => format!(
                    "status={} latency_ms={}",
                    mode.get("status").unwrap_or(&serde_json::Value::Null),
                    mode.get("latency_ms").unwrap_or(&serde_json::Value::Null)
                ),
                Err(e) => format!("unreachable ({e})"),
            },
            Err(e) => format!("unreachable ({e})"),
        }
    );
    Ok(())
}

async fn chaos_reset(
    root: &std::path::Path,
    endpoints: &Endpoints,
    http: &reqwest::Client,
) -> anyhow::Result<()> {
    for url in [
        &endpoints.bank_bca,
        &endpoints.bank_mandiri,
        &endpoints.bank_ewallet,
    ] {
        http.post(format!("{url}/admin/reset")).send().await?;
    }
    http.put(format!("{}/admin/mode", endpoints.merchant))
        .json(&serde_json::json!({"status": 200}))
        .send()
        .await?;
    // Release lab-held DB locks.
    if let Ok(out) = compose_exec(
        root,
        "postgres",
        &[
            "psql",
            "-U",
            "lab_app",
            "-d",
            "payslo_lab",
            "-t",
            "-c",
            "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE query LIKE '%ACCESS EXCLUSIVE%' AND pid <> pg_backend_pid();",
        ],
    ) {
        println!("db locks released: {}", out.trim());
    }
    record_session_action(root, "chaos", "reset");
    println!("chaos reset: banks normal, merchant 200, db locks released");
    Ok(())
}

fn scenario_dir(root: &std::path::Path, id: &str) -> PathBuf {
    root.join("scenarios").join(id)
}

async fn scenario_cmd(
    root: &std::path::Path,
    endpoints: &Endpoints,
    http: &reqwest::Client,
    action: ScenarioAction,
) -> anyhow::Result<()> {
    match action {
        ScenarioAction::List {} => {
            let dir = root.join("scenarios");
            let mut entries: Vec<String> = std::fs::read_dir(&dir)
                .map_err(|e| anyhow::anyhow!("cannot list scenarios: {e}"))?
                .filter_map(|e| e.ok())
                .filter(|e| e.path().join("scenario.yml").exists())
                .filter_map(|e| e.file_name().into_string().ok())
                .collect();
            entries.sort();
            for id in entries {
                let scenario = model::load(&scenario_dir(root, &id).join("scenario.yml"))?;
                println!("{id}: {}", scenario.title);
            }
            Ok(())
        }
        ScenarioAction::Solution { id } => {
            let path = scenario_dir(root, &id).join("solution.md");
            print!(
                "{}",
                std::fs::read_to_string(&path)
                    .map_err(|e| anyhow::anyhow!("cannot read {}: {e}", path.display()))?
            );
            Ok(())
        }
        ScenarioAction::Start { id, force } => {
            // 09 is the blind dispatcher: starting it picks randomly.
            if id == "09" {
                println!("09 is a blind dispatcher: picking a random scenario ...");
                return Box::pin(scenario_cmd(
                    root,
                    endpoints,
                    http,
                    ScenarioAction::Random { seed: None },
                ))
                .await;
            }
            check_docker()?;
            if active_session_mut(root).is_some() && !force {
                anyhow::bail!(
                    "a scenario is already active (use --force to replace or `lab scenario stop`)"
                );
            }
            // Validate first (fail fast on bad definition).
            let scenario = model::load(&scenario_dir(root, &id).join("scenario.yml"))?;
            println!("=== {}: {} ===", scenario.id, scenario.title);
            println!("{}", scenario.brief);
            if !scenario.questions.is_empty() {
                println!("questions:");
                for question in &scenario.questions {
                    println!("  [{}] {}", question.id, question.prompt);
                }
            }
            // Fresh 30-minute loadgen window so metrics are live.
            match compose(root, &["--profile", "traffic", "up", "-d", "loadgen"], &[]) {
                Ok(out) if out.status.success() => {}
                _ => println!("warning: could not (re)start loadgen; metrics may be stale"),
            }
            run_setup(root, endpoints, http, &scenario).await?;
            // NOTE: start itself is NOT recorded as an action — TTD measures
            // the first trainee response after the briefing.
            let session = session::Session::new(&id, session::now_unix());
            session::save(root, &session)?;
            println!("scenario {id} started");
            Ok(())
        }
        ScenarioAction::Stop { id } => {
            let scenario = model::load(&scenario_dir(root, &id).join("scenario.yml"))?;
            run_teardown(root, endpoints, http, &scenario).await?;
            if let Ok(mut session) = session::load(root, &id) {
                session.ended_at = Some(session::now_unix());
                session::save(root, &session)?;
            }
            println!("scenario {id} stopped (teardown done)");
            Ok(())
        }
        ScenarioAction::Random { seed } => {
            let dir = root.join("scenarios");
            let mut entries: Vec<String> = std::fs::read_dir(&dir)
                .map_err(|e| anyhow::anyhow!("cannot list scenarios: {e}"))?
                .filter_map(|e| e.ok())
                .filter(|e| e.path().join("scenario.yml").exists())
                .filter_map(|e| e.file_name().into_string().ok())
                .collect();
            entries.sort();
            if entries.is_empty() {
                anyhow::bail!("no scenarios found");
            }
            let seed = seed.unwrap_or_else(|| session::now_unix() as u64);
            let pick = pick_random(&entries, seed).to_owned();
            println!("random pick (seed {seed}): {pick}");
            Box::pin(scenario_cmd(
                root,
                endpoints,
                http,
                ScenarioAction::Start {
                    id: pick,
                    force: false,
                },
            ))
            .await
        }
    }
}

/// Deterministic game-day pick (same seed => same scenario). Pure: unit-tested.
#[must_use]
pub fn pick_random(entries: &[String], seed: u64) -> &str {
    &entries[(seed as usize) % entries.len()]
}

async fn run_setup(
    root: &std::path::Path,
    endpoints: &Endpoints,
    http: &reqwest::Client,
    scenario: &model::Scenario,
) -> anyhow::Result<()> {
    for step in &scenario.setup {
        run_setup_step(root, endpoints, http, step).await?;
    }
    Ok(())
}

async fn run_teardown(
    root: &std::path::Path,
    endpoints: &Endpoints,
    http: &reqwest::Client,
    scenario: &model::Scenario,
) -> anyhow::Result<()> {
    for step in &scenario.teardown {
        run_setup_step(root, endpoints, http, step).await?;
    }
    Ok(())
}

async fn run_setup_step(
    root: &std::path::Path,
    endpoints: &Endpoints,
    http: &reqwest::Client,
    step: &model::SetupAction,
) -> anyhow::Result<()> {
    use model::SetupAction as Action;
    match step {
        Action::BankProfile {
            provider,
            profile,
            error_rate,
            latency_ms_min,
            latency_ms_max,
            callback_delay_s,
        } => {
            let base = endpoints
                .bank(provider)
                .ok_or_else(|| anyhow::anyhow!("unknown bank {provider}"))?;
            let mut body = serde_json::json!({"profile": profile});
            if let Some(v) = error_rate {
                body["error_rate"] = (*v).into();
            }
            if let Some(v) = latency_ms_min {
                body["latency_ms_min"] = (*v).into();
            }
            if let Some(v) = latency_ms_max {
                body["latency_ms_max"] = (*v).into();
            }
            if let Some(v) = callback_delay_s {
                body["callback_delay_s"] = (*v).into();
            }
            http.put(format!("{base}/admin/profile"))
                .json(&body)
                .send()
                .await?
                .error_for_status()?;
            println!("setup: bank {provider} -> {profile}");
            Ok(())
        }
        Action::BankReset { provider } => {
            let base = endpoints
                .bank(provider)
                .ok_or_else(|| anyhow::anyhow!("unknown bank {provider}"))?;
            http.post(format!("{base}/admin/reset"))
                .send()
                .await?
                .error_for_status()?;
            println!("teardown: bank {provider} reset");
            Ok(())
        }
        Action::MerchantMode { status, latency_ms } => {
            http.put(format!("{}/admin/mode", endpoints.merchant))
                .json(&serde_json::json!({"status": status, "latency_ms": latency_ms.unwrap_or(0)}))
                .send()
                .await?
                .error_for_status()?;
            println!("setup: merchant mode {status}");
            Ok(())
        }
        Action::Routing { provider, enabled } => {
            set_routing(http, endpoints, provider, *enabled).await?;
            println!("setup: routing {provider} enabled={enabled}");
            Ok(())
        }
        Action::GatewayTtl { secs } => {
            check_docker()?;
            let secs_str = secs.to_string();
            let out = compose(
                root,
                &[
                    "--profile",
                    "traffic",
                    "--profile",
                    "obs",
                    "up",
                    "-d",
                    "--force-recreate",
                    "gateway-api",
                ],
                &[("GATEWAY_TXN_TTL_SECS", secs_str.as_str())],
            )?;
            if !out.status.success() {
                anyhow::bail!("{}", String::from_utf8_lossy(&out.stderr));
            }
            // Wait for health.
            for _ in 0..24 {
                if http
                    .get(format!("{}/healthz", endpoints.gateway))
                    .send()
                    .await
                    .is_ok()
                {
                    break;
                }
                tokio::time::sleep(std::time::Duration::from_secs(5)).await;
            }
            println!("setup: gateway TTL={secs}s");
            Ok(())
        }
        Action::DbLock { secs } => {
            check_docker()?;
            let sql = format!(
                "BEGIN; LOCK TABLE transactions IN ACCESS EXCLUSIVE MODE; SELECT pg_sleep({secs}); COMMIT;"
            );
            // Detached: returns immediately, lock held in background.
            let mut cmd = std::process::Command::new("docker");
            cmd.args([
                "compose",
                "exec",
                "-d",
                "postgres",
                "psql",
                "-U",
                "lab_app",
                "-d",
                "payslo_lab",
                "-c",
                &sql,
            ])
            .current_dir(root);
            let status = cmd.status()?;
            if !status.success() {
                anyhow::bail!("db lock injection failed");
            }
            println!("setup: db lock held ({secs}s)");
            Ok(())
        }
        Action::DbUnlock {} => {
            chaos_reset_db_only(root)?;
            println!("teardown: db locks released");
            Ok(())
        }
        Action::DeployGateway { version } => {
            let version = version.to_lowercase();
            if version != "good" && version != "bad" {
                anyhow::bail!("deploy_gateway version must be good|bad, got {version:?}");
            }
            http.put(format!("{}/admin/release", endpoints.gateway_admin))
                .json(&serde_json::json!({"version": version}))
                .send()
                .await?
                .error_for_status()?;
            println!("setup: gateway release {version}");
            Ok(())
        }
        Action::DdReset {} => {
            reset_datadog_mock(http, root).await?;
            println!("teardown: datadog-mock objects cleared");
            Ok(())
        }
        Action::Sleep { secs } => {
            tokio::time::sleep(std::time::Duration::from_secs(*secs)).await;
            Ok(())
        }
    }
}

/// Delete all managed SLOs/monitors from datadog-mock (test isolation).
async fn reset_datadog_mock(http: &reqwest::Client, root: &std::path::Path) -> anyhow::Result<()> {
    let conns =
        payslo_adapter::connections::load(&root.join("payslo-workspace").join("connections.yml"))?;
    let Some(conn) = conns.iter().find(|c| c.backend == "datadog") else {
        println!("warning: no datadog connection; dd_reset skipped");
        return Ok(());
    };
    let endpoint = conn.endpoint.clone().unwrap_or_default();
    let api_key = conn
        .api_key
        .as_ref()
        .map(payslo_adapter::connections::resolve_api_key)
        .transpose()?
        .unwrap_or_default();
    let slos: serde_json::Value = http
        .get(format!("{endpoint}/api/v1/slo?tags=managed-by:payslo"))
        .header("DD-API-KEY", &api_key)
        .send()
        .await?
        .json()
        .await?;
    let mons: serde_json::Value = http
        .get(format!("{endpoint}/api/v1/monitor?tags=managed-by:payslo"))
        .header("DD-API-KEY", &api_key)
        .send()
        .await?
        .json()
        .await?;
    let mut removed = 0;
    for mon in mons.as_array().cloned().unwrap_or_default() {
        if let Some(id) = mon.pointer("/id").and_then(serde_json::Value::as_i64) {
            http.delete(format!("{endpoint}/api/v1/monitor/{id}"))
                .header("DD-API-KEY", &api_key)
                .send()
                .await?;
            removed += 1;
        }
    }
    for slo in slos
        .pointer("/data")
        .and_then(serde_json::Value::as_array)
        .cloned()
        .unwrap_or_default()
    {
        if let Some(id) = slo.pointer("/id").and_then(serde_json::Value::as_str) {
            http.delete(format!("{endpoint}/api/v1/slo/{id}"))
                .header("DD-API-KEY", &api_key)
                .send()
                .await?;
            removed += 1;
        }
    }
    println!("dd_reset: removed {removed} object(s)");
    Ok(())
}

fn chaos_reset_db_only(root: &std::path::Path) -> anyhow::Result<()> {
    let _ = compose_exec(
        root,
        "postgres",
        &[
            "psql",
            "-U",
            "lab_app",
            "-d",
            "payslo_lab",
            "-t",
            "-c",
            "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE query LIKE '%ACCESS EXCLUSIVE%' AND pid <> pg_backend_pid();",
        ],
    );
    Ok(())
}

fn hint(root: &std::path::Path, id: &str) -> anyhow::Result<()> {
    let mut session = session::load(root, id)?;
    let text = std::fs::read_to_string(scenario_dir(root, id).join("hints.md"))
        .map_err(|e| anyhow::anyhow!("cannot read hints.md: {e}"))?;
    let hints = parse_hints(&text);
    if hints.is_empty() {
        anyhow::bail!("no hints found in hints.md (use `## Hint N` sections)");
    }
    let next = session.hints_used.len();
    if next >= hints.len() {
        println!(
            "no more hints (used {}/{})",
            session.hints_used.len(),
            hints.len()
        );
        return Ok(());
    }
    println!(
        "--- hint {}/{} (penalti kecil pada skor) ---",
        next + 1,
        hints.len()
    );
    println!("{}", hints[next].1);
    session.hints_used.push(hints[next].0.clone());
    session::save(root, &session)?;
    Ok(())
}

/// Split hints.md into (title, body) by `## ` headers.
fn parse_hints(text: &str) -> Vec<(String, String)> {
    let mut out = Vec::new();
    let mut title = String::new();
    let mut body = String::new();
    for line in text.lines() {
        if let Some(header) = line.strip_prefix("## ") {
            if !title.is_empty() || !body.trim().is_empty() {
                out.push((title.clone(), body.trim().to_owned()));
            }
            title = header.trim().to_owned();
            body.clear();
        } else {
            body.push_str(line);
            body.push('\n');
        }
    }
    if !title.is_empty() || !body.trim().is_empty() {
        out.push((title, body.trim().to_owned()));
    }
    out
}

fn answer(root: &std::path::Path, question: &str, text: &str) -> anyhow::Result<()> {
    let mut session = active_session_mut(root).ok_or_else(|| {
        anyhow::anyhow!("no active scenario (run `lab scenario start <id>` first)")
    })?;
    // Validate the question exists in the scenario definition.
    let scenario = model::load(&scenario_dir(root, &session.scenario_id).join("scenario.yml"))?;
    if !scenario.questions.iter().any(|q| q.id == question) {
        anyhow::bail!(
            "unknown question {question:?} (available: {})",
            scenario
                .questions
                .iter()
                .map(|q| q.id.as_str())
                .collect::<Vec<_>>()
                .join(", ")
        );
    }
    session.answers.insert(question.to_owned(), text.to_owned());
    let now = session::now_unix();
    session.record(now, "answer", question);
    session::save(root, &session)?;
    println!("answer[{question}] recorded ({} chars)", text.len());
    Ok(())
}

async fn verify(
    root: &std::path::Path,
    endpoints: &Endpoints,
    http: &reqwest::Client,
    id: &str,
) -> anyhow::Result<()> {
    let scenario = model::load(&scenario_dir(root, id).join("scenario.yml"))?;
    let session = session::load(root, id)?;
    let now = session.ended_at.unwrap_or_else(session::now_unix);
    println!("=== verify {}: {} ===", scenario.id, scenario.title);
    let mut diag_earned = 0u32;
    let mut diag_total = 0u32;
    let mut all_hard_pass = true;
    for scored in &scenario.checks {
        let result = eval::evaluate(http, endpoints, root, &session, now, scored).await;
        let mark = if result.passed { "PASS" } else { "FAIL" };
        let soft = if result.soft { " (soft)" } else { "" };
        println!("[{mark}{soft}] {} -- {}", result.description, result.detail);
        if !result.soft && !result.passed {
            all_hard_pass = false;
        }
        if matches!(
            scored.check,
            model::Check::AnswerContains { .. }
                | model::Check::PrometheusQuery { .. }
                | model::Check::SliAbove { .. }
        ) {
            diag_total += scored.points;
            diag_earned += result.points;
        }
    }
    // Diagnosis subscore scaled to 25.
    let diagnosis = if diag_total == 0 {
        25
    } else {
        ((diag_earned as f64 / diag_total as f64) * 25.0).round() as u32
    };
    // TTD / TTM.
    let (ttd, ttd_detail) = eval::score_ttd(&session, &scenario.scoring);
    // Mitigation evidence: non-answer, non-duration, non-soft checks passing.
    let mitigated_now = mitigation_passing(root, &scenario, &session, http, endpoints, now).await;
    let resolved_at = incident_resolved_at(http, endpoints, &scenario, &session).await;
    let (ttm, ttm_detail) =
        eval::score_ttm(&session, &scenario.scoring, resolved_at, mitigated_now, now);
    // Safety.
    let safety = if session.actions.iter().any(|a| is_unsafe(&scenario, a)) {
        0
    } else {
        15
    };
    let hints_penalty = (session.hints_used.len() as u32 * 2).min(10);
    let auto_total = ttd + ttm + diagnosis + safety;
    let auto_total = auto_total.saturating_sub(hints_penalty);
    println!("---");
    println!("TTD (first action): {ttd}/15 ({ttd_detail})");
    println!("TTM (mitigation): {ttm}/25 ({ttm_detail})");
    println!("diagnosis: {diagnosis}/25");
    println!("safety: {safety}/15");
    if hints_penalty > 0 {
        println!(
            "hints used: {} (-{hints_penalty})",
            session.hints_used.len()
        );
    }
    println!("AUTO: {auto_total}/80  (+ mentor communication/postmortem: /20 pending)");
    if all_hard_pass {
        println!("RESULT: PASS");
    } else {
        println!("RESULT: FAIL (some hard checks failed)");
        std::process::exit(1);
    }
    Ok(())
}

fn is_unsafe(scenario: &model::Scenario, action: &session::SessionAction) -> bool {
    eval::is_unsafe_action(&scenario.unsafe_actions, &action.cmd, &action.detail)
}

async fn mitigation_passing(
    root: &std::path::Path,
    scenario: &model::Scenario,
    session: &session::Session,
    http: &reqwest::Client,
    endpoints: &Endpoints,
    now: i64,
) -> bool {
    for scored in &scenario.checks {
        if scored.soft
            || matches!(
                scored.check,
                model::Check::AnswerContains { .. } | model::Check::DurationLt { .. }
            )
        {
            continue;
        }
        let result = eval::evaluate(http, endpoints, root, session, now, scored).await;
        if !result.passed {
            return false;
        }
    }
    true
}

/// Resolved timestamp of the scenario's incident (for TTM), if any.
async fn incident_resolved_at(
    http: &reqwest::Client,
    endpoints: &Endpoints,
    scenario: &model::Scenario,
    _session: &session::Session,
) -> Option<i64> {
    let (slo, service) = scenario.checks.iter().find_map(|c| match &c.check {
        model::Check::IncidentExists { slo, service } => Some((slo.clone(), service.clone())),
        _ => None,
    })?;
    let (_, resolved) = eval::incident_times(http, endpoints, slo.as_deref(), service.as_deref())
        .await
        .ok()?;
    resolved
}

#[cfg(test)]
mod cli_tests {
    use super::*;

    #[test]
    fn same_seed_same_pick() {
        let entries = [
            "01".to_owned(),
            "02".to_owned(),
            "03".to_owned(),
            "04".to_owned(),
            "05".to_owned(),
            "06".to_owned(),
            "07".to_owned(),
            "08".to_owned(),
            "09".to_owned(),
        ];
        assert_eq!(pick_random(&entries, 42), pick_random(&entries, 42));
        // Full sweep: every seed lands in range.
        for seed in 0..100u64 {
            assert!(entries.contains(&pick_random(&entries, seed).to_owned()));
        }
    }
}
