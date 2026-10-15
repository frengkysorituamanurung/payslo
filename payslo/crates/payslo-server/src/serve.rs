#![forbid(unsafe_code)]

//! Serve entrypoint: `payslo serve` wiring (called by payslo-cli).
//! Env: PAYSLO_PORT (8081), PAYSLO_DB_PATH, PAYSLO_WEBHOOK_TOKEN (optional),
//! NTFY_URL + NTFY_TOPIC (else NullResponder), PAYSLO_PUBLIC_URL,
//! GRAFANA_PUBLIC_URL.

use std::net::SocketAddr;
use std::sync::Arc;

use tracing::info;

use crate::app::{AppConfig, AppState};
use crate::responder::{NtfyResponder, NullResponder, Responder};

fn env_or(key: &str, default: &str) -> String {
    std::env::var(key).unwrap_or_else(|_| default.to_owned())
}

/// Run the incident server until shutdown signal.
pub async fn serve() -> anyhow::Result<()> {
    let port: u16 = env_or("PAYSLO_PORT", "8081").parse().unwrap_or(8081);
    let db_path = env_or("PAYSLO_DB_PATH", "payslo-workspace/payslo.db");
    let conn = crate::db::open(std::path::Path::new(&db_path))?;

    let responder: Arc<dyn Responder> = match (
        std::env::var("NTFY_URL").ok(),
        std::env::var("NTFY_TOPIC").ok(),
    ) {
        (Some(url), Some(topic)) => Arc::new(NtfyResponder::new(
            url,
            topic,
            env_or("PAYSLO_PUBLIC_URL", "http://127.0.0.1:8081"),
            env_or("GRAFANA_PUBLIC_URL", "http://127.0.0.1:3002"),
        )),
        _ => {
            info!("NTFY_URL/TOPIC unset: notifications disabled (NullResponder)");
            Arc::new(NullResponder)
        }
    };

    let state = AppState::new(
        conn,
        responder,
        AppConfig {
            webhook_token: std::env::var("PAYSLO_WEBHOOK_TOKEN").ok(),
            grafana_public_url: env_or("GRAFANA_PUBLIC_URL", "http://127.0.0.1:3002"),
        },
    );
    let app = crate::app::router(state);
    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    info!(%addr, db = %db_path, "payslo serve listening");
    let listener = tokio::net::TcpListener::bind(addr).await?;
    axum::serve(listener, app)
        .with_graceful_shutdown(shutdown_signal())
        .await?;
    Ok(())
}

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
        let _ = tokio::signal::ctrl_c().await;
        info!("shutdown signal received");
    }
}
