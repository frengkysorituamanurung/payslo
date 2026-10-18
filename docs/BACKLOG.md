# BACKLOG — PaySLO Lab

Fitur di luar spesifikasi / ide tambahan. JANGAN dikerjakan tanpa diminta;
catat di sini saja (aturan AGENTS.MD no. 7).

Status per v0.1.0 (FASE 8, 2026-10-15):

- [x] `lab` shell completion via `clap_complete` (selesai FASE 6).
- [x] lab CLI: `mentor timeline|inject`, `snapshot save|load` (selesai FASE 7).
- [x] `docker-compose.prebuilt.yml` + publikasi GHCR (selesai FASE 8: prebuilt +
      workflow `release.yml`; image pertama terbit saat tag v0.1.0 di-push).
- [ ] Coverage `cargo llvm-cov` >= 70% logika inti (→ good-first-issue no. 1).
- [ ] Skenario tambahan di luar daftar kurikulum inti (→ no. 2).
- [ ] Dashboard Grafana tambahan di luar kebutuhan SLO/alert lab (→ no. 3).
- [ ] `verify` menolak insiden basi — temuan FASE 8 (→ no. 4).
- [ ] Healthcheck `payslo serve` + Envoy (→ no. 5, 6).
- [ ] SBOM file per rilis; `promtool check` di CI; test CLI tanpa Docker (→ no. 7, 8, 9).
- [ ] Label GitHub `good-first-issue` + filing 10 isu awal: draf siap di
      `docs/GOOD_FIRST_ISSUES.md` (repo belum punya remote; file-kan saat repo dibuat).

Rincian 10 isu awal: `docs/GOOD_FIRST_ISSUES.md`.

## UI (contract-first via api/openapi.yaml)
- [x] Endpoint Go: SSE `GET /api/v1/events` (U1), payments + bank-status +
      traffic/routing/release + audit (U2), SLO (U3), insiden/approval/runbook/
      postmortem/settings (U4; approval + notes + timeline di PaySLO).
- [ ] Endpoint Go lanjutan: chaos per-provider, scenario, mentor.
- [x] Halaman: Mission Control (U1), Peta Arsitektur (U0), Pembayaran +
      Chaos & Kontrol (U2), SLO Center (U3), Insiden (U4). Sisanya masih
      placeholder.
- [x] Halaman U7: Observability (/observe: iframe Grafana kiosk + fallback,
      tab Prometheus/Alertmanager/ntfy, anotasi filter-mode) + Pengaturan &
      Diagnostik (/settings: nama/bahasa/tema/ambang, laporan bebas secret,
      docker stats graceful). Endpoint Go: observe/* + /diagnostics.
- [x] Komponen UI-SPEC inti terpakai: TimeSeriesChart (U1), Timeline (U2),
      DiffViewer + YamlEditor/CodeMirror (U3), ApprovalCard + toast + badge (U4).
- [ ] Komponen lanjutan UI-SPEC: ErrorBudgetBar, BurnRateGauge, ServiceNode,
      HintLadder, ScoreCard.
- [ ] Chaos per-provider/DB/merchant (mentor-only) belum ada; U2 hanya
      traffic/routing/release (mitigasi trainee yang sah).
- [x] Budget JS: utama 208 KB gzip (< 350 KB); editor CodeMirror chunk malas
      terpisah 142 KB gzip (hanya dimuat di /slo).
- [x] MODE mentor + PIN + penegakan spoiler server-side (UI-SPEC §KEAMANAN 6-7) —
      selesai FASE U6: gate token + rate limit (5 salah -> jeda 5 mnt), /mentor 403
      untuk trainee, menu nav disembunyikan, dokumentasi jujur di mentor-guide.
- [x] Endpoint Go lanjutan: chaos per-provider, scenario, mentor — selesai FASE U6
      (mentor timeline/inject/snapshot/session/assessment/export + learn
      curriculum/glossary/progress; chaos per-provider tetap via lab CLI Rust).
- [x] Halaman: Belajar + Mentor (U6) — render kurikulum tersanitasi, checklist
      per modul, progres SQLite; timeline audit gabunga, inject, snapshot,
      penilaian manual, ekspor laporan.

## U8 (aksesibilitas, performa, keamanan, kompatibilitas, rilis)
- [x] Audit a11y: axe 0 serious/critical (9 halaman × 2 tema × 2 bahasa),
      keyboard-only, `aria-live`, `prefers-reduced-motion`, zoom 200% —
      `ui/e2e/a11y.spec.ts`.
- [x] Visual regression 9 halaman × 2 tema (deterministik, `--demo`) —
      `ui/e2e/visual.spec.ts`, baseline darwin + linux di-commit.
- [x] Performa: anggaran bundle (gate), FCP/DCL lokal, soak SSE 1 jam —
      `ui/e2e/perf.spec.ts` + `ui/scripts/check-bundle.mjs`.
- [x] Keamanan: DNS-rebinding (Host asing ditolak), CSP tanpa pelanggaran,
      hanya loopback, `npm audit` + lisensi, plus `go vet`/gofmt —
      `ui/e2e/security.spec.ts`, `cmd/lab/security_u8_test.go`.
- [x] Kompatibilitas Chromium/Firefox/WebKit, 1280/1024/768 px —
      `ui/e2e/compat.spec.ts`.
- [x] Rilis: binary console lintas platform (UI tersemat) di `release.yml`.
- [ ] **Skenario 01 sepenuhnya via UI**: rute `/scenarios` masih placeholder;
      acceptance "selesaikan skenario 01 lewat UI" BELUM terpenuhi (skenario
      tetap via CLI `lab`/halaman Insiden). Butuh endpoint Go `scenario/*`
      (start/stop/hint/answer/verify) + halaman wizard. → isu lanjutan.
- [ ] **Uji pembaca layar sungguhan** (NVDA/VoiceOver/TalkBack), bukan asersi
      peran/ARIA Playwright: struktur sudah diuji otomatis, tetapi verifikasi
      manual dengan pembaca layar nyata belum dilakukan di dev ini.
- [ ] Snapshot visual & metrik waktu hanya diukur di Chromium; Firefox/WebKit
      hanya dicek kompatibilitas fungsional + axe.
- [ ] Lighthouse penuh (butuh Chrome + audit jaringan) — saat ini memakai metrik
      setara dari Performance API (FCP/DCL/resource count).
