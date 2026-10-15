#![forbid(unsafe_code)]

//! Canonical PaySLO v1 model: types, YAML loading, JSON Schema + validation.

pub mod error;
pub mod loader;
pub mod model;
pub mod validate;

pub use error::ModelError;
pub use loader::{load_dir, load_file};
pub use model::{
    AlertStatus, Alerting, BurnRatePolicy, CanonicalAlert, Incident, IncidentStatus, Sli,
    SloDocument, SloTarget,
};
pub use validate::parse_duration;
