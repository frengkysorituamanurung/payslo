#![forbid(unsafe_code)]

//! Minimal embedded migration runner (replaces `sqlx::migrate!`).
//! Rationale: sqlx's `migrate` feature pulls `sqlx-sqlite`, whose
//! `libsqlite3-sys 0.3x` conflicts (`links = "sqlite3"`) with rusqlite's
//! `libsqlite3-sys 0.38` in one workspace graph — unresolvable. This runner
//! covers our needs: ordered embedded scripts + applied-version tracking in a
//! `_sqlx_migrations`-compatible table. See docs/DECISIONS.md.

use sha2::{Digest, Sha256};
use sqlx::PgPool;
use thiserror::Error;

#[derive(Debug, Error)]
pub enum MigrateError {
    #[error("migration {version} ({description}) failed: {reason}")]
    Apply {
        version: i64,
        description: String,
        reason: String,
    },
    #[error("checksum drift on applied migration {version} ({description})")]
    Checksum { version: i64, description: String },
    #[error("database error: {0}")]
    Db(String),
}

struct Migration {
    version: i64,
    description: &'static str,
    sql: &'static str,
}

const MIGRATIONS: &[Migration] = &[Migration {
    version: 1,
    description: "init",
    sql: include_str!("../../../db/migrations/0001_init.sql"),
}];

fn checksum(sql: &str) -> Vec<u8> {
    let mut hasher = Sha256::new();
    hasher.update(sql.as_bytes());
    hasher.finalize().to_vec()
}

/// Apply pending migrations in version order (idempotent).
pub async fn run(pool: &PgPool) -> Result<(), MigrateError> {
    sqlx::query(
        "CREATE TABLE IF NOT EXISTS _sqlx_migrations (
             version BIGINT PRIMARY KEY,
             description TEXT NOT NULL,
             installed_on TIMESTAMPTZ NOT NULL DEFAULT now(),
             success BOOLEAN NOT NULL,
             checksum BYTEA NOT NULL,
             execution_time BIGINT NOT NULL
         )",
    )
    .execute(pool)
    .await
    .map_err(|e| MigrateError::Db(e.to_string()))?;

    let applied: Vec<(i64, Vec<u8>)> =
        sqlx::query_as("SELECT version, checksum FROM _sqlx_migrations")
            .fetch_all(pool)
            .await
            .map_err(|e| MigrateError::Db(e.to_string()))?;

    for migration in MIGRATIONS {
        let sum = checksum(migration.sql);
        // None = fresh, Some(true) = adopt legacy row, Some(false) = drift.
        let mut adopt = false;
        match applied.iter().find(|(v, _)| *v == migration.version) {
            Some((_, old)) if *old == sum => continue,
            Some((_, old)) if old.len() != sum.len() => {
                // Legacy row (e.g. written by sqlx's runner, sha384 vs our
                // sha256): adopt it after proving the objects exist by
                // re-running the idempotent script once.
                eprintln!(
                    "migrate: adopting legacy row for migration {} ({})",
                    migration.version, migration.description
                );
                adopt = true;
            }
            Some(_) => {
                return Err(MigrateError::Checksum {
                    version: migration.version,
                    description: migration.description.to_owned(),
                });
            }
            None => {}
        }
        let start = std::time::Instant::now();
        // raw_sql uses the simple query protocol, which allows the whole
        // script (including DO blocks) in one go.
        sqlx::raw_sql(migration.sql)
            .execute(pool)
            .await
            .map_err(|e| MigrateError::Apply {
                version: migration.version,
                description: migration.description.to_owned(),
                reason: e.to_string(),
            })?;
        if adopt {
            sqlx::query(
                "UPDATE _sqlx_migrations SET checksum = $2, execution_time = $3 WHERE version = $1",
            )
            .bind(migration.version)
            .bind(sum)
            .bind(start.elapsed().as_millis() as i64)
            .execute(pool)
            .await
            .map_err(|e| MigrateError::Db(e.to_string()))?;
        } else {
            sqlx::query(
                "INSERT INTO _sqlx_migrations
                     (version, description, success, checksum, execution_time)
                 VALUES ($1, $2, TRUE, $3, $4)",
            )
            .bind(migration.version)
            .bind(migration.description)
            .bind(sum)
            .bind(start.elapsed().as_millis() as i64)
            .execute(pool)
            .await
            .map_err(|e| MigrateError::Db(e.to_string()))?;
        }
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn embedded_set_matches_dir() {
        // Every db/migrations/*.sql must be embedded above (no silent drift).
        let dir = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../db/migrations");
        let mut files: Vec<String> = std::fs::read_dir(&dir)
            .map(|rd| {
                rd.filter_map(|e| e.ok().map(|e| e.file_name().to_string_lossy().into_owned()))
                    .collect()
            })
            .unwrap_or_default();
        files.sort();
        let embedded: Vec<String> = MIGRATIONS
            .iter()
            .map(|m| format!("{:04}_{}.sql", m.version, m.description))
            .collect();
        assert_eq!(files, embedded);
    }

    /// Live-DB test: migrations apply twice cleanly (idempotent).
    #[tokio::test]
    async fn migrate_twice_ok() -> anyhow::Result<()> {
        let Some(url) = std::env::var("DATABASE_URL").ok() else {
            eprintln!("SKIP migrate_twice_ok: DATABASE_URL unset");
            return Ok(());
        };
        let pool = sqlx::postgres::PgPoolOptions::new()
            .max_connections(1)
            .connect(&url)
            .await?;
        run(&pool).await.map_err(|e| anyhow::anyhow!("{e}"))?;
        run(&pool).await.map_err(|e| anyhow::anyhow!("{e}"))?;
        Ok(())
    }
}
