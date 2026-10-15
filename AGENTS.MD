# PERAN
Kamu adalah senior SRE + Rust engineer. Kamu membangun "PaySLO Lab": lab simulasi
payment gateway yang berjalan 100% lokal dengan Docker Compose, untuk onboarding SRE baru,
sekaligus menjadi lingkungan uji untuk tool open source "PaySLO" (SLO-as-code multi-provider
+ incident management + runbook automation).

# TUJUAN AKHIR (Definition of Done proyek)
Dari clone bersih, di laptop (4 CPU / 8 GB RAM, Linux/macOS/Windows-WSL2, amd64 dan arm64):
1. `make up-lite` menyalakan seluruh stack tanpa error, tanpa akun cloud/internet selain pull image
   dan download crate saat build.
2. `make smoke` lulus (end-to-end: transaksi -> metric -> SLO -> alert -> incident -> notifikasi).
3. Trainee bisa menjalankan `lab scenario start 01`, mendeteksi insiden lewat alert, memitigasi,
   lalu `lab verify 01` memberi skor.
4. Semua dokumentasi (README, curriculum, mentor guide, troubleshooting) ada dan akurat.
5. Tidak ada secret/data asli. Semua data sintetis.

# NON-GOALS
- Bukan produk produksi. Tidak ada Kubernetes, tidak ada cloud, tidak ada Datadog asli.
- Tidak membuat autentikasi/otorisasi kelas produksi (cukup basic untuk lab, ditandai jelas).

# ATURAN KERJA (WAJIB)
1. Kerjakan HANYA fase yang sedang diminta. Setelah selesai, berhenti dan laporkan:
   apa yang dibuat, cara memverifikasi, hasil verifikasi nyata, dan hal yang belum selesai.
2. Jangan klaim "sudah jalan" tanpa benar-benar menjalankan perintah verifikasi dan menampilkan
   outputnya. Jika tidak bisa menjalankan (mis. tidak ada Docker), katakan dengan jelas.
3. Jangan mengarang versi image, versi crate, nama field API, atau opsi konfigurasi. Cek sumber
   resmi (docs.rs, crates.io, dokumentasi vendor) yang tersedia. Pin versi spesifik; DILARANG
   `latest` untuk image dan DILARANG wildcard (`*`) untuk crate. Jika ragu, catat di
   `docs/ASSUMPTIONS.md` beserta cara memverifikasinya.
4. Satu perubahan = satu commit kecil dengan pesan Conventional Commits.
5. Tulis test bersamaan dengan kode. Fase tidak selesai jika test belum hijau.
6. Jika ada keputusan desain yang tidak tercantum di sini, ambil opsi paling sederhana,
   catat di `docs/DECISIONS.md` (konteks, opsi, keputusan, alasan), lalu lanjut. Hanya bertanya
   jika benar-benar terblokir.
7. Jangan menambah fitur di luar spesifikasi. Fitur tambahan dicatat di `docs/BACKLOG.md`.
8. Jangan mengubah file di luar scope fase kecuali perlu untuk membuatnya jalan, dan sebutkan.

# TECH STACK (tetap, jangan diganti)
## Bahasa dan toolchain
- Rust stable, versi dipin lewat `rust-toolchain.toml` (komponen: rustfmt, clippy). Edition terbaru
  yang didukung toolchain tersebut. `Cargo.lock` WAJIB di-commit (ini aplikasi/binary).
- Satu Cargo workspace di root untuk SEMUA komponen kustom: gateway-api, bank-sim, merchant-sink,
  datadog-mock, loadgen-helper (jika perlu), payslo (beberapa crate), lab CLI.
- Konvensi kode: `#![forbid(unsafe_code)]` di semua crate; lint workspace menolak `unwrap`/`expect`/`panic`
  di kode non-test (`clippy::unwrap_used`, `clippy::expect_used`, `clippy::panic` = deny; diizinkan
  di test dan di `main` hanya untuk inisialisasi dengan pesan jelas). Error: `thiserror` di library,
  `anyhow` hanya di binary/CLI.

## Crate yang diizinkan (tambahan lain wajib dicatat di DECISIONS.md)
- Async/HTTP server: `tokio`, `axum`, `tower`, `tower-http`.
- HTTP client: `reqwest` dengan fitur `rustls-tls` (DILARANG OpenSSL/native-tls, demi build statis
  dan multi-arch).
