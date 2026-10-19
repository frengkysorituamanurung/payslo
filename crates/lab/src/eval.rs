#![forbid(unsafe_code)]

//! Check evaluation + rubric scoring (pure logic; unit-tested without Docker).

use crate::docker::Endpoints;
use crate::model::Check;
use crate::session::Session;

/// One evaluated check.
#[derive(Debug, Clone)]
pub struct CheckResult {
    pub description: String,
    pub passed: bool,
    pub soft: bool,
    pub points: u32,
    pub detail: String,
}

/// Compare a Prometheus value with an operator.
pub fn compare(op: &str, actual: f64, expected: f64) -> Result<bool, String> {
    match op {
        ">" => Ok(actual > expected),
        ">=" => Ok(actual >= expected),
        "<" => Ok(actual < expected),
        "<=" => Ok(actual <= expected),
        "==" => Ok((actual - expected).abs() < 1e-9),
        "!=" => Ok((actual - expected).abs() >= 1e-9),
        _ => Err(format!("unknown operator {op:?}, want >|>=|<|<=|==|!=")),
    }
}

/// Linear decay score: full at <= full_secs, 0 at >= zero_secs.
pub fn decay_score(max_points: u32, elapsed_secs: u64, full_secs: u64, zero_secs: u64) -> u32 {
    if elapsed_secs <= full_secs {
        return max_points;
    }
    if elapsed_secs >= zero_secs || zero_secs <= full_secs {
        return 0;
    }
    let span = (zero_secs - full_secs) as f64;
    let left = (zero_secs - elapsed_secs) as f64;
    ((max_points as f64 * left / span).round() as u32).min(max_points)
}

/// Query Prometheus instant API. NoData -> Err (caller decides).
pub async fn prom_query(
    http: &reqwest::Client,
    prom: &str,
    query: &str,
) -> anyhow::Result<Option<f64>> {
    let resp: serde_json::Value = http
        .get(format!("{prom}/api/v1/query"))
        .query(&[("query", query)])
        .send()
        .await?
        .json()
        .await?;
    if resp.pointer("/status").and_then(serde_json::Value::as_str) != Some("success") {
        anyhow::bail!("prometheus query failed: {resp}");
    }
    let results = resp
        .pointer("/data/result")
        .and_then(serde_json::Value::as_array)
        .cloned()
        .unwrap_or_default();
    let first = results.first();
    match first.and_then(|r| r.pointer("/value/1")).and_then(|v| {
        v.as_str()
            .and_then(|s| s.parse::<f64>().ok())
            .or_else(|| v.as_f64())
    }) {
        Some(v) if v.is_finite() => Ok(Some(v)),
        _ => Ok(None),
    }
}

/// Count SLOs/monitors in datadog-mock via a named connection.
pub async fn datadog_counts(
    http: &reqwest::Client,
    root: &std::path::Path,
    connection: &str,
) -> anyhow::Result<(usize, usize)> {
    let conns =
        payslo_adapter::connections::load(&root.join("payslo-workspace").join("connections.yml"))?;
    let conn = conns
        .iter()
        .find(|c| c.name == connection)
        .ok_or_else(|| anyhow::anyhow!("unknown connection {connection:?}"))?;
    if conn.backend != "datadog" {
        anyhow::bail!("connection {connection:?} is not a datadog backend");
    }
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
    Ok((
        slos.pointer("/data")
            .and_then(serde_json::Value::as_array)
            .map(Vec::len)
            .unwrap_or(0),
        mons.as_array().map(Vec::len).unwrap_or(0),
    ))
}

/// Gateway routing state for a provider.
pub async fn routing_enabled(
    http: &reqwest::Client,
    gateway: &str,
    provider: &str,
) -> anyhow::Result<bool> {
    let routing: serde_json::Value = http
        .get(format!("{gateway}/admin/routing"))
        .send()
        .await?
        .json()
        .await?;
    Ok(routing
        .pointer(&format!("/{provider}/enabled"))
        .and_then(serde_json::Value::as_bool)
        .unwrap_or(false))
}

