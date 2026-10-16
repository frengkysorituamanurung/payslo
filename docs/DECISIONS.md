# DECISIONS — PaySLO Lab

Catat setiap keputusan desain yang tidak tercantum di `AGENTS.MD`:
konteks, opsi, keputusan, alasan. Opsi paling sederhana diutamakan.

## Format

```text
## YYYY-MM-DD — Judul singkat
- Konteks: ...
- Opsi: A / B / ...
- Keputusan: ...
- Alasan: ...
```

## Log

### 2026-10-14 — Skeleton FASE 0 tanpa dependensi eksternal
- Konteks: FASE 0 meminta crate kerangka "kosong tapi ter-compile" + `[workspace.dependencies]`.
- Opsi: A) langsung tambah tokio/axum/dsb; B) kerangka tanpa deps, deps ditambah per fase.
- Keputusan: B. Workspace punya `[workspace.dependencies]` berisi `anyhow 1.0.104`
  dan `thiserror 2.0.21` (terverifikasi via `cargo info` pada 2026-10-14) sebagai fondasi
  error-handling, tapi crate skeleton belum menggunakannya agar build FASE 0 cepat.
- Alasan: hindari waktu kompilasi dan risiko versi yang belum terverifikasi; setiap
  dependensi baru dicatat di sini per fase (aturan AGENTS.MD).

### 2026-10-14 — YAML crate pengganti serde_yaml (ditunda ke fase model)
- Konteks: AGENTS.MD meminta YAML crate yang masih dipelihara (serde_yaml di-archive).
- Opsi: A) `yaml-rust2` + `serde`; B) `serde_yaml_ng` (fork aktif); C) tunda.
- Keputusan: Tunda ke fase PaySLO-model; kandidat utama `serde_yaml_ng`.
- Alasan: FASE 0 tidak butuh YAML; versi akan diverifikasi via `cargo info` saat fase itu.
- Verifikasi: `cargo info serde_yaml_ng` + cek docs.rs sebelum dipakai.

### 2026-10-14 — Ban crate metrics gateway-api via CI grep
- Konteks: `cargo-deny` bans bersifat workspace-wide, sedangkan P1 hanya melarang
  `prometheus`/`metrics`/exporter OTel di `gateway-api`.
- Opsi: A) deny workspace-wide (over-strict); B) CI grep khusus `apps/gateway-api`.
- Keputusan: B + komentar di `deny.toml`. Job CI `ban-gateway-metrics` gagal jika
  `apps/gateway-api` menyebut crate terlarang.
- Alasan: presisi sesuai P1 tanpa menghambat crate lain yang mungkin butuh metrik.

### 2026-10-14 — cargo-deny 0.20.2 (bukan 0.18.0)
- Konteks: `cargo-deny 0.18.0` gagal parse `cargo metadata` dari cargo 1.97.1
  (`failed to interpret cargo metadata's json ... column 14214`).
- Opsi: A) tetap 0.18.0; B) naik ke 0.20.2 (latest saat verifikasi).
- Keputusan: B. Skema `deny.toml` disesuaikan (hapus `version=`, `unmaintained`/`unsound`,
  `workspace-dependencies`; pakai `workspace-default-features`/`external-default-features`,
  ban memakai `{ crate = ... }`).
- Alasan: 0.18.0 tidak kompatibel dengan toolchain terpin; 0.20.2 `cargo deny check` exit 0.
- Verifikasi: `cargo deny --version` → 0.20.2; `cargo deny check` → `advisories ok, bans ok, licenses ok, sources ok`.

### 2026-10-14 — Dependensi FASE 1 (pin spesifik, terverifikasi)
- Konteks: gateway-api/bank-sim/merchant-sink butuh async HTTP + DB + RNG + HMAC.
- Keputusan (semua via `cargo info` / docs.rs 2026-10-14, `[workspace.dependencies]`):
  axum 0.8.9, tokio 1.53.3 (macros, rt-multi-thread, signal, time, net, sync),
  sqlx 0.9.0, reqwest 0.13.5, serde 1.0.229, serde_json 1.0.154,
  tracing 0.1.44, tracing-subscriber 0.3.23 (env-filter, fmt, json),
  uuid 1.28.0 (v4, serde), rand 0.10.3, rand_chacha 0.10.0,
  hmac 0.13.0, sha2 0.11.1, subtle 2.6.1, hex 0.4.3,
  time 0.3.55 (parsing, formatting, serde), cargo-chef 0.1.78.
- Image (terverifikasi `docker manifest inspect`): rust:1.97.1-bookworm,
  postgres:16.6-alpine, envoyproxy/envoy:v1.34.3, grafana/k6:0.57.0.

### 2026-10-14 — Nama fitur TLS dikoreksi dari AGENTS.md
- Konteks: AGENTS.md menulis sqlx fitur `rustls` dan reqwest fitur `rustls-tls`.
- Verifikasi docs.rs: sqlx 0.9.0 tidak punya fitur `rustls` (yang ada `tls-rustls`,
  `tls-rustls-ring`, ...); reqwest 0.13.5 tidak punya `rustls-tls` (yang ada `rustls`).
- Keputusan: sqlx `tls-rustls`, reqwest `default-features=false + ["json","rustls"]`
  (default rustls 0.13 = aws-lc + platform-verifier, tanpa native-tls).
- Alasan: pakai nama yang benar-benar ada; niat (rustls-only) tetap terpenuhi.

### 2026-10-14 — sqlx query runtime saja, tanpa `.sqlx/` offline cache
- Konteks: AGENTS.md mengizinkan query runtime biasa ATAU `query!` + `.sqlx/`.
- Keputusan: semua query via `sqlx::query` runtime; migrasi via `sqlx::migrate!`
  (embed saat kompilasi). Tidak ada `.sqlx/`.
- Alasan: build Docker tidak butuh DB hidup; paling sederhana untuk FASE 1.