- Serialisasi: `serde`, `serde_json`. YAML: pilih crate yang MASIH DIPELIHARA (periksa status
  `serde_yaml`, crate itu sudah di-archive; pilih pengganti yang aktif dan catat alasannya di DECISIONS.md).
- DB: `sqlx` (fitur postgres, runtime-tokio, rustls, migrate) untuk Postgres; `rusqlite` (fitur
  `bundled`) untuk SQLite milik PaySLO dan state lab CLI.
- Logging: `tracing` + `tracing-subscriber` (format JSON ke stdout). Header `traceparent` diteruskan
  manual (jangan menambah SDK OpenTelemetry ke gateway, lihat P1 di bawah).
- CLI: `clap` (derive) + `clap_complete` untuk shell completion.
- Kripto: `hmac`, `sha2`, `subtle` (perbandingan constant-time), `hex`.
- Acak ber-seed: `rand` + `rand_chacha` (WAJIB untuk perilaku yang harus reproducible).
- Validasi JSON Schema: crate `jsonschema`. Waktu: `time` atau `chrono` (pilih satu, catat).
- HTML server-rendered untuk UI PaySLO: `askama` (template terkompilasi). Tanpa framework JS berat.
- Test: `insta` (snapshot/golden), `wiremock` atau server axum in-process untuk mock HTTP,
  `tokio::test`, `testcontainers` HANYA jika benar-benar perlu (lebih baik pakai docker compose di smoke).
- `async-trait` untuk trait adapter yang dipakai sebagai `dyn` (async fn in trait native belum
  object-safe untuk dyn). Catat di DECISIONS.md.

## Image dan build
- Satu `Dockerfile` workspace di root dengan `cargo-chef` (cache dependensi) dan SATU stage builder
  yang meng-compile seluruh workspace sekali (`cargo build --release --workspace`), lalu stage runtime
  per binary (`--target gateway-api`, `--target bank-sim`, dst.) berbasis distroless/static atau alpine,
  user non-root. Target build: musl (`x86_64-unknown-linux-musl` dan `aarch64-unknown-linux-musl`)
  agar binary statis. Multi-arch lewat buildx.
- Profil release: `opt-level=2` (atau 3 jika waktu build wajar), `lto="thin"`, `codegen-units`
  disesuaikan, `strip=true`, `panic="abort"` hanya jika tidak merusak test.
- Batasi paralelisme build agar tidak OOM di 8 GB: dokumentasikan `CARGO_BUILD_JOBS` dan setting
  Docker memory. Ukur dan catat waktu build bersih dan incremental di README.
- Sediakan DUA jalur menjalankan lab: (a) `make up-lite` yang build lokal; (b) `make up-prebuilt`
  yang menarik image publik dari GHCR hasil CI (tag ber-versi, bukan latest). Jalur (b) adalah
  default di quickstart untuk trainee agar tidak menunggu kompilasi.
- Database aplikasi: PostgreSQL. Load generator: k6 (image resmi) + script JS.
- Observability: Prometheus, Alertmanager, Grafana, Loki, Tempo, Grafana Alloy (pengumpul log
  Docker), sql_exporter, Envoy (ingress + tracing). Notifikasi: ntfy.
- Orkestrasi: Docker Compose v2 dengan `profiles`. Otomasi: Makefile + `lab` CLI.

# ARSITEKTUR
Alur: loadgen -> Envoy(:8080) -> gateway-api -> {bank-bca, bank-mandiri, ewallet-sim}
-> callback async ke gateway-api -> webhook ke merchant-sink.
Metrik: Envoy /stats/prometheus + sql_exporter (query read-only ke Postgres).
Log: stdout JSON -> Alloy -> Loki. Trace: Envoy -> Tempo (propagasi header `traceparent`).
Alert: Prometheus -> Alertmanager -> PaySLO (webhook) -> incident -> ntfy + runbook.
PaySLO apply SLO ke: Prometheus (file rules + reload) dan datadog-mock (HTTP API).

# PRINSIP DESAIN YANG HARUS DIPATUHI
P1. `gateway-api` SENGAJA TIDAK punya endpoint metrics Prometheus/instrumentasi metrik kustom
    (DILARANG menambah crate `prometheus`, `metrics`, atau exporter OTel di crate ini; cek di CI
    dengan `cargo deny`/grep). SLI diambil dari Envoy (HTTP) dan sql_exporter (status bisnis).
    Gateway hanya boleh: log JSON terstruktur dan meneruskan header `traceparent`.
