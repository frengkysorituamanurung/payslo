#![forbid(unsafe_code)]

//! Postgres access via runtime `sqlx::query` (no `query!` macros), so Docker
//! builds never need a live DB and no `.sqlx/` offline cache is required.
//!
//! NOTE: UUID/timestamp columns cross the DB boundary as `String`
//! (RFC 3339 for timestamps). sqlx codec features for `uuid`/`time` would
//! pull `sqlx-sqlite` into the graph, conflicting with rusqlite
//! (links="sqlite3"). See docs/DECISIONS.md.

use sqlx::{PgPool, Row};
use uuid::Uuid;

use crate::domain::{Amount, Channel, Provider, TxnEvent, TxnStatus};
use crate::error::AppError;

/// Row of the `transactions` table.
#[derive(Debug, Clone)]
pub struct Transaction {
    pub id: Uuid,
    pub merchant_id: String,
    pub order_id: String,
    pub channel: Channel,
    pub provider: Provider,
    pub amount: Amount,
    pub fee: Amount,
    pub status: TxnStatus,
    pub provider_ref: Option<String>,
    pub created_at: String,
    // Mirrors the DB column; not exposed via the API (kept for completeness).
    #[allow(dead_code)]
    pub updated_at: String,
    pub expires_at: String,
}

fn parse_channel(s: &str) -> Result<Channel, AppError> {
    s.parse::<Channel>()
        .map_err(|_| AppError::BadRequest(format!("unknown channel {s:?}")))
}

fn parse_provider(s: &str) -> Result<Provider, AppError> {
    s.parse::<Provider>()
        .map_err(|_| AppError::BadRequest(format!("unknown provider {s:?}")))
}

fn parse_status(s: &str) -> Result<TxnStatus, AppError> {
    s.parse::<TxnStatus>()
        .map_err(|_| AppError::BadRequest(format!("unknown status {s:?}")))
}

fn parse_uuid(s: &str) -> Result<Uuid, AppError> {
    s.parse::<Uuid>()
        .map_err(|_| AppError::BadRequest(format!("invalid uuid {s:?}")))
}

fn row_to_transaction(row: &sqlx::postgres::PgRow) -> Result<Transaction, AppError> {
    let id_raw: String = row.try_get("id").map_err(AppError::Db)?;
    Ok(Transaction {
        id: parse_uuid(&id_raw)?,
        merchant_id: row.try_get("merchant_id").map_err(AppError::Db)?,
        order_id: row.try_get("order_id").map_err(AppError::Db)?,
        channel: parse_channel(row.try_get("channel").map_err(AppError::Db)?)?,
        provider: parse_provider(row.try_get("provider").map_err(AppError::Db)?)?,
        amount: Amount(row.try_get("amount").map_err(AppError::Db)?),
        fee: Amount(row.try_get("fee").map_err(AppError::Db)?),
        status: parse_status(row.try_get("status").map_err(AppError::Db)?)?,
        provider_ref: row.try_get("provider_ref").map_err(AppError::Db)?,
        created_at: row.try_get("created_at").map_err(AppError::Db)?,
        updated_at: row.try_get("updated_at").map_err(AppError::Db)?,
        expires_at: row.try_get("expires_at").map_err(AppError::Db)?,
    })
}

/// Input for [`insert_created`] (keeps the argument count small for clippy).
pub struct NewTransaction<'a> {
    pub id: Uuid,
    pub merchant_id: &'a str,
    pub order_id: &'a str,
    pub channel: Channel,
    pub provider: Provider,
    pub amount: Amount,
    pub fee: Amount,
    pub expires_at: String,
}

/// Insert a CREATED transaction. Respects UNIQUE(merchant_id, order_id).
pub async fn insert_created(
    pool: &PgPool,
    tx: NewTransaction<'_>,
) -> Result<Transaction, AppError> {
    let row = sqlx::query(
        "INSERT INTO transactions
            (id, merchant_id, order_id, channel, provider, amount, fee, status, expires_at)
         VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, 'CREATED', $8::timestamptz)
         RETURNING id::text, merchant_id, order_id, channel, provider, amount, fee,
                   status, provider_ref, created_at::text, updated_at::text, expires_at::text",
    )
    .bind(tx.id.to_string())
    .bind(tx.merchant_id)
    .bind(tx.order_id)
    .bind(tx.channel.to_string())
    .bind(tx.provider.to_string())
    .bind(tx.amount.0)
    .bind(tx.fee.0)
    .bind(tx.expires_at)
    .fetch_one(pool)
    .await
    .map_err(|e| {
        if let sqlx::Error::Database(db) = &e
            && db.code().as_deref() == Some("23505")
        {
            return AppError::BadRequest("duplicate order_id for merchant".to_owned());
        }
        AppError::Db(e)
    })?;
    row_to_transaction(&row)
}

