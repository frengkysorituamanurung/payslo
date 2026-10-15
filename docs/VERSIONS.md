# VERSIONS — PaySLO Lab v0.1.0

Semua versi di-pin; DILARANG `latest` untuk image dan wildcard (`*`) untuk crate
(aturan AGENTS.MD no. 3). `Cargo.lock` WAJIB di-commit.

## Toolchain

| Komponen | Versi | Didefinisikan di |
|---|---|---|
| Rust toolchain | 1.97.1 | `rust-toolchain.toml`, `Dockerfile` (`ARG RUST_VERSION`), `.github/workflows/ci.yml` |
| Edition | 2024 | `Cargo.toml` (`[workspace.package]`) |
| `rust-version` MSRV | 1.97.1 | `Cargo.toml` |
| cargo-chef (Docker builder) | 0.1.78 | `Dockerfile` (`cargo install --locked`) |
| Docker Compose | v2 (format tanpa `version:`) | `docker-compose*.yml` |
| k6 | 0.57.0 | image `grafana/k6` |

## Container image (semua pin eksplisit, multi-arch amd64+arm64)

| Service | Image | Arsitektur |
|---|---|---|
| postgres | `postgres:16.6-alpine` | amd64+arm64 (resmi) |
| envoy | `envoyproxy/envoy:v1.34.3` | amd64+arm64 (resmi) |
| loadgen | `grafana/k6:0.57.0` | amd64+arm64 (resmi) |
| prometheus | `prom/prometheus:v3.3.1` | amd64+arm64 (resmi) |
| alertmanager | `prom/alertmanager:v0.28.1` | amd64+arm64 (resmi) |
| sql-exporter | `burningalchemist/sql_exporter:0.24.9` | amd64+arm64 (resmi; BUKAN `burningalaxyhq`, repo itu 404 — lihat DECISIONS) |
| grafana | `grafana/grafana:11.6.1` | amd64+arm64 (resmi) |
| loki | `grafana/loki:3.4.2` | amd64+arm64 (resmi) |
| tempo | `grafana/tempo:2.7.1` | amd64+arm64 (resmi) |
| alloy | `grafana/alloy:v1.7.3` | amd64+arm64 (resmi) |
| ntfy | `binwiederhier/ntfy:v2.30.0` | amd64+arm64 (+arm v6/v7) (resmi) |
| gateway-api / bank-sim / merchant-sink / payslo / datadog-mock | build lokal (`Dockerfile`, musl statis, `scratch`) atau `ghcr.io/payslo-lab/<nama>:vX.Y.Z` (prebuilt) | amd64+arm64 via buildx |

Image prebuilt dipublikasikan oleh `.github/workflows/release.yml` setiap push tag
`v*`. Verifikasi: `docker manifest inspect <image>` harus mencantumkan
`linux/amd64` dan `linux/arm64`.

## Crate Rust (workspace, dari `Cargo.toml` + `Cargo.lock`)

| Crate | Versi | Keterangan |
|---|---|---|
| anyhow | 1.0.104 | binary/CLI saja |
| async-trait | 0.1.92 | trait `SloBackend` (`dyn`) |
| axum | 0.8.9 | HTTP server |
| clap (+ derive) | 4.6.7 | CLI |
| clap_complete | 4.6.11 | shell completion `lab` |
| hex | 0.4.3 | — |
| hmac | 0.13.0 | webhook HMAC |
| insta | 1.49.0 | snapshot test renderer |
| jsonschema | 0.58.6 | validasi JSON Schema SLO |
| rand / rand_chacha | 0.10.3 / 0.10.0 | RNG ber-seed (reproducible) |
| reqwest (`json`, `rustls`, `query`; tanpa default-features) | 0.13.5 | HTTP client rustls-only |
| rusqlite (`bundled`) | 0.40.2 | SQLite PaySLO (WAL) |
| serde / serde_json | 1.0.229 / 1.0.154 | — |
| serde_yaml_ng | 0.10.0 | YAML (pengganti `serde_yaml` yang di-archive) |
| sha2 | 0.11.1 | — |
| sqlx (`postgres`, `runtime-tokio`, `tls-rustls`; tanpa default-features) | 0.9.0 | Postgres gateway (query runtime; tanpa `.sqlx/`) |
| subtle | 2.6.1 | perbandingan constant-time |
| thiserror | 2.0.21 | error library |
| time (`parsing`, `formatting`, `serde`) | 0.3.55 | waktu (satu-satunya; bukan chrono) |
| tokio (`macros`, `rt-multi-thread`, `signal`, `time`, `net`, `sync`) | 1.53.3 | async runtime |
| tracing / tracing-subscriber (`env-filter`, `fmt`, `json`) | 0.1.44 / 0.3.23 | log JSON |
| uuid (`v4`, `serde`) | 1.28.0 | ID transaksi |

## CI actions (pin commit atau versi minor, di `.github/workflows/`)

| Action | Pin saat rilis |
|---|---|
| `actions/checkout` | v4.2.2 |
| `dtolnay/rust-toolchain` | stable (+ `toolchain: 1.97.1`) |
| `Swatinem/rust-cache` | v2.7.3 |
| `EmbarkStudios/cargo-deny-action` | v2.0.9 |
| `docker/setup-qemu-action` | v3.2.0 |
| `docker/setup-buildx-action` | v3.8.0 |
| `docker/build-push-action` | v6.10.0 |
| `docker/login-action` | v3.3.0 |
| `gitleaks/gitleaks-action` | v2.3.9 |
| `cargo-audit` | 0.22.2 (`--locked`) |

## Cara upgrade (terkontrol)

1. **Crate**: JANGAN `cargo update` buta (menggeser seluruh `Cargo.lock`).
   Naikkan SATU crate: `cargo update -p <nama>`, lalu `cargo build + test + deny + audit`.
   Commit `Cargo.toml` + `Cargo.lock` bersamaan dengan catatan di `docs/DECISIONS.md`
   bila crate menambah dependensi baru atau waktu build berubah signifikan.
2. **Image vendor**: cek `docker manifest inspect <img>:<versi-baru>` (pastikan
   amd64+arm64), ganti tag di `docker-compose.yml`, jalankan `make up-lite`
   + `make e2e`, catat di tabel atas + `docs/DECISIONS.md`.
3. **Toolchain Rust**: ubah SERENTAK `rust-toolchain.toml` + `Dockerfile`
   (`ARG RUST_VERSION`) + `Cargo.toml` (`rust-version`) + `ci.yml`
   (`toolchain:` di semua job). Full `cargo test --workspace` + rebuild Docker.
4. **Release image prebuilt**: setelah versi stabil, buat tag git
   `git tag -s vX.Y.Z && git push origin vX.Y.Z` → workflow `release.yml`
   mempublikasikan ke GHCR → uji `PAYSLO_IMAGE_TAG=vX.Y.Z make up-prebuilt`
   dari clone bersih sebelum mengumumkan.
