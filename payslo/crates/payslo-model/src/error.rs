#![forbid(unsafe_code)]

//! Actionable errors: every variant carries the file and (when known) the
//! field path so messages tell the user exactly what to fix.

use thiserror::Error;

/// Error loading or validating SLO documents.
#[derive(Debug, Error)]
pub enum ModelError {
    #[error("{file}: cannot read file: {reason}")]
    Io { file: String, reason: String },
    #[error("{file}: invalid YAML: {reason}")]
    Yaml { file: String, reason: String },
    #[error("{file}: schema violation at {path}: {reason}")]
    Schema {
        file: String,
        path: String,
        reason: String,
    },
    #[error("{file}: invalid value at {path}: {reason}")]
    Validation {
        file: String,
        path: String,
        reason: String,
    },
    #[error("duplicate SLO name {name:?} (also defined in {first_file})")]
    DuplicateName { name: String, first_file: String },
}

impl ModelError {
    pub fn validation(file: &str, path: &str, reason: impl Into<String>) -> Self {
        Self::Validation {
            file: file.to_owned(),
            path: path.to_owned(),
            reason: reason.into(),
        }
    }
}
