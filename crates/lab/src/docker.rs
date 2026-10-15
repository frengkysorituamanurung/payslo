#![forbid(unsafe_code)]

//! Docker Compose subprocess wrapper + lab endpoint config.
//! Clear errors when Docker/Compose is missing (AGENTS.md: actionable errors).

use std::process::Command;

/// Lab endpoints (env-overridable, host-side URLs).
#[derive(Debug, Clone)]
pub struct Endpoints {
    pub gateway: String,
    /// U0: gateway /admin/* pindah ke listener loopback terpisah (:3001).
    pub gateway_admin: String,
    pub bank_bca: String,
    pub bank_mandiri: String,
    pub bank_ewallet: String,
    pub merchant: String,
    pub prometheus: String,
    /// payslo-serve base URL (incident reads go over HTTP, never the DB file:
    /// bind-mounted SQLite is incoherent across the container boundary).
    pub payslo: String,
}

impl Endpoints {
    pub fn from_env(_root: std::path::PathBuf) -> Self {
        let env =
            |key: &str, default: &str| std::env::var(key).unwrap_or_else(|_| default.to_owned());
        Self {
            gateway: env("LAB_GATEWAY_URL", "http://127.0.0.1:3000"),
            gateway_admin: env("LAB_GATEWAY_ADMIN_URL", "http://127.0.0.1:3001"),
            bank_bca: env("LAB_BANK_BCA_URL", "http://127.0.0.1:4001"),
            bank_mandiri: env("LAB_BANK_MANDIRI_URL", "http://127.0.0.1:4002"),
            bank_ewallet: env("LAB_BANK_EWALLET_URL", "http://127.0.0.1:4003"),
            merchant: env("LAB_MERCHANT_URL", "http://127.0.0.1:5000"),
            prometheus: env("LAB_PROM_URL", "http://127.0.0.1:9090"),
            payslo: env("LAB_PAYSLO_URL", "http://127.0.0.1:8081"),
        }
    }

    #[must_use]
    pub fn bank(&self, provider: &str) -> Option<&str> {
        match provider {
            "bca" => Some(&self.bank_bca),
            "mandiri" => Some(&self.bank_mandiri),
            "ewallet" => Some(&self.bank_ewallet),
            _ => None,
        }
    }
}

/// Verify docker + compose plugin exist.
pub fn check_docker() -> anyhow::Result<()> {
    let out = Command::new("docker").arg("info").output().map_err(|_| {
        anyhow::anyhow!(
            "Docker tidak ditemukan: install Docker Desktop (atau colima) lalu coba lagi"
        )
    })?;
    if !out.status.success() {
        anyhow::bail!(
            "Docker daemon tidak jalan: jalankan Docker Desktop / `colima start`, lalu coba lagi"
        );
    }
    let out = Command::new("docker")
        .args(["compose", "version"])
        .output()
        .map_err(|_| anyhow::anyhow!("Plugin `docker compose` tidak ditemukan"))?;
    if !out.status.success() {
        anyhow::bail!("`docker compose` gagal: update Docker ke versi dengan Compose v2");
    }
    Ok(())
}

/// Run `docker compose ...` in the repo root, inheriting extra env.
pub fn compose(
    root: &std::path::Path,
    args: &[&str],
    extra_env: &[(&str, &str)],
) -> anyhow::Result<std::process::Output> {
    let mut cmd = Command::new("docker");
    cmd.arg("compose").args(args).current_dir(root);
    for (k, v) in extra_env {
        cmd.env(k, v);
    }
    cmd.output()
        .map_err(|e| anyhow::anyhow!("gagal menjalankan docker compose: {e}"))
}

/// Run docker compose exec (non-interactive) and return stdout.
pub fn compose_exec(root: &std::path::Path, service: &str, cmd: &[&str]) -> anyhow::Result<String> {
    let mut args = vec!["exec", "-T", service];
    args.extend_from_slice(cmd);
    let out = compose(root, &args, &[])?;
    if !out.status.success() {
        anyhow::bail!(
            "docker compose exec {service} gagal: {}",
            String::from_utf8_lossy(&out.stderr).trim()
        );
    }
    Ok(String::from_utf8_lossy(&out.stdout).into_owned())
}
