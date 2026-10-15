#![forbid(unsafe_code)]

//! payslo-server: incident intake (webhooks), correlation, responders.
//! FASE 5a: webhooks + SQLite + ntfy. U4 (lab console): incident detail,
//! notes, approvals over HTTP; runbook execution itself stays in lab serve.

pub mod app;
pub mod approvals;
pub mod db;
pub mod incidents;
pub mod normalize;
pub mod responder;
pub mod serve;