### 2026-10-14 — Pengecualian `openssl-probe` + lisensi Zlib
- Konteks: `cargo deny` menolak `openssl-probe` (ban FASE 0) dan `foldhash` (Zlib).
- Verifikasi: openssl-probe 0.2.1 = helper path sertifikat, nol dependensi, nol
  linkage kripto (cek sumber di registry); ditarik reqwest `rustls` via
  rustls-platform-verifier. Zlib = lisensi permisif OSI-approved.
- Keputusan: hapus openssl-probe dari daftar ban (ban openssl/openssl-sys/
  native-tls tetap), tambah Zlib ke allow-list.
- Alasan: ban menarget implementasi TLS OpenSSL, bukan helper path; build statis
  musl tetap terjaga (terbukti: binary `statically linked`).

### 2026-10-14 — Adaptasi API crate baru
- `rand` 0.10: `random_range`/`random_bool` pindah ke trait `RngExt` (bukan `Rng`).
- `hmac` 0.13: `new_from_slice` butuh `KeyInit` dalam scope (`use hmac::{Hmac, KeyInit, Mac}`).
- serde: aturan bernama `kebab-case` (bukan `kebabcase`).
- k6 0.57.0: `import { check } from 'k6'` (`k6/check` dihapus → loadgen exit 107).

### 2026-10-14 — Dockerfile: fallback arsitektur + runtime scratch
- Konteks: build pertama gagal — builder klasik tanpa buildx membuat TARGETARCH
  kosong → skrip default ke x86_64 di host aarch64 → aws-lc asm gagal.
- Keputusan: fallback `uname -m` bila TARGETARCH kosong; install plugin
  docker-buildx di host dev. Runtime `scratch` + `USER 65532:65532` (tanpa shell,
  non-root; healthcheck via subcommand `--healthcheck`).
- Alasan: robust di builder klasik maupun buildx; memenuhi acceptance image.

### 2026-10-14 — Penolakan cepat bank tetap lewat PENDING (P8)
- Konteks: bank 5xx cepat harus jadi FAILED (HTTP 200), tapi P8 melarang lompatan
  CREATED→FAILED.
- Keputusan: CREATED→PENDING→FAILED (dua transisi legal, tercatat di DB).
- Alasan: tidak ada transisi ilegal yang dapat direpresentasikan.

### 2026-10-14 — Envoy FASE 1 tanpa healthcheck
- Konteks: image envoyproxy/envoy tanpa shell/curl; probe admin /ready butuh HTTP.
- Keputusan: tanpa healthcheck di FASE 1; gateway yang di-depend-on sudah healthy.
- Alasan: paling sederhana; ditambah bersama tooling obs di fase berikut.

### 2026-10-14 — Cek P1 dipersempit ke dependensi + pemakaian kode
- Konteks: grep CI awal mencocokkan kata "prometheus/metrics" di komentar
  dokumentasi (`//! P1: ...`), false positive.
- Keputusan: CI cek (1) nama crate di `apps/gateway-api/Cargo.toml`,
  (2) pola `use`/`::` di `src/`, (3) `cargo tree -p gateway-api`.
- Alasan: larangan menyasar dependensi & kode, bukan penyebutan di docs.

### 2026-10-14 — Image obs FASE 2 (semua terverifikasi manifest)
- prom/prometheus:v3.3.1, prom/alertmanager:v0.28.1, grafana/grafana:11.6.1,
  grafana/loki:3.4.2, grafana/tempo:2.7.1, grafana/alloy:v1.7.3,
  burningalchemist/sql_exporter:0.24.9 (bukan `burningalaxyhq`, repo itu 404).
- `docker manifest inspect` tiap image sebelum dipakai (multi-arch amd64+arm64).

### 2026-10-14 — sql_exporter butuh `key_labels` eksplisit
- Konteks: tanpa key_labels semua baris punya label kosong → error
  "collected with the same name and label values", metrik nyaris kosong.
- Keputusan: key_labels [channel,provider,status] dsb. (sesuai dokumentasi
  sql_exporter: kolom non-values TIDAK otomatis jadi label).

### 2026-10-14 — Timeout charge tetap PENDING + callback fallback transaction_id
- Konteks: demo down membuktikan timeout membocorkan baris CREATED selamanya
  (tak terlihat expiry/gauge; callback ambiguous 404 karena provider_ref NULL).
- Keputusan: setiap pembayaran diterima mencapai PENDING (timeout → PENDING +
  503); callback bawa transaction_id sebagai kunci fallback; baris CREATED yang
  menerima callback berjalan CREATED→PENDING→final. Tanpa transisi ilegal (P8).

### 2026-10-14 — loadgen lewat Envoy, bukan langsung gateway
- Konteks: SLI Envoy (RPS/5xx/p99) kosong karena loadgen menembak gateway:3000.
- Keputusan: GATEWAY_URL loadgen = http://envoy:8080 (sesuai arsitektur).

### 2026-10-14 — Alloy hanya project payslo-lab; backlog drain
- Konteks: Alloy mengangkut SEMUA container host (429 + entry too old).
- Keputusan: relabel keep `com_docker_compose_project=payslo-lab`. Replay log
  lama saat Alloy (re)start tetap ditolak Loki (too far behind) — transien,
  reda setelah backlog habis; di clone bersih backlog nyaris nol.

### 2026-10-14 — Tempo storage tmpfs; Envoy tanpa healthcheck (tetap)
- Konteks: image tempo/loki jalan sebagai uid 10001; volume bernama root-owned
  → Tempo gagal mkdir. Keputusan: tmpfs untuk /tmp/tempo (ephemeral, non-root).
- Envoy tetap tanpa healthcheck (tidak ada shell); kesiapan diverifikasi via
  :9901/stats/prometheus dari host.

### 2026-10-14 — Cek P1 dipersempit (lanjutan FASE 1)
- grep awal false-positive pada komentar docs; CI kini cek Cargo.toml +
  pola `use`/`::` di src + `cargo tree -p gateway-api`.

