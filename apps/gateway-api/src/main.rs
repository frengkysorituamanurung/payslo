#![forbid(unsafe_code)]

//! gateway-api: payment intake + bank routing + callbacks + merchant webhooks.
//! P1: this crate has NO Prometheus/metrics endpoint or instrumentation.
//! P2: HTTP 200 does not mean payment success (see routes).

mod bank;
mod config;
mod db;
mod domain;
mod error;
mod migrate;
mod routes;
mod webhook;

use std::net::SocketAddr;
use std::sync::Arc;

use tracing::info;

use crate::config::Config;
use crate::routes::AppState;

/// `gateway-api --healthcheck`: TCP-probe /healthz without curl/shell
/// (distroless/scratch runtimes have neither). Exit 0 on HTTP 200.
fn run_healthcheck() -> ! {
    let port = std::env::var("GATEWAY_PORT")
        .ok()
        .and_then(|v| v.parse::<u16>().ok())
        .unwrap_or(3000);
    std::process::exit(probe_healthz(port));
}

fn probe_healthz(port: u16) -> i32 {
    use std::io::{Read, Write};
    let addr = SocketAddr::from(([127, 0, 0, 1], port));
    let Ok(mut stream) =
        std::net::TcpStream::connect_timeout(&addr, std::time::Duration::from_secs(3))
    else {
        eprintln!("healthcheck: connect {addr} failed");
        return 1;
    };
    if stream
        .write_all(b"GET /healthz HTTP/1.0\r\nHost: localhost\r\n\r\n")
        .is_err()
    {
        eprintln!("healthcheck: write failed");
        return 1;
    }
    let mut buf = [0u8; 512];
    let Ok(n) = stream.read(&mut buf) else {
        eprintln!("healthcheck: read failed");
        return 1;
    };
    let head = String::from_utf8_lossy(&buf[..n]);
    if head.starts_with("HTTP/1.0 200") || head.starts_with("HTTP/1.1 200") {
        0
    } else {
        eprintln!("healthcheck: unexpected response: {head:.80}");
        1
    }
}

#[tokio::main]
async fn main() {
    if std::env::args().any(|a| a == "--healthcheck") {
        run_healthcheck();
    }
    if let Err(e) = run().await {
        eprintln!("gateway-api: fatal: {e:#}");
        std::process::exit(1);
    }
}

async fn run() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .json()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("gateway_api=info")),
        )
        .init();

    let config = Config::from_env()?;
    let pool = sqlx::postgres::PgPoolOptions::new()
        .max_connections(10)
        .connect(&config.database_url)
        .await?;
    crate::migrate::run(&pool)
        .await
        .map_err(|e| anyhow::anyhow!("{e}"))?;
    info!("database migrated");

    let state = Arc::new(AppState::new(pool.clone(), config.clone()));

    // Background expiry job: PENDING past expires_at -> EXPIRED.
    let expiry_pool = pool.clone();
    let expiry_interval = config.expiry_interval;
    tokio::spawn(async move {
        loop {
            tokio::time::sleep(expiry_interval).await;
            match crate::db::expire_overdue(&expiry_pool).await {
                Ok(ids) if !ids.is_empty() => {
                    info!(count = ids.len(), "expired overdue transactions")
                }
                Ok(_) => {}
                Err(e) => tracing::warn!(error = %e, "expiry sweep failed"),
            }
        }
    });

    let app = routes::public_router(state.clone());
    let admin_app = routes::admin_router(state);

    let addr = SocketAddr::from(([0, 0, 0, 0], config.port));
    info!(%addr, "gateway-api listening");
    let admin_addr = SocketAddr::from(([0, 0, 0, 0], config.admin_port));
    info!(%admin_addr, "gateway-api admin listening (loopback-only, never via Envoy)");
    let listener = tokio::net::TcpListener::bind(addr).await?;
    let admin_listener = tokio::net::TcpListener::bind(admin_addr).await?;
    let shutdown = shutdown_signal();
    let public = axum::serve(listener, app).with_graceful_shutdown(shutdown);
    let shutdown_admin = shutdown_signal();
    let admin = axum::serve(admin_listener, admin_app).with_graceful_shutdown(shutdown_admin);
    let (pub_res, admin_res) = tokio::join!(public, admin);
    pub_res?;
    admin_res?;
    Ok(())
}

/// Graceful shutdown on SIGTERM (Docker stop) or SIGINT (Ctrl-C).
async fn shutdown_signal() {
    #[cfg(unix)]
    {
        use tokio::signal::unix::{SignalKind, signal};
        let term = signal(SignalKind::terminate());
        let int = signal(SignalKind::interrupt());
        let term = async {
            if let Ok(mut sig) = term {
                sig.recv().await;
            }
        };
        let int = async {
            if let Ok(mut sig) = int {
                sig.recv().await;
            }
        };
        tokio::select! {
            () = term => info!("SIGTERM received, shutting down"),
            () = int => info!("SIGINT received, shutting down"),
        }
    }
    #[cfg(not(unix))]
    {
        let _ = signal::ctrl_c().await;
        info!("shutdown signal received");
    }
}
