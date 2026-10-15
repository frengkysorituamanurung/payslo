#![forbid(unsafe_code)]

//! Apply-state store (rusqlite, WAL): per (slo, connection) records the
//! content hash of the last applied rendering plus the backend's external id.
//! `plan`/`drift` compare desired state against this record and the backend.

use rusqlite::{Connection, params};
use sha2::{Digest, Sha256};
use thiserror::Error;

/// State-store errors.
#[derive(Debug, Error)]
pub enum StoreError {
    #[error("sqlite error: {0}")]
    Sqlite(String),
    #[error("cannot create state dir {dir}: {reason}")]
    Dir { dir: String, reason: String },
}

/// One apply record.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ApplyRecord {
    pub slo: String,
    pub connection: String,
    pub content_hash: String,
    pub external_id: String,
}

/// SQLite-backed apply state. Synchronous API (CLI calls it directly;
/// server code must use `spawn_blocking`).
pub struct StateStore {
    conn: Connection,
}

impl StateStore {
    /// Open (creating parents) at `path`, enabling WAL + applies table.
    pub fn open(path: &std::path::Path) -> Result<Self, StoreError> {
        if let Some(parent) = path.parent()
            && !parent.as_os_str().is_empty()
        {
            std::fs::create_dir_all(parent).map_err(|e| StoreError::Dir {
                dir: parent.display().to_string(),
                reason: e.to_string(),
            })?;
        }
        let conn = Connection::open(path).map_err(|e| StoreError::Sqlite(e.to_string()))?;
        conn.pragma_update(None, "journal_mode", "WAL")
            .map_err(|e| StoreError::Sqlite(e.to_string()))?;
        conn.execute_batch(
            "CREATE TABLE IF NOT EXISTS applies (
                 slo TEXT NOT NULL,
                 connection TEXT NOT NULL,
                 content_hash TEXT NOT NULL,
                 external_id TEXT NOT NULL,
                 updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
                 PRIMARY KEY (slo, connection)
             )",
        )
        .map_err(|e| StoreError::Sqlite(e.to_string()))?;
        Ok(Self { conn })
    }

    /// Record a successful apply (upsert).
    pub fn record(
        &self,
        slo: &str,
        connection: &str,
        content_hash: &str,
        external_id: &str,
    ) -> Result<(), StoreError> {
        self.conn
            .execute(
                "INSERT INTO applies (slo, connection, content_hash, external_id)
                 VALUES (?1, ?2, ?3, ?4)
                 ON CONFLICT (slo, connection) DO UPDATE SET
                   content_hash = excluded.content_hash,
                   external_id = excluded.external_id,
                   updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')",
                params![slo, connection, content_hash, external_id],
            )
            .map_err(|e| StoreError::Sqlite(e.to_string()))?;
        Ok(())
    }

    /// Fetch one record.
    pub fn get(&self, slo: &str, connection: &str) -> Result<Option<ApplyRecord>, StoreError> {
        let mut stmt = self
            .conn
            .prepare("SELECT slo, connection, content_hash, external_id FROM applies WHERE slo = ?1 AND connection = ?2")
            .map_err(|e| StoreError::Sqlite(e.to_string()))?;
        let mut rows = stmt
            .query(params![slo, connection])
            .map_err(|e| StoreError::Sqlite(e.to_string()))?;
        match rows.next() {
            Ok(Some(row)) => Ok(Some(ApplyRecord {
                slo: row.get(0).map_err(|e| StoreError::Sqlite(e.to_string()))?,
                connection: row.get(1).map_err(|e| StoreError::Sqlite(e.to_string()))?,
                content_hash: row.get(2).map_err(|e| StoreError::Sqlite(e.to_string()))?,
                external_id: row.get(3).map_err(|e| StoreError::Sqlite(e.to_string()))?,
            })),
            Ok(None) => Ok(None),
            Err(e) => Err(StoreError::Sqlite(e.to_string())),
        }
    }

    /// Remove one record (after backend delete).
    pub fn remove(&self, slo: &str, connection: &str) -> Result<(), StoreError> {
        self.conn
            .execute(
                "DELETE FROM applies WHERE slo = ?1 AND connection = ?2",
                params![slo, connection],
            )
            .map_err(|e| StoreError::Sqlite(e.to_string()))?;
        Ok(())
    }

    /// All records, ordered by (slo, connection).
    pub fn list(&self) -> Result<Vec<ApplyRecord>, StoreError> {
        let mut stmt = self
            .conn
            .prepare("SELECT slo, connection, content_hash, external_id FROM applies ORDER BY slo, connection")
            .map_err(|e| StoreError::Sqlite(e.to_string()))?;
        let rows = stmt
            .query_map([], |row| {
                Ok(ApplyRecord {
                    slo: row.get(0)?,
                    connection: row.get(1)?,
                    content_hash: row.get(2)?,
                    external_id: row.get(3)?,
                })
            })
            .map_err(|e| StoreError::Sqlite(e.to_string()))?;
        let mut out = Vec::new();
        for row in rows {
            out.push(row.map_err(|e| StoreError::Sqlite(e.to_string()))?);
        }
        Ok(out)
    }
}

/// sha256 hex of content (drift/plan comparison unit).
#[must_use]
pub fn content_hash(content: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(content.as_bytes());
    hex::encode(hasher.finalize())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn record_roundtrip() -> Result<(), String> {
        let dir = std::env::temp_dir().join(format!("payslo-store-{}", std::process::id()));
        std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
        let store = StateStore::open(&dir.join("state.db")).map_err(|e| e.to_string())?;
        assert!(store.get("a", "c").map_err(|e| e.to_string())?.is_none());
        store
            .record("a", "c", "hash1", "ext1")
            .map_err(|e| e.to_string())?;
        let rec = store
            .get("a", "c")
            .map_err(|e| e.to_string())?
            .ok_or("missing")?;
        assert_eq!(rec.content_hash, "hash1");
        store
            .record("a", "c", "hash2", "ext2")
            .map_err(|e| e.to_string())?;
        let rec = store
            .get("a", "c")
            .map_err(|e| e.to_string())?
            .ok_or("missing")?;
        assert_eq!(
            (rec.content_hash.as_str(), rec.external_id.as_str()),
            ("hash2", "ext2")
        );
        store.remove("a", "c").map_err(|e| e.to_string())?;
        assert!(store.get("a", "c").map_err(|e| e.to_string())?.is_none());
        Ok(())
    }

    #[test]
    fn hash_stable() {
        assert_eq!(content_hash("x"), content_hash("x"));
        assert_ne!(content_hash("x"), content_hash("y"));
    }
}