P2. HTTP 200 TIDAK berarti pembayaran sukses. Gateway mengembalikan 200 untuk pembayaran yang
    diterima meski kemudian FAILED/PENDING. Hanya error internal/timeout -> 5xx.
P3. Cacat sengaja (intentional lab flaws), harus didokumentasikan di mentor-guide, bukan disembunyikan:
    - callback duplikat TIDAK di-dedup oleh gateway (untuk skenario 03).
    - tidak ada circuit breaker otomatis; mitigasi lewat admin routing manual.
    Beri komentar `// LAB-FLAW:` di lokasi kodenya agar mudah dilacak.
P4. Keamanan lab: semua port di-bind ke 127.0.0.1. Kredensial lab jelas bertanda "lab-only".
    User DB `lab_ro` hanya SELECT. Tidak ada data nyata. docker.sock hanya di-mount `:ro` dan hanya
    untuk Alloy, dengan komentar peringatan.
P5. Semua perintah harus idempotent dan aman dijalankan ulang (`make up` dua kali tidak error).
P6. Resource: pasang `mem_limit` wajar per service agar total profil `up-lite` <= ~5 GB
    (runtime Rust biasanya kecil; ukur dan catat).
P7. Money: nominal uang SELALU integer (i64 satuan terkecil IDR). DILARANG f32/f64 untuk uang.
    Gunakan newtype (mis. `Amount(i64)`) agar tidak tercampur.
P8. State machine transaksi dimodelkan sebagai enum Rust dengan transisi yang dicek di kompilator/test
    (transisi ilegal tidak boleh bisa dibuat). Status yang disimpan di DB dikonversi lewat enum, bukan string bebas.

# SPESIFIKASI DOMAIN

## Database (apps/gateway-api, db/schema.sql)
Tabel `transactions`: id uuid PK, merchant_id text, order_id text, channel text
(VA|EWALLET|QRIS), provider text (bca|mandiri|ewallet), amount bigint (IDR, integer, DILARANG float),
fee bigint, status text (CREATED|PENDING|SUCCESS|FAILED|EXPIRED), provider_ref text,
created_at, updated_at, expires_at timestamptz. UNIQUE(merchant_id, order_id).
Index (status, created_at), (provider, status).
Tabel `callback_log`: id, transaction_id, provider, payload jsonb, received_at.
Tabel `webhook_delivery`: id, transaction_id, attempt int, http_status int, status
(DELIVERED|FAILED), created_at.
Role `lab_ro`: hanya SELECT pada tabel di atas. seed.sql: data historis sintetis (deterministik).
Migrasi memakai `sqlx migrate` (folder migrations/), dan `schema.sql` yang dimount ke Postgres harus
konsisten dengan migrasi (tambahkan test yang membandingkan hasil keduanya).
Query sqlx: gunakan query runtime biasa atau `sqlx::query!` dengan mode offline (`cargo sqlx prepare`,
file `.sqlx/` di-commit) agar build di Docker tidak butuh DB hidup.

## gateway-api (port 3000)
- POST /v1/payments  body {merchant_id, order_id, amount, channel}; header opsional Idempotency-Key
  (hanya dicatat, TIDAK ditegakkan untuk callback; untuk create boleh dihormati via UNIQUE).
  Router memilih provider berdasarkan channel + provider yang enabled + bobot
  (VA: bca 60 / mandiri 40; EWALLET & QRIS: ewallet 100). Memanggil bank-sim POST /charge dengan
  timeout (default 5s). Respons 200 {id, status:"PENDING"|"FAILED", ...}.
- GET /v1/payments/{id}
- POST /v1/callbacks/{provider}  menerima callback bank -> update status final -> kirim webhook ke
  merchant-sink (HMAC-SHA256 header X-Signature + X-Timestamp, retry 3x backoff).
- Job background (tokio task): PENDING melewati expires_at -> EXPIRED (default 10 menit; di lab
  dapat dipercepat via env).
- Admin (hanya jaringan internal + 127.0.0.1): GET/PUT /admin/routing (enable/disable provider,
  bobot); PUT /admin/release {version: "good"|"bad"} ("bad" = 20% respons 500 + latency tambahan,
  untuk skenario regresi deploy).
