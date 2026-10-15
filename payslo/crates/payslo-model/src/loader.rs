#![forbid(unsafe_code)]

//! YAML loader: file -> JSON Schema check -> typed document -> code validation.
//! `load_file` for one SLO, `load_dir` for a directory of `*.yml` with
//! cross-file name-uniqueness enforcement.

use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

use crate::error::ModelError;
use crate::model::SloDocument;
use crate::validate::validate_document;

/// Embedded copy of `payslo/schema/v1/slo.json` (same file, checked by test).
const SLO_SCHEMA_JSON: &str = include_str!("../../../schema/v1/slo.json");

fn schema_validator() -> Result<jsonschema::Validator, ModelError> {
    let schema: serde_json::Value =
        serde_json::from_str(SLO_SCHEMA_JSON).map_err(|e| ModelError::Schema {
            file: "<embedded slo.json>".to_owned(),
            path: "$".to_owned(),
            reason: format!("embedded schema is not valid JSON: {e}"),
        })?;
    jsonschema::validator_for(&schema).map_err(|e| ModelError::Schema {
        file: "<embedded slo.json>".to_owned(),
        path: "$".to_owned(),
        reason: format!("embedded schema is not a valid JSON Schema: {e}"),
    })
}

/// Load + fully validate one SLO file. `workspace_root` resolves runbooks.
pub fn load_file(path: &Path, workspace_root: &Path) -> Result<SloDocument, ModelError> {
    let file = path.display().to_string();
    let text = std::fs::read_to_string(path).map_err(|e| ModelError::Io {
        file: file.clone(),
        reason: e.to_string(),
    })?;
    let yaml: serde_yaml_ng::Value =
        serde_yaml_ng::from_str(&text).map_err(|e| ModelError::Yaml {
            file: file.clone(),
            reason: e.to_string(),
        })?;
    let json = serde_json::to_value(&yaml).map_err(|e| ModelError::Yaml {
        file: file.clone(),
        reason: format!("YAML is not JSON-compatible: {e}"),
    })?;
    let validator = schema_validator()?;
    if let Some(e) = validator.iter_errors(&json).next() {
        return Err(ModelError::Schema {
            file: file.clone(),
            path: e.instance_path().to_string(),
            reason: e.to_string(),
        });
    }
    let doc: SloDocument = serde_json::from_value(json).map_err(|e| ModelError::Schema {
        file: file.clone(),
        path: "$".to_owned(),
        reason: format!("shape mismatch after schema pass: {e}"),
    })?;
    validate_document(&file, &doc, workspace_root)?;
    Ok(doc)
}

/// Load all `*.yml` in `dir` (sorted for determinism), enforcing unique names.
pub fn load_dir(
    dir: &Path,
    workspace_root: &Path,
) -> Result<BTreeMap<String, (PathBuf, SloDocument)>, ModelError> {
    let mut entries: Vec<PathBuf> = std::fs::read_dir(dir)
        .map_err(|e| ModelError::Io {
            file: dir.display().to_string(),
            reason: e.to_string(),
        })?
        .filter_map(|e| e.ok().map(|e| e.path()))
        .filter(|p| {
            p.extension()
                .is_some_and(|ext| ext == "yml" || ext == "yaml")
        })
        .collect();
    entries.sort();
    let mut out: BTreeMap<String, (PathBuf, SloDocument)> = BTreeMap::new();
    for path in entries {
        let doc = load_file(&path, workspace_root)?;
        if let Some((first_path, _)) = out.get(&doc.name) {
            return Err(ModelError::DuplicateName {
                name: doc.name.clone(),
                first_file: first_path.display().to_string(),
            });
        }
        out.insert(doc.name.clone(), (path, doc));
    }
    Ok(out)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn embedded_schema_matches_file() {
        let manifest = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
        let on_disk = std::fs::read_to_string(manifest.join("../../schema/v1/slo.json"));
        match on_disk {
            Ok(text) => assert_eq!(
                text, SLO_SCHEMA_JSON,
                "schema drift: edit payslo/schema/v1/slo.json only"
            ),
            Err(_) => eprintln!("SKIP: schema file not found (monorepo layout changed?)"),
        }
    }
}
