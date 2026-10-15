#![forbid(unsafe_code)]

//! merchant-sink: HMAC-verified webhook ledger for the lab.

mod routes;
mod verify;

use std::net::SocketAddr;

use axum::{
    Router,
    routing::{get, post, put},
};
use tracing::info;

use crate::routes::SinkState;

fn run_healthcheck() -> ! {
    let port = std::env::var("MERCHANT_SINK_PORT")
        .ok()
        .and_then(|v| v.parse::<u16>().ok())
        .unwrap_or(5000);
    std::process::exit(probe(port));
}

fn probe(port: u16) -> i32 {
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
        eprintln!("merchant-sink: fatal: {e:#}");
        std::process::exit(1);
    }
}

async fn run() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .json()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("merchant_sink=info")),
        )
        .init();

    let port: u16 = std::env::var("MERCHANT_SINK_PORT")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(5000);
    let secret: String = std::env::var("MERCHANT_WEBHOOK_SECRET").unwrap_or_else(|_| {
        eprintln!("MERCHANT_WEBHOOK_SECRET unset; using lab-only default (NOT for production)");
        "lab-only-webhook-secret-change-me".to_owned()
    });

    let state = SinkState::new(secret);
    let app = Router::new()
        .route("/healthz", get(routes::healthz))
        .route("/webhook", post(routes::webhook))
        .route("/state", get(routes::get_state))
        .route("/admin/mode", put(routes::put_mode).get(routes::get_mode))
        .with_state(state);

    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    info!(%addr, "merchant-sink listening");
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
