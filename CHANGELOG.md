# CHANGELOG — PaySLO Lab

Format: Keep a Changelog (Added/Fixed/Changed/Security), versi Semantic Versioning.
Tanggal dalam UTC.

## [Unreleased]

### Added
- Lab Console (`lab serve` + React, U0–U4): AppShell + navigasi 11 rute,
  Mission Control (HTTP success vs Payment success), Peta Arsitektur,
  **Pembayaran** (form uji + tabel kursor + detail/timeline + status ke bank +
  tautan Loki), **Chaos & Kontrol** (traffic/routing/release + audit),
  **SLO Center** (daftar/detail SLO, editor YAML CodeMirror lazy, validasi
  berlokasi, plan + DiffViewer, apply ConfirmDialog + no-op jelas, drift,
  matriks kapabilitas, kalkulator error budget),
  **Insiden** (daftar/detail + timeline, alert terkorelasi sebagai teks,
  runbook diagnostik + ApprovalCard tanpa jalan pintas, catatan, postmortem
  + ekspor Markdown, toast + badge realtime).
- API U2 di `lab serve`: `GET/POST /api/v1/payments`, `GET /payments/{id}`,
  `GET /payments/{id}/bank-status`, `GET/PUT /traffic`, `GET/PUT /routing`,
  `PUT /release`, `GET /audit`. Baca Postgres sebagai `lab_ro` (SELECT saja)
  via `jackc/pgx/v5`; audit JSONL append-only; create lewat Envoy.
- API U3 di `lab serve`: `GET /api/v1/slos`, `GET/PUT /api/v1/slos/{name}`,
  `POST /slos/{name}/validate|plan|apply`, `GET /api/v1/drift`,
  `GET /api/v1/capabilities`. Membungkus `payslo` CLI via subprocess
  (resolusi `LAB_PAYSLO_BIN` → `target/debug/payslo` → PATH); editor hanya
  menulis `<slug>.yml` di `payslo-workspace/slo/` (traversal/absolut/symlink
  ditolak, maks 64 KiB); SLI live + alert aktif dari Prometheus rules API.
- API U4 di `lab serve` + payslo-server: `GET /api/v1/incidents[?status]`,
  `GET /api/v1/incidents/{id}` (timeline, alert terkorelasi, runbook,
  approvals), `POST .../notes`, `.../approvals` (request; aksi wajib langkah
  remediation runbook), `POST /api/v1/approvals/{aid}/decision` (approved →
  eksekusi remediation server-side; ganda → 409; reject tanpa reason → 422),
  `POST .../runbook/steps/{idx}/run` (diagnostic saja; remediation → 409),
  `GET/PUT .../postmortem` (Markdown server-side), `GET/PUT /api/v1/settings`
  (nama trainee = identitas approver), SSE `incident` + `approval`.
  Runbook tereksekusi: `payslo-workspace/runbooks/<slo>.yml` (diagnostic
  promql-allowlist/http-GET-loopback; remediation hanya `lab_routing` +
  impact wajib + approval wajib); tak ada endpoint eksekusi langsung.
- `api/openapi.yaml` v0.5.0 (skema pembayaran/kontrol/audit + SLO + insiden)
  + tipe TS di-generate (`openapi-typescript`).

### Tests
- Go: kontrak rute vs `openapi.yaml`, uji live `lab_ro` read-only
  (INSERT/UPDATE/DELETE ditolak), keamanan path editor (traversal, absolut,
  symlink escape), validasi berlokasi, parser output CLI, budget
  (99.9%/30d ≈ 43,2 mnt; 99.99%/30d ≈ 4,32 mnt), alur demo
  save→validate→plan→apply→no-op, runbook enforcement (remediation wajib
  approval/impact, diagnostic allowlist, URL loopback), postmortem render/
  roundtrip, alur approval demo (request→approve→executed, 409 ganda),
  fixture `--demo`.
- Rust (`payslo-server`): detail/notes/approvals lifecycle (422 reject tanpa
  reason, 409 ganda, 404, dedup idempoten, XSS verbatim).
