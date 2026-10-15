#![forbid(unsafe_code)]

//! Incident responders: trait + ntfy implementation.
//! Notifications fire on incident open/resolve with dashboard, runbook and
//! approve/reject action links (approval endpoints land in FASE 5b; the URL
//! contract is fixed here).

use thiserror::Error;

#[derive(Debug, Error)]
pub enum ResponderError {
    #[error("notify failed: {0}")]
    Notify(String),
}

/// Responder event (dispatched after the correlate transaction commits).
#[derive(Debug, Clone)]
pub enum ResponderEvent {
    Opened {
        incident_id: String,
        title: String,
        slo: String,
        service: String,
        severity: String,
        runbook: String,
    },
    Resolved {
        incident_id: String,
        title: String,
        slo: String,
        service: String,
    },
}

/// Incident responder (swappable: ntfy, mocks in tests).
#[async_trait::async_trait]
pub trait Responder: Send + Sync {
    async fn notify(&self, event: &ResponderEvent) -> Result<(), ResponderError>;
}

/// No-op responder (tests, `--no-notify` runs).
pub struct NullResponder;

#[async_trait::async_trait]
impl Responder for NullResponder {
    async fn notify(&self, _event: &ResponderEvent) -> Result<(), ResponderError> {
        Ok(())
    }
}

/// ntfy.sh-compatible responder (POST topic with headers).
/// Action buttons point at the server's approval endpoints
/// (`POST /approvals/{id}/approve|reject`, FASE 5b URL contract).
pub struct NtfyResponder {
    http: reqwest::Client,
    base_url: String,
    topic: String,
    server_public_url: String,
    grafana_public_url: String,
}

impl NtfyResponder {
    #[must_use]
    pub fn new(
        base_url: String,
        topic: String,
        server_public_url: String,
        grafana_public_url: String,
    ) -> Self {
        Self {
            http: reqwest::Client::new(),
            base_url,
            topic,
            server_public_url,
            grafana_public_url,
        }
    }

    fn build(
        &self,
        title: &str,
        body: &str,
        priority: &str,
        tags: &str,
        click: &str,
        actions: &str,
    ) -> reqwest::RequestBuilder {
        self.http
            .post(format!("{}/{}", self.base_url, self.topic))
            .header("Title", title)
            .header("Priority", priority)
            .header("Tags", tags)
            .header("Click", click)
            .header("Actions", actions)
            .header("Markdown", "yes")
            .body(body.to_owned())
    }
}

#[async_trait::async_trait]
impl Responder for NtfyResponder {
    async fn notify(&self, event: &ResponderEvent) -> Result<(), ResponderError> {
        let (title, body, priority, tags, actions) = match event {
            ResponderEvent::Opened {
                incident_id,
                title,
                slo,
                service,
                severity,
                runbook,
            } => (
                format!("[{severity}] {title}"),
                format!(
                    "Incident `{incident_id}` opened for SLO `{slo}` on `{service}`.\nRunbook: `{runbook}`"
                ),
                if severity == "page" {
                    "urgent"
                } else {
                    "default"
                },
                "rotating_light",
                format!(
                    "view, Open incident, {}/incidents/{}, clear=true; http, Approve remediation, {}/approvals/pending?incident={}",
                    self.server_public_url, incident_id, self.server_public_url, incident_id
                ),
            ),
            ResponderEvent::Resolved {
                incident_id,
                title,
                slo,
                service,
            } => (
                format!("[resolved] {title}"),
                format!("Incident `{incident_id}` resolved for SLO `{slo}` on `{service}`."),
                "low",
                "white_check_mark",
                String::new(),
            ),
        };
        let click = format!("{}/d/payment-overview?orgId=1", self.grafana_public_url);
        let req = self.build(&title, &body, priority, tags, &click, &actions);
        req.send()
            .await
            .map_err(|e| ResponderError::Notify(e.to_string()))?
            .error_for_status()
            .map_err(|e| ResponderError::Notify(e.to_string()))?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::{Arc, Mutex};

    struct Recording {
        titles: Vec<String>,
        bodies: Vec<String>,
        actions: Vec<String>,
    }

    struct FakeResponder {
        seen: Arc<Mutex<Recording>>,
    }

    #[async_trait::async_trait]
    impl Responder for FakeResponder {
        async fn notify(&self, event: &ResponderEvent) -> Result<(), ResponderError> {
            let mut seen = self.seen.lock().unwrap_or_else(|e| e.into_inner());
            match event {
                ResponderEvent::Opened { title, .. } => seen.titles.push(format!("open:{title}")),
                ResponderEvent::Resolved { title, .. } => {
                    seen.titles.push(format!("resolved:{title}"));
                }
            }
            seen.bodies.push(format!("{event:?}"));
            seen.actions.push("notify".to_owned());
            Ok(())
        }
    }

    #[tokio::test]
    async fn fake_responder_records() -> Result<(), String> {
        let seen = Arc::new(Mutex::new(Recording {
            titles: Vec::new(),
            bodies: Vec::new(),
            actions: Vec::new(),
        }));
        let responder = FakeResponder { seen: seen.clone() };
        responder
            .notify(&ResponderEvent::Opened {
                incident_id: "inc-1".to_owned(),
                title: "t".to_owned(),
                slo: "s".to_owned(),
                service: "svc".to_owned(),
                severity: "page".to_owned(),
                runbook: "r.md".to_owned(),
            })
            .await
            .map_err(|e| e.to_string())?;
        let seen = seen.lock().map_err(|e| e.to_string())?;
        assert_eq!(seen.titles, ["open:t"]);
        Ok(())
    }

    /// Ntfy request shape against a local fake server.
    #[tokio::test]
    async fn ntfy_posts_expected_headers() -> Result<(), String> {
        let seen: Arc<Mutex<Vec<(String, String)>>> = Arc::new(Mutex::new(Vec::new()));
        let seen_cb = seen.clone();
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
            .await
            .map_err(|e| e.to_string())?;
        let addr = listener.local_addr().map_err(|e| e.to_string())?;
        let app = axum::Router::new().route(
            "/topic",
            axum::routing::post(move |headers: axum::http::HeaderMap, body: String| {
                let seen_cb = seen_cb.clone();
                async move {
                    let title = headers
                        .get("title")
                        .and_then(|v| v.to_str().ok())
                        .unwrap_or("")
                        .to_owned();
                    seen_cb
                        .lock()
                        .unwrap_or_else(|e| e.into_inner())
                        .push((title, body));
                    "ok"
                }
            }),
        );
        let server = tokio::spawn(async move { if axum::serve(listener, app).await.is_err() {} });
        let responder = NtfyResponder::new(
            format!("http://{addr}"),
            "topic".to_owned(),
            "http://payslo:8081".to_owned(),
            "http://127.0.0.1:3002".to_owned(),
        );
        responder
            .notify(&ResponderEvent::Opened {
                incident_id: "inc-9".to_owned(),
                title: "slo down".to_owned(),
                slo: "s".to_owned(),
                service: "svc".to_owned(),
                severity: "page".to_owned(),
                runbook: "r.md".to_owned(),
            })
            .await
            .map_err(|e| e.to_string())?;
        server.abort();
        let seen = seen.lock().map_err(|e| e.to_string())?;
        assert_eq!(seen.len(), 1);
        assert!(seen[0].0.contains("slo down"), "{:?}", seen[0].0);
        assert!(seen[0].1.contains("inc-9"), "{:?}", seen[0].1);
        Ok(())
    }
}
