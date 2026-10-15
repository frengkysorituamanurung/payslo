#![forbid(unsafe_code)]

//! datadog-mock: subset tiruan Datadog API v1 (SLO + monitor).
//! BUKAN Datadog asli — lihat README + docs/ASSUMPTIONS.md.

use std::net::SocketAddr;

use datadog_mock::{MockConfig, MockState, fallback, router};
use tracing::info;

fn run_healthcheck() -> ! {
    let port = std::env::var("DATADOG_MOCK_PORT")
        .ok()
        .and_then(|v| v.parse::<u16>().ok())
        .unwrap_or(8126);
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
        eprintln!("datadog-mock: fatal: {e:#}");
        std::process::exit(1);
    }
}

async fn run() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .json()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("datadog_mock=info")),
        )
        .init();

    let port: u16 = std::env::var("DATADOG_MOCK_PORT")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(8126);
    // LAB-ONLY credential (synthetic lab).
    let api_key = std::env::var("DD_API_KEY").unwrap_or_else(|_| "lab-only-dd-api-key".to_owned());
    let db_file = std::env::var("DD_MOCK_DB_FILE").ok();
    let state = MockState::new(MockConfig { api_key, db_file });

    let app = router(state).fallback(fallback);
    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    info!(%addr, "datadog-mock listening (NOT real Datadog)");
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