/// Apply a state-machine event and persist the new status (+ optional provider ref).
pub async fn apply_event(
    pool: &PgPool,
    id: Uuid,
    event: TxnEvent,
    provider_ref: Option<&str>,
) -> Result<Transaction, AppError> {
    let current = get(pool, id).await?;
    let next = current
        .status
        .transition(event)
        .map_err(|_| AppError::BadRequest("illegal status transition".to_owned()))?;
    let row = sqlx::query(
        "UPDATE transactions SET status = $2, provider_ref = COALESCE($3, provider_ref),
                 updated_at = now()
         WHERE id = $1::uuid
         RETURNING id::text, merchant_id, order_id, channel, provider, amount, fee,
                   status, provider_ref, created_at::text, updated_at::text, expires_at::text",
    )
    .bind(id.to_string())
    .bind(next.to_string())
    .bind(provider_ref)
    .fetch_one(pool)
    .await
    .map_err(AppError::Db)?;
    row_to_transaction(&row)
}

pub async fn get(pool: &PgPool, id: Uuid) -> Result<Transaction, AppError> {
    let row = sqlx::query(
        "SELECT id::text, merchant_id, order_id, channel, provider, amount, fee,
                status, provider_ref, created_at::text, updated_at::text, expires_at::text
         FROM transactions WHERE id = $1::uuid",
    )
    .bind(id.to_string())
    .fetch_optional(pool)
    .await
    .map_err(AppError::Db)?
    .ok_or(AppError::NotFound)?;
    row_to_transaction(&row)
}

pub async fn find_by_provider_ref(
    pool: &PgPool,
    provider_ref: &str,
) -> Result<Option<Transaction>, AppError> {
    let row = sqlx::query(
        "SELECT id::text, merchant_id, order_id, channel, provider, amount, fee,
                status, provider_ref, created_at::text, updated_at::text, expires_at::text
         FROM transactions WHERE provider_ref = $1 ORDER BY created_at DESC LIMIT 1",
    )
    .bind(provider_ref)
    .fetch_optional(pool)
    .await
    .map_err(AppError::Db)?;
    row.map(|r| row_to_transaction(&r)).transpose()
}

/// Expire PENDING transactions past `expires_at` via the state machine (P8).
/// Returns expired ids.
pub async fn expire_overdue(pool: &PgPool) -> Result<Vec<Uuid>, AppError> {
    let rows = sqlx::query(
        "SELECT id::text FROM transactions WHERE status = 'PENDING' AND expires_at <= now()",
    )
    .fetch_all(pool)
    .await
    .map_err(AppError::Db)?;
    let mut ids = Vec::with_capacity(rows.len());
    for row in rows {
        let id_raw: String = row.try_get("id").map_err(AppError::Db)?;
        let id = parse_uuid(&id_raw)?;
        // PENDING -> EXPIRED is the only legal path here; anything else is a
        // race with a concurrent callback and is left for the next sweep.
        if apply_event(pool, id, TxnEvent::Expired, None).await.is_ok() {
            ids.push(id);
        }
    }
    Ok(ids)
}

pub async fn insert_callback_log(
    pool: &PgPool,
    transaction_id: Uuid,
    provider: &str,
    payload: &serde_json::Value,
) -> Result<(), AppError> {
    sqlx::query(
        "INSERT INTO callback_log (transaction_id, provider, payload) VALUES ($1::uuid, $2, $3::jsonb)",
    )
    .bind(transaction_id.to_string())
    .bind(provider)
    .bind(payload.to_string())
    .execute(pool)
    .await
    .map_err(AppError::Db)?;
    Ok(())
}