- GET /healthz, /readyz. Log JSON memuat transaction_id, provider, status, trace_id (dari traceparent).
- Graceful shutdown (SIGTERM) dan timeout di setiap panggilan keluar.

## bank-sim (port 4000, satu binary, dipakai 3 instance via env PROVIDER_NAME)
- POST /charge -> {provider_ref}; callback async ke gateway sesuai profil.
- GET /status/{provider_ref}  (dipakai trainee untuk cek status "ke bank").
- Admin: GET/PUT /admin/profile {profile, error_rate, latency_ms_min, latency_ms_max, callback_delay_s,
  duplicate_callbacks}, POST /admin/reset.
- Profil: normal (100-300ms, sukses 99.5%), slow (5-15s), flaky (error 5xx/timeout acak sesuai
  error_rate), down (tidak merespons), ambiguous (bank memproses sukses tapi response ke gateway
  timeout), late-callback (callback tertunda), dup-callback (callback 2-3x).
- Perilaku acak memakai `ChaCha8Rng` dengan seed dari env SEED supaya reproducible.

## merchant-sink (port 5000)
- POST /webhook: verifikasi HMAC (constant-time); catat ledger saldo per order (kredit TIAP webhook
  diterima, sehingga duplikat tampak sebagai saldo ganda). GET /state (saldo + daftar event).
- Admin: PUT /admin/mode {status: 200|500, latency_ms}.

## datadog-mock (port 8126)
Meniru subset API Datadog: POST/GET/PUT/DELETE /api/v1/slo dan /api/v1/monitor, validasi header
DD-API-KEY (nilai lab), penyimpanan in-memory + persist ke file, ID dibuat server, dukungan list
dengan filter tags, dan endpoint /api/v1/slo/{id}/history sederhana. Wajib didokumentasikan bahwa
ini tiruan perilaku, bukan Datadog asli, dan field yang diverifikasi dari dokumentasi resmi dicatat
di docs/ASSUMPTIONS.md.

## PaySLO (crate-crate di ./payslo/crates, bagian dari workspace)
Pembagian crate: `payslo-model` (tipe + loader + validator + JSON Schema), `payslo-adapter`
(trait + utilitas bersama), `payslo-adapter-prometheus`, `payslo-adapter-datadog`, `payslo-testkit`
(contract test suite generik), `payslo-runbook`, `payslo-server` (axum + UI + SQLite), `payslo-cli`
(binary `payslo`).
Model kanonik YAML `payslo/v1` kind SLO (objective, window, sli{type,good,total,source},
alerting{burnRates[],runbook}, targets[{backend,overrides}]). JSON Schema di
payslo/schema/v1/slo.json. Validasi tambahan di kode: short<long, window valid per backend,
runbook ada, nama unik. Gunakan `#[serde(deny_unknown_fields)]`.
CLI: `payslo validate|render|plan|apply|drift|capabilities|serve`.
Trait adapter SLO: name, capabilities, validate, render, plan, apply, delete, status
(async, `#[async_trait]`, mengembalikan `Result<_, AdapterError>` dengan thiserror).
Adapter: prometheus (tulis rules ke direktori + panggil /-/reload), datadog (HTTP ke datadog-mock;
urutan: SLO dulu, baru monitor). Contract test suite = fungsi generik
`run_contract_suite(adapter_factory)` di payslo-testkit yang dipanggil dari test SETIAP adapter.
Server (`payslo serve`, port 8081): endpoint /webhooks/alertmanager dan /webhooks/datadog ->
CanonicalAlert -> dedup & korelasi (slo+service) -> Incident -> responder ntfy -> runbook engine.
Runbook YAML: steps bertipe diagnostic|remediation|manual; executor (trait): http, promql,
datadog_query, lab_routing. ATURAN KERAS: step remediation wajib requires_approval=true (ditegakkan
di kode saat load, via tipe yang tidak memungkinkan remediation tanpa approval, dan di DB CHECK).
Approval lewat HTTP endpoint + tombol aksi ntfy + halaman web minimal.
Penyimpanan: SQLite (rusqlite, WAL mode; akses dari async lewat `spawn_blocking` atau pool dedicated).
Audit log append-only (trigger SQLite melarang UPDATE/DELETE).
UI web minimal (askama): daftar incident, timeline, antrean approval, status SLO per backend.
Sumber SLI di lab: `metric` (metric Envoy dan hasil sql_exporter).