/// Incident list via payslo-serve HTTP (never the DB file: container/host
/// SQLite sharing over bind mounts is incoherent).
async fn fetch_incidents(
    http: &reqwest::Client,
    payslo: &str,
    slo: Option<&str>,
    service: Option<&str>,
) -> anyhow::Result<Vec<serde_json::Value>> {
    let mut url = format!("{payslo}/incidents");
    let mut parts = Vec::new();
    if let Some(slo) = slo {
        parts.push(format!("slo={slo}"));
    }
    if let Some(service) = service {
        parts.push(format!("service={service}"));
    }
    if !parts.is_empty() {
        url.push('?');
        url.push_str(&parts.join("&"));
    }
    let body: serde_json::Value = http.get(url).send().await?.json().await?;
    Ok(body
        .pointer("/incidents")
        .and_then(serde_json::Value::as_array)
        .cloned()
        .unwrap_or_default())
}

/// Incident row lookup (any status unless filtered by caller).
pub async fn incident_status(
    http: &reqwest::Client,
    endpoints: &Endpoints,
    slo: Option<&str>,
    service: Option<&str>,
) -> anyhow::Result<Option<String>> {
    let items = fetch_incidents(http, &endpoints.payslo, slo, service).await?;
    Ok(items
        .first()
        .and_then(|i| i.pointer("/status"))
        .and_then(serde_json::Value::as_str)
        .map(str::to_owned))
}

/// Payslo incident opened/resolved timestamps (for TTM).
pub async fn incident_times(
    http: &reqwest::Client,
    endpoints: &Endpoints,
    slo: Option<&str>,
    service: Option<&str>,
) -> anyhow::Result<(Option<i64>, Option<i64>)> {
    let items = fetch_incidents(http, &endpoints.payslo, slo, service).await?;
    let first = items.into_iter().next();
    match first {
        Some(item) => {
            let created = item
                .pointer("/created_at")
                .and_then(serde_json::Value::as_str)
                .and_then(parse_ts);
            let resolved = item
                .pointer("/resolved_at")
                .and_then(serde_json::Value::as_str)
                .and_then(parse_ts);
            Ok((created, resolved))
        }
        None => Ok((None, None)),
    }
}

fn parse_ts(_rfc3339: &str) -> Option<i64> {
    // Minimal RFC3339 UTC parser (format emitted by payslo-serve).
    let s = _rfc3339.strip_suffix('Z')?;
    let (date, time) = s.split_once('T')?;
    let d: Vec<u64> = date.split('-').filter_map(|x| x.parse().ok()).collect();
    let t: Vec<u64> = time.split(':').filter_map(|x| x.parse().ok()).collect();
    if d.len() != 3 || t.len() != 3 {
        return None;
    }
    let days = days_since_epoch(d[0], d[1], d[2])?;
    Some((days * 86_400 + t[0] * 3_600 + t[1] * 60 + t[2]) as i64)
}

fn days_since_epoch(y: u64, m: u64, d: u64) -> Option<u64> {
    if !(1..=12).contains(&m) || !(1..=31).contains(&d) || y < 1970 {
        return None;
    }
    let leap = |year: u64| {
        year.is_multiple_of(4) && (!year.is_multiple_of(100) || year.is_multiple_of(400))
    };
    let mut days = 0u64;
    for year in 1970..y {
        days += u64::from(!leap(year)) + 365;
    }
    let month_days = [
        31,
        28 + u64::from(leap(y)),
        31,
        30,
        31,
        30,
        31,
        31,
        30,
        31,
        30,
        31,
    ];
    for days_in_month in month_days.iter().take((m - 1) as usize) {
        days += days_in_month;
    }
    Some(days + d - 1)
}

