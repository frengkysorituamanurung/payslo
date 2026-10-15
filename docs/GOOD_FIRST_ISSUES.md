# GOOD FIRST ISSUES — 10 isu awal (label `good first issue`)

> Catatan FASE 8: repo ini belum punya remote git, jadi isu-isu di bawah ini
> adalah draf siap-filing (judul + label + isi). Saat repo GitHub dibuat,
> file-kan satu per satu via template di `.github/ISSUE_TEMPLATE/`.
> Sumber: `docs/BACKLOG.md` + temuan FASE 8. Estimasi: tiap isu < 1 hari
> untuk kontributor baru yang sudah lulus kurikulum hari 1–2.

## 1. Coverage logika inti >= 70% (`cargo llvm-cov`)
- Label: `good first issue`, `enhancement`, area: build/test.
- Isi: pasang `cargo-llvm-cov` di CI (job baru, non-blocking dulu), ukur baseline
  workspace, tambah test unit hingga modul inti
  (`gateway-api` state machine, `payslo-server` correlate, `lab` verify) >= 70%.
- Acceptance: badge/angka coverage tercatat di README; `cargo llvm-cov --workspace`
  hijau lokal.

## 2. Skenario 10: sampel dari game day
- Label: `good first issue`, `enhancement`, area: skenario.
- Isi: buat `scenarios/10/` (TTL pendek, 1 check `prometheus_query`, hints,
  solution, `solve.sh`) dari pola skenario 01. Daftarkan di `lab scenario list`.
- Acceptance: solver 10 PASS + `verify 10` FAIL-tanpa-aksi.

## 3. Dashboard Grafana tambahan: ledger merchant vs callback
- Label: `good first issue`, `enhancement`, area: observability.
- Isi: panel webhook deliveries vs ledger merchant-sink (membantu skenario 03
  duplikat). Taruh di `config/grafana/dashboards/`, provision otomatis.
- Acceptance: screenshot + query PromQL di `docs/`; `make up-lite` tetap <= 5 GB.

## 4. `verify` menolak insiden basi (freshness check)
- Label: `good first issue`, `bug`, area: lab CLI.
- Isi (temuan FASE 8): `verify 01` PASS memakai insiden resolved dari run
  berhari-hari lalu. Tambahkan check: `incident.created_at >= scenario start`
  (hard, kecilkan TTM bila perlu). Lihat `scripts/smoke.sh` step 6 untuk pola
  filter `SOLVER_T0` yang sudah terbukti.
- Acceptance: test unit (start lama + insiden lama → FAIL) + solver tetap PASS.

## 5. Subcommand `--healthcheck` untuk `payslo serve`
- Label: `good first issue`, `enhancement`, area: payslo.
- Isi: tiru pola `gateway-api --healthcheck` (GET /incidents internal atau buka
  SQLite read-only), lalu pasang `healthcheck:` + `depends_on` di service payslo.
- Acceptance: `docker compose ps` menunjukkan payslo `(healthy)`; image tetap
  `scratch` tanpa shell.

## 6. Healthcheck Envoy via admin `/ready`
- Label: `good first issue`, `enhancement`, area: observability.
- Isi: image envoy tanpa shell; gunakan `test: ["CMD-SHELL", ...]` tidak bisa.
  Opsi: sidecar probe atau `curl` image? Catat trade-off di DECISIONS; solusi
  paling sederhana yang membuat `depends_on envoy healthy` mungkin.
- Acceptance: gateway/loadgen `depends_on` envoy `service_healthy` hijau.

## 7. SBOM sebagai artefak rilis (bukan hanya attestation)
- Label: `good first issue`, `enhancement`, area: build/rilis.
- Isi: workflow `release.yml` sudah mengaktifkan attestasi SBOM; tambahkan file
  `sbom-<target>.spdx.json` ke GitHub Release via `anchore/sbom-action` (pin versi).
- Acceptance: tiap tag `v*` punya 5 file SBOM di halaman Release.

## 8. `promtool check rules` di CI untuk `payslo-workspace/generated/`
- Label: `good first issue`, `enhancement`, area: CI.
- Isi: job CI baru: render SLO contoh lalu `promtool check rules` via image
  `prom/prometheus` (pin sama dengan compose). Lihat catatan DECISIONS
  "Uji promtool pakai tmp di repo + entrypoint eksplisit".
- Acceptance: rules hasil render contoh selalu valid; gagal bila template rusak.

## 9. Test integrasi `lab scenario` tanpa Docker (mock compose)
- Label: `good first issue`, `enhancement`, area: lab CLI/test.
- Isi: `lab` memanggil `docker compose` sebagai subprocess; buat seam
  (env `LAB_COMPOSE_BIN`) agar test bisa memakai stub script + fixture,
  lalu tulis test `scenario start/verify` level CLI.
- Acceptance: `cargo test -p lab` mencakup alur start→answer→verify tanpa Docker.

## 10. Entri troubleshooting dari kegagalan FASE 8 nyata
- Label: `good first issue`, `documentation`, area: docs.
- Isi: abadikan 3 kegagalan nyata FASE 8 ke `docs/troubleshooting.md` bila belum ada:
  (a) `UNIQUE incidents.id` + alertmanager retry selamanya (cara baca log
  alertmanager `Notify for alerts failed`); (b) stream ntfy `/json` tidak
  pernah selesai (pakai `--max-time` + assert isi); (c) slow burn 15m/1h
  menahan incident open ~15-25 mnt (bukan bug).
- Acceptance: tiap entri punya gejala, perintah diagnosis, dan perbaikan.
