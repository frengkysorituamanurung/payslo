//! `promtool check rules` on every rendered template.
//! Prefers a local `promtool`, else Docker (`prom/prometheus:v3.3.1`).
//! Skips (green) only when neither is available.

use std::path::{Path, PathBuf};

fn templates() -> Result<Vec<PathBuf>, String> {
    let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../templates");
    let mut files: Vec<PathBuf> = std::fs::read_dir(&dir)
        .map_err(|e| e.to_string())?
        .filter_map(|e| e.ok().map(|e| e.path()))
        .filter(|p| p.extension().is_some_and(|e| e == "yml"))
        .collect();
    files.sort();
    Ok(files)
}

fn render_to(dir: &Path, template: &Path) -> Result<PathBuf, String> {
    let root = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../../payslo-workspace");
    let doc = payslo_model::load_file(template, &root).map_err(|e| e.to_string())?;
    let text = payslo_adapter_prometheus::PrometheusBackend::new(None)
        .render_native_text(&doc)
        .map_err(|e| e.to_string())?;
    let out = dir.join(template.file_name().ok_or("template has no name")?);
    std::fs::write(&out, text).map_err(|e| e.to_string())?;
    Ok(out)
}

fn have(program: &str) -> bool {
    std::process::Command::new(program)
        .arg("--help")
        .output()
        .is_ok()
}

#[test]
fn rendered_rules_pass_promtool() -> Result<(), String> {
    // NOTE: temp dir lives inside the repo (workspace target/) because Docker
    // runs in a Linux VM (colima): macOS paths like /var/folders are not
    // mounted into the VM, while the repo tree is.
    let work = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../../target/payslo-promtool")
        .join(format!("{}", std::process::id()));
    std::fs::create_dir_all(&work).map_err(|e| e.to_string())?;
    let mut rendered = Vec::new();
    for template in templates()? {
        rendered.push(render_to(&work, &template)?);
    }
    if rendered.is_empty() {
        return Err("no templates found".to_owned());
    }

    if have("promtool") {
        for file in &rendered {
            let status = std::process::Command::new("promtool")
                .args(["check", "rules"])
                .arg(file)
                .status()
                .map_err(|e| e.to_string())?;
            if !status.success() {
                return Err(format!("promtool rejected {}", file.display()));
            }
        }
        return Ok(());
    }
    if have("docker") {
        for file in &rendered {
            let name = file
                .file_name()
                .and_then(|n| n.to_str())
                .ok_or("template name not utf8")?;
            let status = std::process::Command::new("docker")
                .args([
                    "run",
                    "--rm",
                    "--entrypoint",
                    "promtool",
                    "-v",
                    &format!("{}:/rules:ro", work.display()),
                    "prom/prometheus:v3.3.1",
                    "check",
                    "rules",
                    &format!("/rules/{name}"),
                ])
                .status()
                .map_err(|e| e.to_string())?;
            if !status.success() {
                return Err(format!("promtool rejected {}", file.display()));
            }
        }
        return Ok(());
    }
    eprintln!("SKIP rendered_rules_pass_promtool: neither promtool nor docker found");
    Ok(())
}