/// Evaluate one check.
pub async fn evaluate(
    http: &reqwest::Client,
    endpoints: &Endpoints,
    root: &std::path::Path,
    session: &Session,
    now: i64,
    check: &crate::model::ScoredCheck,
) -> CheckResult {
    let desc = crate::model::describe(&check.check);
    let (passed, detail) = match &check.check {
        Check::PrometheusQuery {
            query,
            operator,
            value,
        } => match prom_query(http, &endpoints.prometheus, query).await {
            Ok(Some(actual)) => match compare(operator, actual, *value) {
                Ok(ok) => (ok, format!("value={actual}")),
                Err(e) => (false, e),
            },
            Ok(None) => (false, "nodata".to_owned()),
            Err(e) => (false, format!("query error: {e:#}")),
        },
        Check::IncidentExists { slo, service } => {
            match incident_status(http, endpoints, slo.as_deref(), service.as_deref()).await {
                Ok(Some(status)) => (true, format!("status={status}")),
                Ok(None) => (false, "no incident row".to_owned()),
                Err(e) => (false, format!("payslo error: {e:#}")),
            }
        }
        Check::RoutingState { provider, enabled } => {
            // U0: /admin/* hanya pada listener loopback admin (:3001), bukan
            // listener publik gateway (:3000) yang di-proxy Envoy.
            match routing_enabled(http, &endpoints.gateway_admin, provider).await {
                Ok(actual) => (actual == *enabled, format!("enabled={actual}")),
                Err(e) => (false, format!("gateway error: {e:#}")),
            }
        }
        Check::AnswerContains { question, contains } => match session.answers.get(question) {
            Some(answer) => {
                let ok = answer.to_lowercase().contains(&contains.to_lowercase());
                (ok, format!("answer len={}", answer.len()))
            }
            None => (false, "no answer submitted".to_owned()),
        },
        Check::SliAbove { query, threshold } => {
            match prom_query(http, &endpoints.prometheus, query).await {
                Ok(Some(actual)) => (actual > *threshold, format!("value={actual}")),
                Ok(None) => (false, "nodata".to_owned()),
                Err(e) => (false, format!("query error: {e:#}")),
            }
        }
        Check::DurationLt { max_secs } => {
            let elapsed = (now - session.started_at).max(0) as u64;
            (elapsed < *max_secs, format!("elapsed={elapsed}s"))
        }
        Check::DatadogObjects {
            connection,
            min_slos,
            min_monitors,
        } => match datadog_counts(http, root, connection).await {
            Ok((slos, mons)) => (
                slos >= *min_slos as usize && mons >= *min_monitors as usize,
                format!("slos={slos} monitors={mons}"),
            ),
            Err(e) => (false, format!("datadog error: {e:#}")),
        },
    };
    CheckResult {
        description: desc,
        passed: passed || check.soft,
        soft: check.soft,
        points: if passed { check.points } else { 0 },
        detail,
    }
}

/// Score TTD (15): first trainee action after start.
pub fn score_ttd(session: &Session, scoring: &crate::model::Scoring) -> (u32, String) {
    match session.first_action_after() {
        Some(secs) if secs >= 0 => {
            let pts = decay_score(
                15,
                secs as u64,
                scoring.ttd_full_secs,
                scoring.ttd_zero_secs,
            );
            (pts, format!("first action after {secs}s"))
        }
        _ => (0, "no trainee action recorded".to_owned()),
    }
}

/// Score TTM (25): resolved_at - start, or now - start if mitigated live.
pub fn score_ttm(
    session: &Session,
    scoring: &crate::model::Scoring,
    resolved_at: Option<i64>,
    mitigated_now: bool,
    now: i64,
) -> (u32, String) {
    let end = resolved_at.or(if mitigated_now { Some(now) } else { None });
    match end {
        Some(t) => {
            let elapsed = (t - session.started_at).max(0) as u64;
            let pts = decay_score(25, elapsed, scoring.ttm_full_secs, scoring.ttm_zero_secs);
            (pts, format!("mitigated after {elapsed}s"))
        }
        None => (0, "not mitigated".to_owned()),
    }
}