### 2026-10-14 — Demo down memakai TTL 45 dtk (diizinkan spec)
- AGENTS.md: TTL "dapat dipercepat via env" untuk lab. Command:
  `GATEWAY_TXN_TTL_SECS=45 docker compose up -d gateway-api` (default file 600).

### 2026-10-14 — serde_yaml_ng sebagai crate YAML (FASE 3)
- Konteks: AGENTS.md melarang serde_yaml (di-archive); kandidat: yaml-rust2, serde_yaml_ng.
- Verifikasi: serde_yaml_ng 0.10.0 = fork drop-in serde_yaml yang aktif (cargo info).
- Keputusan: serde_yaml_ng. Alasan: API kompatibel serde (loader 1 baris), terpelihara.

### 2026-10-14 — async-trait untuk SloBackend
- Konteks: AGENTS.md mengizinkan async-trait untuk trait adapter `dyn`.
- Keputusan: pakai async-trait 0.1.92 (verifikasi cargo info).
- Alasan: async fn native belum object-safe untuk `dyn SloBackend`.

### 2026-10-14 — Sanitiasi `-` → `_` pada nama rekaman Prometheus
- Konteks: nama SLO `payment-success-rate` menghasilkan record
  `payslo:slo_error_ratio:payment-success-rate:5m` — hyphen ilegal di
  identifier PromQL: series tersimpan tapi tak bisa di-query; alert berevaluasi
  kosong. Terbukti via API (count=9 ada, query nama persis kosong).
- Keputusan: `prom_slug()` ganti `-` jadi `_` untuk NAMA METRIK saja; label
  `slo:` dan nama grup tetap hyphen.
- Alasan: grup/label tak di-parse sebagai PromQL; metrik harus valid identifier.

### 2026-10-14 — Render teks manual (bukan YAML emitter)
- Konteks: butuh urutan field stabil + block scalar untuk PromQL.
- Keputusan: bangun teks YAML manual (BTreeSet/BTreeMap, sort eksplisit).
- Alasan: deterministik byte-identik; snapshot insta mengunci.

### 2026-10-14 — Uji promtool pakai tmp di repo + entrypoint eksplisit
- Konteks: (1) image prom/prometheus entrypoint-nya prometheus, jadi argumen
  harus `--entrypoint promtool`; (2) /var/folders macOS tak terlihat dari VM
  colima → tmp di `target/payslo-promtool-<pid>/`.

### 2026-10-14 — cargo-deny FASE 3: MIT-0, path-dep versions, RUSTSEC-2024-0449
- MIT-0 (borrow-or-share via jsonschema) = OSI-approved permisif → allow-list.
- Intra-workspace path deps wajib `version = "0.1.0"` (wildcards=deny).
- RUSTSEC-2024-0449 (unsafe-libyaml unmaintained, bukan vulnerabel): ignore
  beralasan di deny.toml — binding tipis ke libyaml C via serde_yaml_ng.

### 2026-10-14 — Selalu pakai --profile saat override env compose
- Konteks: `GATEWAY_TXN_TTL_SECS=45 docker compose up -d gateway-api` (tanpa
  --profile) tidak me-recreate container (TTL tetap 600); dengan
  `--profile traffic --profile obs` override bekerja (terverifikasi via inspect).
- Keputusan: semua perintah compose memakai profil eksplisit (sesuai Makefile).

### 2026-10-14 — Tempo mem_limit 384m setelah exit 137
- Konteks: tempo exit 137 (SIGKILL) sekali saat tekanan paralel; total profil
  masih ±2.2 GB (jauh di bawah batas 5 GB P6).

### 2026-10-14 — sqlx tanpa codec uuid/time/migrate (konflik links sqlite3)
- Konteks: sqlx (fitur apa pun yang menyentuh `sqlx-sqlite?/*`: uuid/time/migrate)
  + rusqlite 0.40 (libsqlite3-sys 0.38) = konflik `links="sqlite3"` yang tak
  terpecahkan dalam satu graph (sqlx-sqlite butuh libsqlite3-sys <0.38).
- Keputusan: sqlx = postgres+runtime-tokio+tls-rustls saja; boundary DB pakai
  String (UUID/RFC3339); migrasi via runner minimal `migrate.rs` (ter-embed,
  tabel `_sqlx_migrations` kompatibel, checksum sha256); rusqlite tetap (spec).
- Alasan: mematuhi spec rusqlite; runner 60 baris + test cukup untuk 1 file SQL.

### 2026-10-14 — State store di payslo-adapter (bukan crate baru)
- Konteks: AGENTS.md tak punya crate store; butuh content_hash+external_id.
- Keputusan: modul `store` di payslo-adapter ("utilitas bersama"), rusqlite WAL.

### 2026-10-14 — Canonical JSON terpisah placeholder vs resolved (Datadog)
- Konteks: monitor query butuh SLO id (dibuat server); plan vs drift
  membandingkan bentuk berbeda → false drift.
- Keputusan: render = placeholder; CLI plan/drift/apply pakai resolved
  (id asli); trait plan pakai placeholder. fetch_resolved vs fetch_canonical.

### 2026-10-14 — Monitor duplikat: label window
- Dua alert severity sama (fast+slow burn) = nama+label identik di Prometheus.
- Keputusan: label `window: <longWindow>` di kedua renderer.

### 2026-10-14 — RUSTSEC/d allowlist FASE 4
- MIT-0 (via jsonschema) = OSI-approved → allow. Path-dep wajib version
  (wildcards=deny). unsafe-libyaml unmaintained (bukan vuln) → ignore beralasan.

### 2026-10-14 — FASE 5a: fingerprint tanpa status + upsert
- Konteks: alert firing->resolved harus menutup incident; fingerprint mencakup
  status membuat resolved tak pernah cocok (incident tak pernah tutup).
- Keputusan: fingerprint = hash(source, name, label stabil); apply =
  INSERT..ON CONFLICT DO UPDATE status. Resend identik = no-op, transisi
  status menutup incident. Concurrent: Mutex + satu transaksi per correlate.

