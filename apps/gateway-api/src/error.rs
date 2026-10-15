#![forbid(unsafe_code)]

//! Shared error type mapped to HTTP responses.
//! P2: only internal errors/timeouts become 5xx; business failures stay 200.

use axum::{Json, http::StatusCode, response::IntoResponse};
use serde_json::json;
use thiserror::Error;

#[derive(Debug, Error)]
pub enum AppError {
    #[error("invalid request: {0}")]
    BadRequest(String),
    #[error("payment not found")]
    NotFound,
    #[error("no provider available for channel")]
    NoProvider,
    #[error("bank charge timed out")]
    BankTimeout,
    #[error("database error")]
    Db(#[from] sqlx::Error),
    #[error("downstream call failed")]
    Downstream(#[from] reqwest::Error),
    #[error("internal error")]
    Internal(#[from] anyhow::Error),
}

impl IntoResponse for AppError {
    fn into_response(self) -> axum::response::Response {
        let (status, message) = match &self {
            Self::BadRequest(msg) => (StatusCode::BAD_REQUEST, msg.clone()),
            Self::NotFound => (StatusCode::NOT_FOUND, self.to_string()),
            Self::NoProvider | Self::BankTimeout => {
                (StatusCode::SERVICE_UNAVAILABLE, self.to_string())
            }
            Self::Db(_) | Self::Downstream(_) | Self::Internal(_) => (
                StatusCode::INTERNAL_SERVER_ERROR,
                "internal error".to_owned(),
            ),
        };
        (status, Json(json!({ "error": message }))).into_response()
    }
}
