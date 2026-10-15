//! schema.sql must stay consistent with db/migrations/*.sql (AGENTS.md).
//! This test compares normalized DDL so drift fails loudly.

use std::path::PathBuf;

fn manifest_dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
}

/// Strip `--` comments, collapse whitespace, drop empty lines.
fn normalize(sql: &str) -> String {
    sql.lines()
        .map(|line| match line.find("--") {
            Some(i) => &line[..i],
            None => line,
        })
        .map(str::trim)
        .filter(|line| !line.is_empty())
        .collect::<Vec<_>>()
        .join("\n")
        .split_whitespace()
        .collect::<Vec<_>>()
        .join(" ")
}

fn read_sql(relative: &str) -> String {
    let path = manifest_dir().join(relative);
    std::fs::read_to_string(&path).unwrap_or_else(|_| String::new())
}

#[test]
fn migration_matches_schema() {
    let schema = normalize(&read_sql("../../db/schema.sql"));
    let migration = normalize(&read_sql("../../db/migrations/0001_init.sql"));
    assert!(!schema.is_empty(), "db/schema.sql missing or empty");
    assert!(
        !migration.is_empty(),
        "db/migrations/0001_init.sql missing or empty"
    );
    assert_eq!(
        migration, schema,
        "db/migrations/0001_init.sql drifted from db/schema.sql"
    );
}