### 2026-10-14 — `payslo serve` subcommand (bukan bin terpisah)
- Konteks: AGENTS.md menulis CLI `payslo ...|serve`; sempat dibuat bin
  payslo-serve.
- Keputusan: logika di payslo-server::serve, CLI memanggil; bin dihapus.

### 2026-10-14 — thiserror: field jangan bernama `source`
- thiserror menganggap field `source` sebagai error source -> rename `origin`.

### 2026-10-14 — rusqlite mengaktifkan FK secara default
- Ujian DB CHECK butuh baris parent dulu (berbeda dari SQLite mentah).

### 2026-10-15 — FASE 6: lab CLI + skenario
- Session state = JSON (bukan rusqlite): single-user CLI, cukup + mudah dibaca.
- reads payslo via HTTP /incidents (FASE 6 tambah endpoint): SQLite di bind
  mount colima-sshfs INKOHEREN VM<->host (tulis container tak terlihat host,
  berujung "malformed"); server satu-satunya accessor (named volume).
- payslo service user root (lab-only): volume root-owned, image tanpa tool chown.
- chaos db = ACCESS EXCLUSIVE lock + pg_sleep (deterministik, reversible via
  pg_terminate_backend), bukan statement_timeout (gagal total karena tak ada
  finals baru → burn tak menyala).
- Loadgen via Envoy; scenario start me-restart loadgen (jendela 30 mnt segar).
- Solver mengirim pembayaran uji sendiri (tak tergantung loadgen).
- TTD = aksi trainee pertama; TTM = resolved_at payslo else now-jika-mitigated.
  Safety: chaos-inject/deploy-bad = 0; `chaos reset` aman. PASS = semua hard
  check lolos; auto /80 + manual /20.
- mentor/snapshot CLI ditunda (BACKLOG); lab help + completion clap_complete.

### 2026-10-15 — FASE 7: lab CLI, skenario 06-09, mentor, snapshot
- payslo service user root (lab-only): volume root-owned; DB hanya diakses
  server via named volume (bind-mount sshfs korup SQLite bila dibaca host).
  lab baca insiden via HTTP /incidents (endpoint baru), bukan file DB.
- chaos db = ACCESS EXCLUSIVE lock + pg_sleep (deterministik; statement_timeout
  gagal total karena tak ada finals baru). Reset via pg_terminate_backend.
- Session JSON (bukan rusqlite); TTD = aksi trainee pertama; safety:
  inject chaos / deploy-bad = 0, `chaos reset` aman.
- Game day: `seed % n` terurut; start 09 mendelegasikan ke random.
- Snapshot = pg_dump --data-only (tanpa _sqlx_migrations) + profil chaos;
  load = TRUNCATE CASCADE + replay. Merchant mode selalu ikut snapshot via
  GET /admin/mode (endpoint baru).
- Skenario 08 butuh aksi setup `dd_reset` + `deploy_gateway` (varian SetupAction
  baru) dan check `datadog_objects` (varian Check baru, via connections.yml).

### 2026-10-15 — FASE 8: ntfy sebagai notifier lokal
- Konteks: spec profil payslo = payslo + datadog-mock + ntfy, tapi compose
  belum punya service ntfy (payslo fallback NullResponder bila NTFY_URL unset).
- Verifikasi: `binwiederhier/ntfy:v2.30.0` multi-arch (amd64+arm64+armv6/v7,
  cek `docker manifest inspect` 2026-10-15).
- Keputusan: service `ntfy` (`serve`, `127.0.0.1:8022:80`, tanpa auth,
  lab-only) + `NTFY_URL=http://ntfy:80`, `NTFY_TOPIC=payslo-lab` di payslo.
  Tanpa healthcheck (image tanpa probe stabil; smoke poll HTTP :8022).
- Alasan: payload JSON ntfy cocok untuk assertion smoke (`/json?since=all`);
  kegagalan notify hanya warn (tidak menggagalkan ingest).
- Batasan jujur: image upstream berjalan sebagai root (tanpa USER); mitigasi =
  loopback-only + lab-only. Healthcheck payslo/ntfy/envoy = backlog (isu 5, 6).

### 2026-10-15 — FASE 8: reopen insiden (bug UNIQUE incidents.id)
- Konteks: smoke menemukan alertmanager retry selamanya:
  `UNIQUE constraint failed: incidents.id` — id deterministik per
  (slo, service) menabrak baris resolved dari run sebelumnya, sehingga TIDAK
  ADA insiden/notifikasi baru (verify lolos menumpang insiden lama!).
- Keputusan: `create_or_reopen_incident` — upsert `ON CONFLICT(id) DO UPDATE
  status='open', resolved_at=NULL WHERE status='resolved'` + event Opened
  (timeline `reopened`, audit `incident.reopen`, notifikasi ulang).
- Alasan: mempertahankan konvergensi deterministik restart; test regresi
  `firing_after_resolve_reopens_same_incident` hijau.

### 2026-10-15 — FASE 8: durasi smoke mengikuti burn 1h/10m
- Konteks: insiden tidak resolved hingga ticket 1h/10m reda; empiris ~57 mnt
  pasca-mitigasi (bukan bug — sesuai window lab di spec).
- Keputusan: step 6 smoke poll hingga 3900s; job CI `smoke` timeout 75→120 mnt
  (total smoke ~75 mnt). Deadline, bukan sleep tetap.
- `verify 01` yang menerima insiden basi = perilaku FASE 6 tak diubah
  (di luar scope); smoke mengassert freshness sendiri via SOLVER_T0
  (draf isu no. 4).

### 2026-10-15 — FASE 8: SBOM via attestasi rilis
- Konteks: `syft/trivy/grype` tak tersedia di host; build lokal tanpa registry
  tak bisa attestasi.
- Keputusan: `release.yml` aktifkan `provenance + sbom` attestations;
  file SBOM per rilis = good-first-issue no. 7.