## Konfigurasi waktu untuk lab
Scrape interval 5s, sql_exporter min_interval 5s. SLO lab memakai window pendek supaya
trainee tidak menunggu berjam-jam, contoh burn rate: 5m/1m (page), 15m/5m (page), 1h/10m (ticket).
Sediakan SATU contoh SLO bergaya produksi (30d, 1h/5m) hanya untuk dibaca/render, tidak untuk dites live.

## lab CLI (crate `lab`, binary `lab`)
Perintah: up, down, reset, status, open; chaos <target> ...; traffic set --rate N; routing
enable|disable <provider>; deploy gateway --version good|bad (juga mengirim annotation ke Grafana);
scenario start|stop|solution|list|random --seed; hint <id>; answer <id> "<teks>"; verify <id>;
mentor timeline|inject; snapshot save|load.
Skenario didefinisikan di scenarios/<id>/scenario.yml. DSL check BERTIPE: modelkan sebagai
`enum Check` dengan `#[serde(tag = "type")]` (JANGAN eval string bebas). Tipe: prometheus_query
(query, operator, nilai), incident_exists, routing_state, answer_contains, sli_above, duration_lt (soft).
Skor: lihat rubrik di curriculum. `lab` memanggil Docker lewat `docker compose` sebagai subprocess
(bukan library Docker), dengan error yang jelas jika Docker tidak ada.

# STRUKTUR REPO
```
payslo-lab/
├── Cargo.toml (workspace)  Cargo.lock  rust-toolchain.toml  deny.toml  clippy.toml  rustfmt.toml
├── Dockerfile (workspace, cargo-chef, target runtime per binary)
├── Makefile  docker-compose.yml  docker-compose.prebuilt.yml  .env.example  README.md  LICENSE(Apache-2.0)
├── AGENTS.md  docs/{DECISIONS,ASSUMPTIONS,BACKLOG,VERSIONS,mentor-guide,troubleshooting}.md
├── crates/lab/                      # CLI lab
├── apps/{gateway-api,bank-sim,merchant-sink,datadog-mock}/   # crate Rust
├── apps/loadgen/                    # script k6 (JS) + Dockerfile
├── payslo/{crates/*,schema,templates}/
├── config/{envoy,prometheus,alertmanager,grafana,loki,tempo,alloy,sql_exporter}/
├── payslo-workspace/{slo,runbooks,connections.yml,generated}/   # dimiliki trainee
├── db/{schema.sql,seed.sql,migrations/}
├── scenarios/<id>/{scenario.yml,README.md,hints.md,solution.md}
├── curriculum/  .github/workflows/  scripts/
```

# DOCKER COMPOSE PROFILES
default: postgres, gateway-api, bank-bca, bank-mandiri, ewallet-sim, merchant-sink, envoy
traffic: loadgen | obs: prometheus, alertmanager, grafana, sql-exporter
obs-full: loki, tempo, alloy | payslo: payslo, datadog-mock, ntfy
`make up-lite` = default+traffic+obs+payslo (build lokal). `make up-prebuilt` = sama, tapi image dari
registry. `make up` = semua profil. Semua service punya healthcheck dan depends_on condition
service_healthy. Port host di-bind 127.0.0.1. Healthcheck untuk image distroless: sediakan subcommand
`--healthcheck` pada binary (karena tidak ada curl/shell di image).

# STANDAR KUALITAS
- `cargo fmt --check`, `cargo clippy --workspace --all-targets -- -D warnings`, `cargo test --workspace`
  hijau. `cargo deny check` (lisensi, advisories, larangan crate tertentu) dan `cargo audit` bersih.
- Test unit untuk logika; snapshot test (`insta`) untuk renderer; integration test dengan docker compose
  untuk smoke. Coverage logika inti >= 70% (`cargo llvm-cov`).
- Setiap README komponen memuat: fungsi, cara run, env var, contoh curl.
- Pesan error jelas dan actionable. Tidak ada panic/unwrap di jalur normal.
- Semua skrip shell: `set -euo pipefail`, lulus shellcheck.
- Hindari dependensi berlebihan (waktu kompilasi!); setiap dependensi baru dicatat di DECISIONS.md
  dengan alasan dan perkiraan dampak ke waktu build.