pub async fn insert_webhook_delivery(
    pool: &PgPool,
    transaction_id: Uuid,
    attempt: i32,
    http_status: Option<i32>,
    status: &str,
) -> Result<(), AppError> {
    sqlx::query(
        "INSERT INTO webhook_delivery (transaction_id, attempt, http_status, status)
         VALUES ($1::uuid, $2, $3, $4)",
    )
    .bind(transaction_id.to_string())
    .bind(attempt)
    .bind(http_status)
    .bind(status)
    .execute(pool)
    .await
    .map_err(AppError::Db)?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Live-DB test: runs only when DATABASE_URL is set (compose up).
    /// Without a DB it prints a skip note and passes, so `cargo test`
    /// stays green on a bare clone.
    #[tokio::test]
    async fn transaction_lifecycle() -> anyhow::Result<()> {
        let Some(url) = std::env::var("DATABASE_URL").ok() else {
            eprintln!("SKIP transaction_lifecycle: DATABASE_URL unset");
            return Ok(());
        };
        let pool = sqlx::postgres::PgPoolOptions::new()
            .max_connections(1)
            .connect(&url)
            .await?;
        crate::migrate::run(&pool)
            .await
            .map_err(|e| AppError::Internal(anyhow::anyhow!("{e}")))?;

        let merchant = "test-merchant";
        let order = format!("lifecycle-{}", Uuid::new_v4());
        let tx = insert_created(
            &pool,
            NewTransaction {
                id: Uuid::new_v4(),
                merchant_id: merchant,
                order_id: &order,
                channel: Channel::Va,
                provider: Provider::Bca,
                amount: Amount(100_000),
                fee: Amount(4000),
                expires_at: (time::OffsetDateTime::now_utc() + time::Duration::minutes(10))
                    .format(&time::format_description::well_known::Rfc3339)
                    .unwrap_or_default(),
            },
        )
        .await?;
        assert_eq!(tx.status, TxnStatus::Created);

        let tx = apply_event(&pool, tx.id, TxnEvent::ChargeAccepted, Some("P-test-1")).await?;
        assert_eq!(tx.status, TxnStatus::Pending);

        // Duplicate order rejected via UNIQUE(merchant_id, order_id).
        let dup = insert_created(
            &pool,
            NewTransaction {
                id: Uuid::new_v4(),
                merchant_id: merchant,
                order_id: &order,
                channel: Channel::Va,
                provider: Provider::Bca,
                amount: Amount(100_000),
                fee: Amount(4000),
                expires_at: (time::OffsetDateTime::now_utc() + time::Duration::minutes(10))
                    .format(&time::format_description::well_known::Rfc3339)
                    .unwrap_or_default(),
            },
        )
        .await;
        assert!(matches!(dup, Err(AppError::BadRequest(_))));

        // Illegal jump PENDING -> PENDING rejected.
        assert!(matches!(
            apply_event(&pool, tx.id, TxnEvent::ChargeAccepted, None).await,
            Err(AppError::BadRequest(_))
        ));

        let tx = apply_event(&pool, tx.id, TxnEvent::BankSuccess, None).await?;
        assert_eq!(tx.status, TxnStatus::Success);

        // Terminal state: further transitions illegal.
        assert!(matches!(
            apply_event(&pool, tx.id, TxnEvent::BankFailure, None).await,
            Err(AppError::BadRequest(_))
        ));

        // Expiry path on a dedicated row with a past deadline.
        let id = Uuid::new_v4();
        insert_created(
            &pool,
            NewTransaction {
                id,
                merchant_id: merchant,
                order_id: &format!("expiry-{}", Uuid::new_v4()),
                channel: Channel::Qris,
                provider: Provider::Ewallet,
                amount: Amount(10_000),
                fee: Amount(1500),
                expires_at: (time::OffsetDateTime::now_utc() + time::Duration::minutes(10))
                    .format(&time::format_description::well_known::Rfc3339)
                    .unwrap_or_default(),
            },
        )
        .await?;
        apply_event(&pool, id, TxnEvent::ChargeAccepted, None).await?;
        sqlx::query(
            "UPDATE transactions SET expires_at = now() - interval '1 second' WHERE id = $1::uuid",
        )
        .bind(id.to_string())
        .execute(&pool)
        .await?;
        let expired = expire_overdue(&pool).await?;
        assert!(expired.contains(&id));
        assert_eq!(get(&pool, id).await?.status, TxnStatus::Expired);
        Ok(())
    }
}