### 2026-10-16 — U0: admin gateway pindah :3001, Grafana geser :3002
- Konteks: UI-SPEC U0 mewajibkan admin gateway di 127.0.0.1:3001 + Envoy
  memblokir /admin/*. Port :3001 sudah dipakai Grafana (compose, README,
  kurikulum, troubleshooting, default GRAFANA_PUBLIC_URL payslo).
- Opsi: A) admin di port lain (melanggar spec eksplisit); B) Grafana pindah.
- Keputusan: B. gateway-api serve dua listener (public_router :3000,
  admin_router :3001 via GATEWAY_ADMIN_PORT); Envoy 403 direct_response untuk
  prefix /admin/ dan path /admin (terverifikasi live: 403 via :8080, 404 via
  :3000, 200 via :3001); Grafana 127.0.0.1:3002 + GF_SECURITY_ALLOW_EMBEDDING.
  `lab` CLI memakai LAB_GATEWAY_ADMIN_URL untuk /admin/* gateway.
- Alasan: patuhi spec; satu-satunya konflik port diselesaikan dengan
  memindahkan yang tidak disebut spec. Semua referensi :3001 Grafana
  diperbarui (compose, Makefile, README, lab open, payslo defaults, kurikulum).

### 2026-10-16 — U0: lab serve ditulis Go (bukan Rust)
- Konteks: UI-SPEC menetapkan `lab serve` Go (cmd/lab) sebagai API+static
  server :8088, sementara CLI `lab` yang ada adalah Rust.
- Keputusan: Go stdlib-only (tanpa dependensi pihak ketiga) untuk server U0;
  CLI Rust tidak diubah kecuali endpoint yang digeser U0.
- Alasan: patuhi spec; stdlib Go cukup untuk static+middleware U0 (tanpa
  risiko supply-chain npm/Go tambahan); biner kecil, build cepat.

### 2026-10-16 — U0: openapi-fetch harus lazy-fetch + MSW path wildcard (vitest)
- Konteks: test AppShell gagal konsisten (query error) padahal MSW handler
  benar; debug 1 jam menunjukkan `createClient` menangkap `globalThis.fetch`
  saat modul di-init (sebelum `server.listen()` mem-patch) sehingga request
  lolos ke gateway nyata :3000 (404).
- Keputusan: `fetch: (...args) => globalThis.fetch(...args)` di `api/client.ts`
  (lookup saat request); handler MSW memakai prefix wildcard `*/api/...`
  (path-only tidak cocok di MSW v3).
- Alasan: robust terhadap urutan init (MSW, HMR, dev-proxy).

### 2026-10-16 — U0: Go hanya +yaml.v3 (revisi stdlib-only)
- Konteks: test kontrak Go harus mem-parsing api/openapi.yaml; stdlib tanpa YAML.
- Keputusan: tambah `gopkg.in/yaml.v3 v3.0.1` (stabil sejak 2022, single dep).
  Selainnya tetap stdlib (net/http mux 1.22+, crypto/rand, embed).
- Alasan: kontrak teruji nyata (paths spec vs handler), bukan duplikasi teks.

### 2026-10-16 — U0: embed ui/dist + --web-dir override
- Konteks: UI-SPEC minta go:embed; dev/test butuh override (testdata, ui/).
- Keputusan: `//go:embed web/dist` (disalin via `make ui-build`;
  `PLACEHOLDER.html` agar embed selalu kompilasi) + `--web-dir` override disk
  + `--dev-proxy` hanya untuk path Vite (/src/*, /@vite/*, ...); index.html
  selalu dari Go (agar injeksi token + nonce CSP jalan di semua mode).

### 2026-10-16 — U0: Grafana geser :3001 -> :3002 (sudah di U0-1)
- Lihat entri "admin gateway pindah :3001" di atas; seluruh referensi Grafana
  (compose, Makefile, README, lab open, payslo defaults, kurikulum,
  troubleshooting) diperbarui dan diverifikasi live.

### 2026-10-16 — U1: NaN Prometheus merusak JSON + writeJSON fail-closed
- Konteks: `TestSeriesShapeDemo` 200 + body kosong. Akar: rasio 0/0
  menghasilkan string "NaN" di range vector; `fmt.Sscan` mem-parse NaN valid,
  lalu `json.Marshal(NaN)` error yang diabaikan (`_ = Encode`) setelah header
  200 terkirim.
- Keputusan: NaN/Inf range -> titik null eksplisit (gap, sesuai kontrak);
  instant NaN/Inf -> `no_data`; `writeJSON` marshal-dulu (gagal -> 500
  problem, tak pernah 200 kosong); Sscan error -> sampel dilewati.

### 2026-10-16 — U1: temuan implementasi metrics/SSE/demo
- NaN Prometheus (rasio 0/0) merusak json.Marshal -> 200 body kosong;
  range-NaN jadi titik null, instant-NaN jadi no_data, writeJSON fail-closed.
- Mode --demo tanpa param = fixture healthy (bukan live-query); ?demo=
  memilih skenario; live+demo = 400.
- SPA fallback: semua path non-aset menyajikan index.html (perlu untuk /map).
- Uji live BCA-down memakai TTL 30s sementara (agar EXPIRED mengalir <60s),
  lalu dikembalikan 600; ubin BCA <0.9 pada t+50s, HTTP 0.87 (divergensi
  terlihat, sesuai U2).

### 2026-10-16 — U2: driver Postgres = pgx/v5, bukan database/sql
- Konteks: `lab serve` (Go) harus membaca Postgres sebagai `lab_ro` (SELECT)
  dan kursor keyset. Awalnya stdlib-only.
- Opsi: A) `database/sql` + driver; B) `jackc/pgx/v5` pool langsung.
- Keputusan: B, pin `github.com/jackc/pgx/v5 v5.8.0`. Modul menetapkan
  `go 1.24.0` (bukan 1.25) agar tidak memaksa bump toolchain.
- Alasan: pgx punya pool + tipe native (uuid/jsonb) & performa; hanya satu
  dependency tambahan, sudah diverifikasi build/test.

### 2026-10-16 — U2: audit aksi = JSONL append-only; aktor dari X-Lab-Actor
- Konteks: UI-SPEC U4 mewajibkan tiap aksi berisiko tercatat (aktor, aksi,
  target, waktu). Tidak ada tabel audit di DB lab (dan `lab_ro` read-only).
- Keputusan: tulis append-only JSONL ke
  `payslo-workspace/lab-audit.jsonl` (gitignored); cache memori saat `--demo`.
  Aktor = header `X-Lab-Actor` (UI mengisi "trainee"/"mentor"); default
  "lab-console".
- Alasan: sederhana, tidak butuh role DB tulis, tidak mengubah skema
  `lab_ro`; JSONL mudah dibaca trainee.

### 2026-10-16 — U2: create pembayaran lewat Envoy, bukan langsung gateway
- Konteks: trainee menekan "Kirim" di UI; metrik HTTP diambil dari Envoy.
- Keputusan: `lab serve` POST ke ingress Envoy (`LAB_GATEWAY_INGRESS_URL`,
  default http://127.0.0.1:8080), bukan gateway :3000 langsung, agar
  transaksi terhitung di dashboard.
- Alasan: konsisten dengan alur produksi; satu-satunya jalur trainee.

### 2026-10-16 — U2: timeline jujur (langkah turunan tanpa timestamp)
- Konteks: DB hanya menyimpan status sekarang; transisi CREATED->PENDING tidak
  punya kolom waktu sendiri.
- Keputusan: langkah `pending` ditandai `derived=true, at=null` (tanpa waktu
  karangan); `callback`/`final`/`webhook` pakai waktu nyata. `traceId=null`
  (gateway tak menyimpan traceparent); tautan log/trace `null` bila obs-full
  mati (bukan error).
- Alasan: prinsip U1 (jujur ke data). Detail: `docs/ASSUMPTIONS.md` U2-4/5.

### 2026-10-16 — U2 (UI): uang integer + Zod, aksi berisiko = ConfirmDialog
- Konteks: UI-SPEC U6 (uang integer IDR) dan U4 (konfirmasi eksplisit).
- Keputusan: `formatIDR`/`amountSchema` menolak pecahan/NaN/out-of-range
  (Intl id-ID); form divalidasi Zod sebelum memanggil API. Traffic/routing/
  release hanya berangkat setelah `ConfirmDialog` (ketik target). /release
  tidak punya GET -> UI menampilkan "Tidak diketahui", tidak menebak.
- Alasan: mencegah bug uang & aksi tak sengaja; menjaga kejujuran data.

### 2026-10-16 — U2: prettier menormalkan file U1 yang belum rapi
- Konteks: `make ui-test` menjalankan `prettier --check .`; `.editorconfig`
  (root) memberi indent 4 untuk `.tsx` dan indent 2 untuk `.ts`/`.json`.
  Beberapa file U1 (client.ts, useLab.ts, pages.tsx, ArchMap, MissionControl,
  e2e) masih indent 2.
- Keputusan: jalankan `prettier --write .` (tanpa perubahan logika) agar gate
  `ui-test` hijau; file baru U2 mengikuti konvensi yang sama.
- Alasan: gate kualitas wajib hijau; format tidak mengubah perilaku.

### 2026-10-16 — U3: bungkus payslo via subprocess (bukan reimplementasi)
- Konteks: `lab serve` (Go) butuh list/detail/validate/plan/apply/drift/
  capabilities PaySLO; logika itu hidup di Rust.
- Opsi: A) Go memanggil ulang logika (duplikasi); B) subprocess `payslo`.
- Keputusan: B. Resolusi biner `LAB_PAYSLO_BIN` -> `<repoRoot>/target/
  debug/payslo` (absolut) -> `payslo` di PATH; CWD repo-root; argumen path
  absolut eksplisit; timeout 90 dtk; output teks di-parse per prefix
  (`would update|applied|up-to-date|drift:|warning|ok:`). Bila biner hilang ->
  503 dengan petunjuk build; bila `DD_API_KEY` hilang -> pesan menyertakan
  `set DD_API_KEY=lab-only-dd-api-key`.
- Alasan: satu sumber kebenaran (dogfooding CLI); Go hanya penerjemah
  teks->JSON + penjaga keamanan path. Harga: 1-2 exec per request detail
  (~50-200 mdtk, dapat diterima untuk lab).

### 2026-10-16 — U3: validasi dua lapis (Go berlokasi + CLI otoritatif)
- Konteks: `payslo validate` mengembalikan teks tanpa baris/kolom; UI butuh
  error berlokasi.
- Keputusan: Go memeriksa dulu dengan posisi dari `yaml.Node` (sintaks,
  required/unknown fields, tipe, durasi, short<long<=window, runbook di dalam
  workspace, backend enum, rentang prometheus); bila bersih, jalankan
  `payslo validate` sebagai otoritas (skema JSON + aturan backend). Bila Go
  menemukan isu, CLI dilewati (hemat exec, pesan lebih berguna).
- Alasan: lokasi presisi untuk UX; tidak ada divergensi diam-diam karena CLI
  tetap menjadi penentu akhir bila Go lolos.

### 2026-10-16 — U3: nama SLO boleh beda dari nama berkas
- Konteks: `success-rate.yml` berisi `name: payment-success-rate`; asumsi
  `<slug>.yml` gagal (detail 404 padahal list ada).
- Keputusan: resolver mencari `<slug>.yml` dulu, bila tak ada memindai
  direktori dan mencocokkan field `name:`; save menulis ke berkas yang sudah
  ada untuk nama itu (tidak membuat duplikat yang memicu DuplicateName di
  `load_dir`).
- Alasan: menghormati workspace nyata trainee; satu nama = satu berkas.

### 2026-10-16 — U3: diff LCS di Go + `diff` di klien; datadog tanpa diff teks
- Konteks: `payslo plan` hanya memberi flag perubahan (hash), bukan diff
  untuk DiffViewer; `render --backend datadog` ditolak CLI.
- Keputusan: backend menyertakan desired/actual/DiffViewer-unified (LCS,
  batas 2000 baris) untuk koneksi prometheus; untuk datadog hanya flag +
  catatan jujur ("terdeteksi via hash kanonik"). UI menghitung diff baris
  sendiri via paket `diff` dari teks backend.
- Alasan: plan tetap "menunjukkan diff" (prometheus) tanpa mengarang diff
  datadog yang tak tersedia.

### 2026-10-16 — U3: NBSP dari contenteditable dinormalkan di editor
- Konteks: e2e Chromium membuktikan browser menyisipkan U+00A0 untuk spasi
  ganda di CodeMirror; indentasi YAML diam-diam menjadi kunci tak dikenal
  (`"  good"`), validasi gagal misterius.
- Keputusan: `normalizeEditorText` (U+00A0 -> spasi, ditulis `\u00a0` agar
  lolos no-irregular-whitespace) dijalankan tiap perubahan di `SloEditor`;
  server tetap ketat (menolak NBSP dengan lokasi) sebagai jaring pengaman.
- Alasan: YAML tak pernah butuh NBSP; normalisasi aman dan menyelamatkan
  trainee dari bug tak kasatmata.

### 2026-10-16 — U3: CodeMirror malas + stabil; mock di jsdom
- Konteks: CodeMirror 6 butuh API layout (getClientRects) yang tak ada di
  jsdom (vitest crash); bundle editor ~434 KB.
- Keputusan: `React.lazy` (chunk terpisah, hanya di /slo) + fallback
  textarea + ErrorBoundary; ekstensi editor identitas stabil (StateField +
  dispatch via ref, bukan rekonfigurasi per ketikan — rekonfigurasi per
  render pernah merusak seleksi); test Vitest mem-mock modul editor,
  pengetikan asli dicakup Playwright/Chromium.
- Alasan: halaman non-SLO tak terbebani; test tetap deterministik.

### 2026-10-16 — U3: tombol Apply selalu aktif (demi no-op yang terlihat)
- Konteks: menonaktifkan Apply saat plan kosong justru membuat no-op tak
  pernah terdemonstrasikan.
- Keputusan: Apply aktif selama tidak sedang berjalan (dengan `title`
  penjelas); hasil `up-to-date ... (no changes)` dirender sebagai banner
  "no-op" tegas + per-item ✓.
- Alasan: memenuhi "pastikan UI menampilkan no-op jelas saat apply kedua".

### 2026-10-16 — U3: role=alert pindah ke pembungkus div
- Konteks: axe `list` rule: `<ul>` hanya boleh berisi `<li>` tanpa role
  override; `<li role="alert">` di panel drift/validasi/alert = pelanggaran
  serious.
- Keputusan: `<div role="alert"><ul><li>…` di ketiga tempat (SloEditor,
  DriftPanel, daftar alert).
- Alasan: live-region tetap diumumkan pembaca layar; struktur list valid.

### 2026-10-16 — U4: approval + notes + detail hidup di payslo-server (Rust)
- Konteks: kontrak 5b hanya janji (URL ntfy 404, `payslo-runbook` skeleton);
  tabel `approval_requests`/`timeline` sudah ada tapi tanpa penulis.
- Keputusan: tambah endpoint Rust minimal (`GET /incidents/{id}`,
  `POST /incidents/{id}/notes`, `GET|POST /approvals`,
  `POST /approvals/{id}/decision`) + migrasi `0002_approval_reason.sql`
  (idempoten, toleran duplicate-column untuk DB lama). Keputusan approval
  diteruskan ke PaySLO; lab serve hanya proksi + eksekutor.
- Alasan: "diteruskan ke PaySLO" menjadi literal; state approval survive
  restart; audit append-only PaySLO mencatat semua keputusan.

### 2026-10-16 — U4: runbook tereksekusi milik lab serve (YAML per-SLO)
- Konteks: tak ada format runbook tereksekusi di PaySLO; workspace hanya
  punya `.md` manusia.
- Keputusan: `payslo-workspace/runbooks/<slo>.yml` (`kind: Runbook`) milik
  lab serve: diagnostic = `promql` (nama allowlist, bukan query bebas) atau
  `http` (GET loopback saja); remediation = HANYA `lab_routing` + `impact`
  wajib + `requires_approval: true` ditolak saat load bila tidak.
- Alasan: browser tak pernah mengirim PromQL/URL bebas; remediation tanpa
  approval mustahil lolos load; selaras P3 (mitigasi = routing manual).

### 2026-10-16 — U4: tanpa endpoint eksekusi remediation (anti jalan pintas)
- Konteks: UI-SPEC U5 + tugas "tidak ada jalan pintas".
- Keputusan: tak ada `POST .../run`; eksekusi terjadi sinkron di
  `DecideApproval` SETELAH PaySLO mencatat approved (plus catatan timeline
  hasil). `POST .../steps/{idx}/run` hanya untuk diagnostic (remediation ->
  409, manual -> 400). Idempoten: routing sudah sesuai -> no-op jujur.
- Alasan: secara konstruksi tak ada jalur UI/server yang mengeksekusi tanpa
  approval tercatat; diuji (409 langsung, 409 ganda).

### 2026-10-16 — U4: postmortem + settings milik lab serve (file JSON)
- Konteks: PaySLO tak punya postmortem/settings; UI-SPEC butuh editor +
  draft/published + ekspor Markdown; approval butuh identitas approver.
- Keputusan: `payslo-workspace/lab-console/{postmortems/<inc>.json,
  settings.json}` (ID insiden tervalidasi `inc-[0-9a-f-]` -> aman untuk nama
  berkas); Markdown di-render server-side deterministik (satu sumber untuk
  pratinjau + ekspor).
- Alasan: tanpa migrasi DB; konsisten dengan audit JSONL lab serve.

### 2026-10-16 — U4: e2e serial + snapshot diregenerasi (browser 1248)
- Konteks: `playwright install` mengunduh headless-shell-1248 (sebelumnya
  1243); widget `<select>` native render berbeda -> snapshot mission/map
  gagal padahal konten identik (terbukti beda piksel hanya x1032-1262/y16-31);
  spec paralel berbagi satu server demo (approval spec insiden membocorkan
  badge ke screenshot lain).
- Keputusan: `ui-e2e.sh` memakai `--workers=1` (satu server demo dipakai
  bersama); snapshot diregenerasi dengan browser terpin dan diverifikasi
  konten-identik; revisi browser dicatat (ASSUMPTIONS U4-6).
- Alasan: determinisme > kecepatan untuk 7 spec (~11 dtk); bump playwright
  wajib verifikasi ulang snapshot.

### 2026-10-16 — U6: SQLite di lab serve (modernc.org/sqlite)
- Konteks: progres checklist Belajar + penilaian manual mentor harus bertahan
  restart `lab serve` (acceptance U6). pilihan: (a) JSON file seperti
  postmortem/settings, (b) SQLite, (c) library di crates/lab via CLI.
- Keputusan: SQLite via `modernc.org/sqlite v1.38.1` (pure Go, tanpa cgo) di
  `payslo-workspace/lab-console/lab.db`.
- Alasan: sesuai spesifikasi AGENTS.md (SQLite untuk state lab CLI); tanpa cgo
  menjaga build statis; satu file untuk progress + assessment; `sqlx` tidak
  bisa dipakai bersama `rusqlite` (konflik libsqlite3-sys, lihat FASE 1).
- Dampak: +1 dependensi Go; waktu build lab serve +~2 dtk (ukuran binary +~3 MB).

### 2026-10-16 — U6: renderer Markdown aman tanpa raw HTML
- Konteks: kurikulum Markdown harus di-render ke HTML yang aman (acceptance:
  `<script>`, event handler, `javascript:`, iframe disanitasi). pilihan: (a)
  library markdown + DOMPurify, (b) renderer kustom dengan escape total.
- Keputusan: renderer kustom `cmd/lab/markdown.go` — SEMUA raw HTML di-escape
  (tidak ada passthrough); hanya tag renderer sendiri yang muncul; link hanya
  http/https/mailto. DOMPurify (`dompurify@3.4.17`) tetap jalan di UI sebagai
  lapisan kedua sebelum `dangerouslySetInnerHTML`.
- Alasan: tanpa dependensi baru untuk library markdown; escape total =
  sanitasi terkuat yang bisa dibuktikan dengan unit test; DOMPurify menutup
  celah jika renderer kustom bug. Kurikulum tidak butuh tabel/gambar.

### 2026-10-16 — U6: gate PIN mentor = pembatas spoiler
- Konteks: mode mentor harus dipin via `MENTOR_PIN`. pilihan: (a) auth
  sungguhan (session/JWT), (b) token sederhana + rate limit.
- Keputusan: (b) — token acak di memori server, dikirim client sebagai
  `X-Mentor-Token`; 5 salah -> jeda 5 menit (429 + Retry-After). Dokumentasi
  jujur di mentor-guide: ini PEMBATAS SPOILER, bukan keamanan (trainee tetap
  bisa curl admin bank-sim langsung — honor system).
- Alasan: sesuai UI-SPEC §KEAMANAN 6; auth sungguhan di luar scope lab;
  rate limit mencegah brute-force casual.

### 2026-10-16 — U7: deteksi gagal-embed Grafana via fetch probe
- Konteks: `onError` iframe tidak andal untuk navigasi yang gagal, dan
  `onLoad` bisa menyala untuk halaman error/X-Frame-Options. Acceptance
  menuntut fallback yang muncul saat embedding salah dikonfigurasi.
- Keputusan: `GrafanaFrame` memakai probe `fetch(src, {mode:'no-cors'})` —
  rejection = network gagal -> panel fallback + tombol "Buka di tab baru"
  (tombol ini SELALU tampil di bawah iframe untuk kasus X-Frame-Options
  yang tak terdeteksi otomatis). Timeout 10 dtk sebagai jaring pengaman.
- Alasan: sinyal jaringan eksplisit > event iframe yang ambigu; fallback
  link permanen menutup celah deteksi.

### 2026-10-16 — U7: ambang data-sedikit di server (settings)
- Konteks: ambang 0.2/s sebelumnya konstanta UI (`LOW_SAMPLE_RPS`); spek
  menuntut pengaturan ambang di halaman Pengaturan.
- Keputusan: `PUT /api/v1/settings` menerima `lowSampleThreshold` opsional
  (0..10, tersimpan di settings.json); MissionControl memakai nilai server
  dengan fallback konstanta bila belum diatur. `Set` nama tidak menimpa
  threshold (dan sebaliknya).
- Alasan: satu sumber kebenaran; backward-compat (field opsional).

### 2026-10-16 — U7: laporan diagnostik bebas secret by-construction + scan
- Konteks: laporan boleh disalin ke mentor; tidak boleh ada token/password.
- Keputusan: handler hanya menyusun field aman (nama, ref image/versi
  buildinfo publik, reachable, latensi, key config TANPA nilai — bahkan
  `config` selalu `{}`); kredensial Grafana hanya dipakai server-side untuk
  basic-auth dan tidak pernah diserialkan. Test `TestDiagnosticsSecretFree`
  memindai JSON untuk pola sensitif.
- Alasan: tidak mengandalkan disiplin manual; pola yang dipindai
  didokumentasikan di test.

### 2026-10-16 — U7: regenerasi golden snapshot (menu Mentor hilang)
- Konteks: `map-demo`/`mission-demo` golden menampilkan item nav "Mentor";
  perilaku U6 yang benar menyembunyikannya untuk trainee. Bukan perubahan
  visual U7 — golden basi sejak U6 (e2e U6 tidak menjalankan spec snapshot).
- Keputusan: regenerasi via `--update-snapshots` setelah verifikasi gambar
  aktual identik kecuali hilangnya item "Mentor".
- Alasan: golden harus mencerminkan perilaku yang dimaksud; dicatat agar
  tidak dikira regresi U7.