/// Unsafe-action matching: rule "cmd" matches any detail; "cmd:substr"
/// matches when detail contains substr. `chaos reset` (detail "reset") never
/// matches inject rules like "chaos:db" — resetting is mitigation, not chaos.
#[must_use]
pub fn is_unsafe_action(rules: &[String], cmd: &str, detail: &str) -> bool {
    rules.iter().any(|rule| match rule.split_once(':') {
        Some((rule_cmd, substr)) => cmd == rule_cmd && detail.contains(substr),
        None => cmd == *rule,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn operators() -> Result<(), String> {
        assert!(compare(">", 2.0, 1.0).map_err(|e| e.to_string())?);
        assert!(compare("==", 0.1 + 0.2, 0.3).map_err(|e| e.to_string())?);
        assert!(compare("~", 1.0, 1.0).is_err());
        Ok(())
    }

    #[test]
    fn decay_bounds() {
        assert_eq!(decay_score(15, 0, 120, 600), 15);
        assert_eq!(decay_score(15, 120, 120, 600), 15);
        assert_eq!(decay_score(15, 600, 120, 600), 0);
        assert_eq!(decay_score(15, 900, 120, 600), 0);
        assert_eq!(decay_score(25, 360, 300, 1500), 24);
    }

    #[test]
    fn timestamps_parse() {
        assert!(parse_ts("2026-10-14T12:00:00Z").is_some());
        assert_eq!(parse_ts("1970-01-01T00:00:00Z"), Some(0));
        assert!(parse_ts("bogus").is_none());
    }

    /// Uji regresi: `Check::RoutingState` harus membaca listener admin
    /// (`gateway_admin`, :3001), bukan listener publik gateway (:3000).
    /// Server publik sengaja mengembalikan `enabled=true`; admin `enabled=false`.
    /// Bila kode keliru memakai `endpoints.gateway`, hasilnya `true` -> test gagal.
    #[tokio::test]
    async fn routing_state_reads_admin_listener() -> Result<(), String> {
        let public = spawn_json_server(r#"{"bca":{"enabled":true}}"#)?;
        let admin = spawn_json_server(r#"{"bca":{"enabled":false}}"#)?;

        let mut ep = Endpoints::from_env(std::path::PathBuf::from("."));
        ep.gateway = public;
        ep.gateway_admin = admin;

        let http = reqwest::Client::builder()
            .timeout(std::time::Duration::from_secs(5))
            .build()
            .map_err(|e| e.to_string())?;
        let session = crate::session::Session::new("01", 0);
        let check = crate::model::ScoredCheck {
            check: crate::model::Check::RoutingState {
                provider: "bca".to_owned(),
                enabled: false,
            },
            points: 10,
            soft: false,
        };
        let result = evaluate(&http, &ep, std::path::Path::new("."), &session, 0, &check).await;
        assert!(result.passed, "detail: {}", result.detail);
        assert_eq!(result.points, 10, "detail: {}", result.detail);
        Ok(())
    }

    /// Server HTTP satu-tembak yang mengembalikan `body` sebagai JSON.
    fn spawn_json_server(body: &'static str) -> Result<String, String> {
        let listener = std::net::TcpListener::bind("127.0.0.1:0").map_err(|e| e.to_string())?;
        let addr = listener.local_addr().map_err(|e| e.to_string())?;
        std::thread::spawn(move || {
            if let Ok((mut stream, _)) = listener.accept() {
                use std::io::{Read as _, Write as _};
                let mut buf = [0u8; 1024];
                let _ = stream.read(&mut buf);
                let resp = format!(
                    "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                    body.len(),
                    body
                );
                let _ = stream.write_all(resp.as_bytes());
                let _ = stream.flush();
            }
        });
        Ok(format!("http://{addr}"))
    }

    #[test]
    fn unsafe_matching() {
        let rules = ["chaos:bank".to_owned(), "deploy:bad".to_owned()];
        assert!(is_unsafe_action(&rules, "chaos", "bank-bca-down"));
        assert!(!is_unsafe_action(&rules, "chaos", "reset"));
        assert!(is_unsafe_action(&rules, "deploy", "gateway=bad"));
        assert!(!is_unsafe_action(&rules, "deploy", "gateway=good"));
        assert!(!is_unsafe_action(&rules, "routing", "bca=false"));
    }
}