- UI: Vitest (uang integer/Zod, timeline, Payments, Control, budget,
  DiffViewer, Slo, normalisasi NBSP, Insiden: XSS/no-shortcut/approval/
  postmortem) + Playwright e2e (axe + hanya loopback) untuk `/payments`,
  `/control`, `/slo`, `/incidents` (termasuk toast realtime + badge).
- `scripts/acceptance-u3.sh`, `scripts/acceptance-u4.sh`: acceptance live
  (U4: bank down → insiden → diagnostik → approve → routing off → pulih →
  resolved; workspace dikembalikan bersih).

## [v0.1.0] - 2026-10-15

Rilis awal lab: seluruh alur trainee + tool PaySLO berjalan lokal via Docker Compose.

### Added
- `gateway-api`: create/get payment, callback bank, webhook HMAC ke merchant-sink,
  routing admin, release good/bad, expiry PENDING, log JSON + propagasi traceparent.
- `bank-sim` (satu binary, 3 instance: bca/mandiri/ewallet): profil
  normal/slow/flaky/down/ambiguous/late-callback/dup-callback, RNG ChaCha8 ber-seed.
- `merchant-sink`: verifikasi HMAC constant-time, ledger per order (duplikat tampak),
  mode 200/500 + latensi untuk chaos.
- `datadog-mock`: subset API SLO/monitor Datadog v1 (header DD-API-KEY, persist file).
- PaySLO: model YAML `payslo/v1` + JSON Schema, CLI
  (`validate/render/plan/apply/drift/capabilities/serve`), adapter
  Prometheus (rules + `/-/reload`) dan Datadog (SLO dulu, baru monitor),
  contract test suite generik, server incident (dedup, korelasi, runbook engine
  dengan approval wajib remediation, audit append-only), UI askama, notifikasi ntfy.
- `lab` CLI: up/down/reset/status/open, chaos, traffic, routing, deploy,
  scenario (09 skenario), hint/answer/verify, mentor, snapshot, completion.
- Observability: Prometheus (scrape 5s), Alertmanager → PaySLO, Grafana
  (Payment Overview, Provider Health, SLO Overview, Incident Triage),
  sql_exporter (`lab_ro` read-only), Loki + Alloy (log Docker, project-scoped),
  Tempo (trace OTLP dari Envoy), Envoy ingress + tracing.
- Skenario 01–09 + solver otomatis + kurikulum 5 hari + mentor guide +
  troubleshooting.
- `scripts/smoke.sh`: pipeline penuh up → traffic → apply SLO → solver 01 →
  incident + ntfy + resolved → down (non-zero bila gagal; job CI 75 mnt).
- `docker-compose.prebuilt.yml` + workflow `release.yml` (GHCR, tag ber-versi,
  multi-arch, provenance + SBOM attestations).
- `docs/VERSIONS.md` (tabel pin + cara upgrade terkontrol).

### Fixed (FASE 8)
- PaySLO: firing ulang setelah incident resolved gagal
  (`UNIQUE constraint failed: incidents.id`, alertmanager retry selamanya);
  kini reopen insiden yang sama + notifikasi ulang (test regresi
  `firing_after_resolve_reopens_same_incident`).
- `lab up/down/reset/status` + `make up-lite` kini mencakup profil `payslo`
  (sebelumnya kontainer payslo/datadog-mock tertinggal saat down).
- `scripts/smoke.sh` menggantikan STUB `smoke.sh` FASE 0; `make e2e` tetap
  cek dasar cepat via `e2e-basic.sh`.

### Security
- Semua port host di-bind `127.0.0.1`; kredensial lab-only; data sintetis.
- Runtime `scratch` + non-root (kecuali service payslo: root lab-only karena
  volume SQLite root-owned — lihat README); tanpa shell di image Rust.
- `docker.sock` hanya `:ro` untuk Alloy (baca log), dengan peringatan.
- `cargo audit` + `cargo deny` bersih; `gitleaks` bersih; `#![forbid(unsafe_code)]`.

[v0.1.0]: https://github.com/payslo-lab/payslo-lab/releases/tag/v0.1.0
