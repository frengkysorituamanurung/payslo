#![forbid(unsafe_code)]

//! SQLite access (rusqlite, WAL). Synchronous API; async handlers must call
//! via `tokio::task::spawn_blocking` (AGENTS.md).

use rusqlite::Connection;
use thiserror::Error;

#[derive(Debug, Error)]
pub enum DbError {
    #[error("sqlite error: {0}")]
    Sqlite(String),
}

pub type Result<T> = std::result::Result<T, DbError>;

fn err(e: rusqlite::Error) -> DbError {
    DbError::Sqlite(e.to_string())
}

/// Open `path` (creating parents), enable WAL, apply embedded migration.
pub fn open(path: &std::path::Path) -> Result<Connection> {
    if let Some(parent) = path.parent()
        && !parent.as_os_str().is_empty()
    {
        std::fs::create_dir_all(parent).map_err(|e| DbError::Sqlite(e.to_string()))?;
    }
    let conn = Connection::open(path).map_err(err)?;
    conn.pragma_update(None, "journal_mode", "WAL")
        .map_err(err)?;
    conn.execute_batch(include_str!("../migrations/0001_init.sql"))
        .map_err(err)?;
    // Later migrations run idempotently: only duplicate-column (already
    // applied) is tolerated; any other failure aborts startup loudly.
    for migration in [include_str!("../migrations/0002_approval_reason.sql")] {
        if let Err(e) = conn.execute_batch(migration)
            && !e.to_string().contains("duplicate column")
        {
            return Err(err(e));
        }
    }
    Ok(conn)
}

pub fn now_rfc3339() -> String {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| {
            let secs = d.as_secs() as i64;
            // Minimal RFC3339 UTC formatting without extra deps.
            format_rfc3339(secs)
        })
        .unwrap_or_default()
}

fn format_rfc3339(epoch_secs: i64) -> String {
    // Days-based civil date calculation (Howard Hinnant's algorithm).
    let days = epoch_secs.div_euclid(86_400);
    let secs_of_day = epoch_secs.rem_euclid(86_400);
    let z = days + 719_468;
    let era = z.div_euclid(146_097);
    let doe = z.rem_euclid(146_097);
    let yoe = (doe - doe / 1_460 + doe / 36_524 - doe / 146_096) / 365;
    let y = yoe + era * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = doy - (153 * mp + 2) / 5 + 1;
    let m = if mp < 10 { mp + 3 } else { mp - 9 };
    let year = if m <= 2 { y + 1 } else { y };
    format!(
        "{year:04}-{m:02}-{d:02}T{h:02}:{mi:02}:{s:02}Z",
        h = secs_of_day / 3_600,
        mi = (secs_of_day % 3_600) / 60,
        s = secs_of_day % 60,
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rfc3339_shapes() {
        // 2026-10-14T00:00:00Z
        assert_eq!(format_rfc3339(1_791_936_000), "2026-10-14T00:00:00Z");
        assert_eq!(format_rfc3339(0), "1970-01-01T00:00:00Z");
    }
}
