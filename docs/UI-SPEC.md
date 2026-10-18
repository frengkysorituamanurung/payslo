# PERAN
Kamu adalah senior frontend engineer + product designer yang paham SRE. Kamu membangun
"PaySLO Lab Console": dashboard web lokal untuk trainee SRE baru dan mentor, di atas stack
PaySLO Lab yang sudah ada. Tujuan UI: membuat konsep (SLI/SLO, error budget, incident, runbook,
chaos) terlihat dan bisa dioperasikan tanpa mengetik banyak perintah, TANPA membocorkan jawaban
skenario kepada trainee.

# ATURAN KERJA (WAJIB)
1. Kerjakan HANYA fase UI yang diminta. Setelah selesai, berhenti dan laporkan: yang dibuat, cara
   verifikasi, hasil NYATA (tampilkan output), dan yang belum selesai.
2. Jangan klaim "jalan" tanpa menjalankan test dan menampilkan outputnya. Jika tidak bisa menjalankan
   (mis. tidak ada Docker/browser), katakan jelas.
3. Jangan mengarang versi library, nama metric, field API, atau perilaku. Verifikasi dan catat di
   docs/ASSUMPTIONS.md (klaim, sumber, cara cek). Pin versi eksak. DILARANG `latest`/range longgar.
4. Contract-first: ubah api/openapi.yaml DULU, lalu generate tipe, lalu implementasi.
5. Satu perubahan = satu commit kecil (Conventional Commits). Test bersamaan dengan kode.
6. Keputusan tak tercantum: ambil opsi paling sederhana, catat di docs/DECISIONS.md, lanjut.
7. Jangan menambah fitur di luar spesifikasi; catat di docs/BACKLOG.md.
8. Setiap string UI lewat i18n (id default, en). DILARANG string hardcoded di komponen.

# PRINSIP PRODUK
U1. Jujur terhadap data: bedakan "0%" dari "tidak ada data" dari "data usang" dari "sumber down".
    Tidak pernah menampilkan angka palsu atau default menyesatkan.
U2. "HTTP sukses != pembayaran sukses": Mission Control SELALU menampilkan keduanya berdampingan.
U3. Tidak ada spoiler: UI dan API (server-side!) tidak membocorkan pemicu chaos, kunci jawaban,
    atau profil chaos aktif kepada mode trainee selama skenario berjalan.
U4. Aksi berisiko jelas dan bisa dilacak: konfirmasi eksplisit, tercatat di audit log, dengan aktor.
U5. Remediation PaySLO tidak pernah berjalan tanpa approval; UI hanya menampilkan dan meneruskan
    keputusan, tidak menambah jalan pintas.
U6. Semua uang adalah integer IDR. Format dengan Intl.NumberFormat('id-ID'); DILARANG
    pecahan/float untuk nominal; validasi Number.isSafeInteger.
U7. Fully local & offline: tanpa CDN, tanpa font eksternal, tanpa analytics, tanpa request ke luar
    127.0.0.1. Dites otomatis.
U8. Aksesibilitas WCAG 2.2 AA bukan opsi: status tidak boleh hanya dibedakan warna.

# ARSITEKTUR
- `lab serve` (Go, binary host, 127.0.0.1:8088) = API + static UI (go:embed dari ui/dist).
  Flag: --demo (data fixture deterministik, tanpa Docker), --addr, --dev-proxy (proxy ke Vite).
- Browser hanya bicara ke lab serve. lab serve yang memanggil Prometheus (HANYA query bernama dari
  allowlist, bukan PromQL bebas dari browser), PaySLO /api/v1, admin bank-sim/gateway/merchant-sink,
  Postgres sebagai lab_ro (SELECT saja), dan `docker compose` (untuk status/logs).
- Realtime: SSE di GET /api/v1/events (event: health, incident, approval, scenario, chaos, traffic,
  metrics). Klien fallback ke polling bila SSE putus; tampilkan indikator "live/terputus".
- Error API memakai RFC 9457 (application/problem+json).
- OpenAPI 3.1 di api/openapi.yaml; tipe TS di-generate (openapi-typescript atau setara — verifikasi);
  test kontrak memastikan handler Go cocok dengan spec.

# PRASYARAT BACKEND (U0, sudah diterapkan)
1. PaySLO JSON API minimum yang dipakai UI (via `lab serve`, bukan langsung
   dari browser):
   - `GET /healthz` -> `{"ok": true}` (sumber: payslo-server `healthz`).
   - `GET /incidents[?slo=&service=]` -> `{"incidents": [{id, title, slo,
     service, status ("open"|"resolved"), severity, created_at, resolved_at}]}`.
   Detail insiden (timeline), approvals, dan eksekusi runbook BELUM ada di API;
   ditambahkan di fase UI berikutnya — server Go harus degradasi anggun
   (panel terkait EmptyState, bukan error) sampai endpoint itu ada.
2. Admin gateway di port loopback terpisah `127.0.0.1:3001` (env
   `GATEWAY_ADMIN_PORT`; publik tetap `:3000`). Envoy memblokir prefix `/admin/`
   dan path `/admin` dengan `403 direct_response` — admin TIDAK PERNAH lewat
   `:8080`. Alasan: `:3001` lama dipakai Grafana, jadi Grafana pindah ke
   `127.0.0.1:3002` (lihat DECISIONS U0). `lab` CLI memakai
   `LAB_GATEWAY_ADMIN_URL` (default `http://127.0.0.1:3001`) untuk /admin/*.
3. Grafana `GF_SECURITY_ALLOW_EMBEDDING=true` (butuh iframe kiosk di /observe)
   dengan CSP `frame-src http://127.0.0.1:3002`. Tetap loopback-only, tanpa auth
   tambahan selain anonim Viewer yang sudah ada (lab-only).

# KEAMANAN lab serve (WAJIB, ini sering terlewat)
Browser di laptop trainee bisa dipaksa situs lain memanggil 127.0.0.1 (CSRF/DNS rebinding), sehingga
endpoint chaos bisa dipicu dari situs jahat. Maka:
1. Bind hanya 127.0.0.1. Tolak request dengan header Host di luar allowlist
   (127.0.0.1:<port>, localhost:<port>).
2. Request mutasi (POST/PUT/DELETE) wajib: header Origin sama dengan origin UI DAN header
   X-Lab-Token berisi token acak yang dibuat saat `lab serve` start (disuntikkan ke halaman, disimpan
   di memori JS, bukan localStorage). Tanpa keduanya -> 403.
3. Tanpa CORS (tidak ada Access-Control-Allow-*).
4. CSP ketat: default-src 'self'; script-src 'self'; style-src 'self' (tanpa inline kecuali nonce);
   img-src 'self' data:; connect-src 'self'; frame-src http://127.0.0.1:3000 (Grafana); object-src 'none';
   base-uri 'none'. Tambahkan X-Content-Type-Options, Referrer-Policy: no-referrer.
5. Semua data dari sumber luar (payload alert, label, log, nama transaksi) = tidak tepercaya: render
   sebagai teks (React escape default), DILARANG dangerouslySetInnerHTML kecuali markdown kurikulum
   yang disanitasi (DOMPurify atau setara) dan diuji.
6. Mode mentor memakai PIN dari env MENTOR_PIN. Dokumentasikan jujur: ini PEMBATAS SPOILER, bukan
   keamanan. Tidak ada klaim autentikasi.
7. Perlindungan spoiler ditegakkan di SERVER: endpoint solusi, status chaos, dan detail pemicu
   mengembalikan 403 untuk mode trainee saat skenario aktif. Catat di mentor-guide bahwa trainee tetap
   bisa curl ke admin bank-sim langsung (honor system).

# DESIGN SYSTEM
- Token (CSS variables) untuk: warna semantik (surface, surface-raised, border, text, text-muted,
  accent, ok, warn, crit, info, neutral), spacing (grid 4px), radius, shadow, z-index, durasi motion,
  tipografi. Tema terang dan gelap (ikuti prefers-color-scheme + toggle manual, simpan di localStorage).
- Semua pasangan teks/latar HARUS lulus kontras 4.5:1 (teks) dan 3:1 (komponen UI). Buat test otomatis
  yang menghitung kontras dari token untuk kedua tema.
- Status ditampilkan sebagai ikon + teks + warna (OK ✓, Warn !, Crit ✕, Unknown ?, Stale ⏱).
  Palet harus aman untuk buta warna merah-hijau (verifikasi dengan simulasi).
- Font: self-hosted (mis. paket @fontsource — verifikasi lisensi OFL), tabular-nums untuk angka.
- Densitas: dashboard operasional (informasi padat tapi lega). Layar utama 1280px+; tablet 768px
  harus dapat dipakai; di bawah itu tidak rusak (tanpa overflow horizontal) walau bukan target utama.
- Motion minimal; hormati prefers-reduced-motion. Tidak ada animasi yang menyampaikan informasi
  esensial sendirian.
- Komponen inti (dengan story/test): AppShell, StatusBadge, MetricTile (nilai, delta, sparkline,
  status data), TimeSeriesChart (+ tabel data alternatif), ErrorBudgetBar, BurnRateGauge, ServiceNode,
  Timeline, ConfirmDialog (ketik nama target untuk aksi berisiko), DiffViewer, YamlEditor,
  ApprovalCard, HintLadder, ScoreCard, EmptyState, ErrorState, StaleBanner, CopyCommand.

# SEMANTIK DATA (kritis)
- Rasio sukses = good/total. Jika total = 0 atau NaN: tampilkan "Tidak ada data", BUKAN 0% atau 100%.
- Tampilkan ukuran sampel (laju transaksi). Bila laju < ambang (default 0.2/s, dapat dikonfigurasi),
  tandai "Data sedikit, rasio tidak stabil" dan jangan beri warna kritis.
- Data usang: bila titik terakhir lebih tua dari 3x interval scrape (15 dtk di lab), tampilkan
  StaleBanner dengan waktu data terakhir.
- Sumber down (Prometheus/PaySLO/DB tidak terjangkau): panel terkait menampilkan ErrorState dengan
  penyebab dan langkah cek; halaman lain tetap berfungsi (kegagalan terisolasi per panel).
- Waktu: tampilkan zona waktu lokal, tooltip ISO-8601 UTC, relatif ("3 mnt lalu") yang memperbarui diri.
- Query Prometheus bernama (di server, parameter divalidasi). Nama metric di bawah ini HARUS
  diverifikasi terhadap output nyata /stats/prometheus Envoy dan sql_exporter, lalu dicatat:
  * http_rps, http_2xx_ratio, http_5xx_ratio, http_p99 dari metric Envoy
    (kandidat: envoy_http_downstream_rq_xx{envoy_response_code_class=...},
    envoy_http_downstream_rq_time_bucket — VERIFIKASI nama dan label aktual)
  * payment_success_ratio_by_provider / _by_channel dari payment_transactions_total{status=...}
    (SUCCESS / (SUCCESS+FAILED+EXPIRED)) dengan rate window 1m
  * payment_pending_stuck dari payment_transactions_pending
  * webhook_delivery_ratio dari payment_webhook_deliveries_total
  Edge pada Peta Arsitektur HANYA menampilkan metric yang benar-benar ada (jangan mengarang laju per
  edge yang tidak diukur).

# IA / HALAMAN (navigasi kiri; top bar: status stack, skenario aktif + timer, badge mode, tema, bahasa)
1. Mission Control (/)  2. Peta Arsitektur (/map)  3. Pembayaran (/payments)  4. SLO Center (/slo)
5. Insiden (/incidents)  6. Skenario (/scenarios)  7. Chaos & Kontrol (/control)
8. Observability (/observe)  9. Belajar (/learn)  10. Mentor (/mentor, hanya mode mentor)
11. Pengaturan & Diagnostik (/settings)

## Detail per halaman
### 1. Mission Control
- Baris status stack (health tiap service, klik -> detail). Banner "Chaos aktif" hanya terlihat mode
  mentor atau di luar skenario.
- Panel utama berdampingan: "HTTP success (Envoy)" vs "Payment success (DB)" dengan penjelasan singkat
  mengapa bisa berbeda (U2). Ubin: RPS, p99, 5xx ratio, pending stuck, webhook delivery.
- Success rate per provider (3 ubin + grafik 15 menit). Error budget SLO teratas dan burn rate.
- Incident aktif (maks 5) dan antrean approval (badge penghitung). Skenario aktif + timer.
- Aksi cepat: kirim 1 pembayaran uji, ubah laju traffic, buka Grafana.
### 2. Peta Arsitektur
- Diagram SVG tetap: loadgen -> Envoy -> gateway -> bank-bca/bank-mandiri/ewallet -> callback ->
  gateway -> merchant-sink; Postgres; observability; PaySLO. Node = health (ikon+teks);
  klik node -> panel samping (metric relevan, log terakhir, link Grafana). Edge bergaya berbeda untuk
  sync vs async (callback). Tersedia tampilan daftar (alternatif aksesibel) yang setara isinya.
- Mode mentor: panel samping node provider memuat kontrol chaos. Mode trainee: tidak ada.
### 3. Pembayaran
- Form kirim pembayaran uji (channel, amount integer IDR, merchant_id, order_id otomatis unik).
- Tabel transaksi (filter status/provider/channel, urut waktu, paginasi kursor). Detail transaksi:
  timeline (CREATED -> PENDING -> callback diterima -> SUCCESS/FAILED/EXPIRED -> webhook delivered),
  provider_ref, tombol "Cek status ke bank" (GET bank-sim /status/{ref}), link log (Loki) dan trace
  (Tempo) via trace_id bila obs-full aktif. Tandai transaksi dengan callback >1 (indikasi duplikat)
  secara netral ("callback diterima 2x"), tanpa menyebut penyebab.
### 4. SLO Center
- Daftar SLO: objective, window, SLI sekarang, sisa error budget, burn rate, status deployment per
  backend (prometheus-local, datadog-mock: pending/applied/drifted/failed/unsupported).
- Detail SLO: grafik SLI, burn rate multi-window, alert yang menyala, link runbook.
- Editor YAML (CodeMirror) + panel hasil: Validate (error dengan baris/kolom), Plan (DiffViewer),
  Apply (ConfirmDialog; hasil per backend; apply kedua = no-op ditampilkan jelas), Drift (daftar
  perubahan manual; aksi revert/accept hanya bila API mendukung).
- Capability matrix backend + peringatan fitur tak didukung (tampilkan, jangan disembunyikan).
- Kalkulator error budget interaktif (objective, window -> menit downtime diizinkan) untuk belajar.
### 5. Insiden
- Daftar (status, severity, layanan, durasi), detail: timeline, alert terkorelasi (sumber, label),
  eksekusi runbook per langkah (diagnostic otomatis menampilkan output; remediation menampilkan
  ApprovalCard: perintah yang akan jalan, dampak, tombol Approve/Reject + alasan wajib saat Reject).
- Catatan manual ke timeline. Postmortem: editor template (ringkasan, dampak, kronologi dari timeline,
  akar masalah, yang berjalan baik/buruk, action item) dengan ekspor Markdown. Status draft/published.
### 6. Skenario
- Katalog (kesulitan, estimasi waktu, prasyarat profil compose, status selesai/skor terbaik).
- Halaman skenario: briefing (Markdown tanpa spoiler), timer, tombol Mulai/Berhenti, HintLadder
  (petunjuk terbuka bertahap, konfirmasi "ini mengurangi skor sedikit"), form Jawaban diagnosis,
  tombol Verifikasi -> ScoreCard (rincian rubrik: TTD 15, TTM 25, diagnosis 25, keamanan aksi 15,
  komunikasi 10, postmortem 10; dua terakhir "menunggu mentor"). Solusi terkunci sampai selesai
  atau mode mentor.
### 7. Chaos & Kontrol
- Traffic (laju, profil), Routing (enable/disable provider, bobot), Release (good/bad), tindakan yang
  juga tersedia sebagai aksi trainee yang sah (mitigasi). Setiap perubahan = ConfirmDialog + audit.
- Chaos per provider/DB/merchant: HANYA mode mentor atau saat tidak ada skenario aktif.
  Tombol "Reset semua chaos" selalu terlihat di mode mentor.
### 8. Observability
- Tab: Grafana dashboards tertanam (iframe, kiosk), Prometheus (alert aktif & rules), Alertmanager
  (alert), ntfy (notifikasi terkirim). Fallback bila iframe diblokir: tombol "Buka di tab baru" dan
  pesan penyebab. Anotasi deploy/chaos tampil di grafik.
### 9. Belajar
- Render kurikulum hari 1-5 dari /curriculum (Markdown tersanitasi), checklist per modul, progres
  disimpan di server lab (SQLite), glosarium (SLI, SLO, error budget, burn rate, TTD, TTM, dll.).
### 10. Mentor
- Timeline aksi trainee (audit), inject (mis. traffic x3), snapshot save/load (dengan konfirmasi),
  penilaian manual (komunikasi, postmortem) dengan catatan, ringkasan sesi, ekspor laporan (Markdown/JSON).
### 11. Pengaturan & Diagnostik
- Nama trainee, bahasa, tema, ambang data-sedikit. Diagnostik: versi tiap service, status koneksi
  (Prometheus, PaySLO, Postgres, bank-sim x3, merchant-sink, Grafana), penggunaan resource
  (docker stats --no-stream bila Docker tersedia), tautan troubleshooting.
  Tombol "Salin laporan diagnostik" (tanpa secret).

# STATE KOSONG, ERROR, DAN EDGE CASE (setiap panel WAJIB menangani)
loading (skeleton, bukan spinner penuh halaman), empty (EmptyState + langkah berikutnya),
error (penyebab + aksi coba lagi), stale, partial (sebagian sumber down), lab belum `up`
(onboarding: tampilkan perintah `make up-lite` dengan tombol salin), profil compose belum aktif
(mis. obs-full mati -> fitur log/trace dinonaktifkan dengan penjelasan, bukan error).

# PERFORMA
Budget awal: JS initial <= 350 KB gzip (ukur, catat; jika terlampaui, catat alasan di DECISIONS.md),
route-level code splitting, update realtime di-throttle (maks 1 render/detik per panel), grafik
menyimpan maks 15 menit x 5 dtk titik di klien, tab tersembunyi menghentikan polling.

# STANDAR KUALITAS
tsc --noEmit strict (tanpa `any` kecuali dikomentari), ESLint (termasuk jsx-a11y dan aturan
larangan string hardcoded), Prettier, Vitest coverage logika/komponen inti >= 80%, Playwright e2e,
axe-core nol pelanggaran serius/kritis pada setiap halaman (kedua tema), Playwright visual snapshot
untuk halaman utama (mode --demo, deterministik: waktu dan acak dibekukan), npm audit (atau setara)
tanpa high/critical, lisensi dependensi diperiksa. Go: gofmt, go vet, golangci-lint, test kontrak
terhadap openapi.yaml.

# AKSESIBILITAS (U8) — hasil audit
Diuji otomatis di `ui/e2e/a11y.spec.ts` (Chromium, `lab serve --demo`) dan diverifikasi manual.

- **axe-core**: 0 pelanggaran `serious`/`critical` pada 9 halaman utama × 2 tema (light/dark) ×
  2 bahasa (id/en) = 36 kombinasi. Tepat satu `<h1>` per halaman; satu `<main>`; satu `<nav>`.
  (Temuan yang diperbaiki: HTML kurikulum ber-`h1` diturunkan ke `h2` saat disematkan —
  `SafeHtml`/`demoteHeadings` di `src/pages/Learn.tsx`.)
- **Keyboard-only**: Tab menjangkau nav → topbar (select tema/bahasa) → konten; semua kontrol utama
  (checkbox routing, dialog konfirmasi Radix) dapat dioperasikan Tab/Enter/Space; indikator fokus
  terlihat (`outline`/`box-shadow`); tidak ada elemen tak terlihat yang menerima fokus.
- **aria-live**: region `aria-live="polite"` mengumumkan insiden/approval baru (diuji: "Approval
  baru: disable provider bca" muncul di dalam region live). Status memakai ikon **+** teks
  (`aria-label`), bukan warna saja (U8).
- **prefers-reduced-motion**: transisi/animasi `body` ≤ 10 ms saat `reduce` diaktifkan.
- **Zoom 200%** (viewport efektif 640 px): 0 overflow horizontal di semua halaman; tabel memakai
  `TableScroll` (region fokusabel), grid memakai `minmax(0,1fr)`, kontrol form dibatasi
  `max-width: 100%` (input `size` bawaan bisa melebihi sel grid pada font Linux).
- **Isolasi server**: spec a11y memutasi state demo (menonaktifkan provider `bca`, membuat
  approval). Karena itu project `a11y` memakai **server demo segar sendiri** (`:8090`),
  terpisah dari spec fungsional `:8088`, agar tidak merusak asumsi spec U2/U4.
- **Bahasa**: `document.documentElement.lang` mengikuti bahasa aktif (WCAG 3.1.1).
- **Pembaca layar (manual, TalkBack/VoiceOver/NVDA)**: verifikasi struktur peran+label dilakukan
  lewat asersi peran/nama + ARIA snapshot Playwright (mis. nav bernama "PaySLO Lab", link bernama,
  `role=status` punya teks). Catatan: pengujian dengan pembaca layar sungguhan (bukan simulasi)
  **belum** dilakukan di lingkungan pengembangan ini — dicatat di `docs/BACKLOG.md`.

# VISUAL REGRESSION (U8)
`ui/e2e/visual.spec.ts`: 9 halaman × 2 tema = 18 snapshot, mode `--demo`, waktu (`Date.now` →
`demoBase`) dan acak (`Math.random` → mulberry32) dibekukan (`freeze()` di `e2e/lib.ts`), animasi
dimatikan, `maxDiffPixelRatio: 0.02`. Snapshot per-platform (`*-visual-<platform>.png`); baseline
`darwin` (dev) dan `linux` (CI) di-commit. Uji memakai **server demo segar** di `:8089` (via
`webServer`) karena state demo dapat berubah akibat mutasi dari spec lain (pembayaran/routing).

# PERFORMA (U8) — hasil ukur
Diukur dengan `ui/scripts/check-bundle.mjs` dan `ui/e2e/perf.spec.ts` (Chromium lokal,
`lab serve --demo`).

- **Bundle initial (gzip)**: JS **225,0 KB** + CSS **3,8 KB** — di bawah anggaran 350 KB.
  Chunk editor CodeMirror **136,3 KB gzip** termuat **malas** (hanya di `/slo`, diverifikasi
  tidak dimuat di `/`). `check-bundle.mjs` gagal bila initial melebihi anggaran atau chunk editor
  bocor ke initial.
- **Metrik setara Lighthouse** (dari Performance API, bukan Lighthouse penuh):
  FCP ≈ **56 ms**, DOMContentLoaded ≈ **36 ms**, resource count = **11** (Chromium lokal,
  contoh pengukuran; ambang: FCP < 2500 ms, DCL < 3500 ms, resources < 80). Uji menunggu
  entri `first-contentful-paint` muncul sebelum mengukur (paint timing bisa diposting
  setelah DCL) agar tidak balapan.
- **Soak SSE 1 jam** (`PERF_SOAK_SECONDS=3600`, Chromium, heap via CDP; 118 sampel @30 dtk
  setelah warmup 60 dtk): start = **5,7 MB**, end = **6,4 MB**, pertumbuhan = **0,7 MB**
  (ambang < 20 MB; sampel berosilasi 5–9 MB tanpa tren naik). Membuktikan stream
  `GET /api/v1/events` terbuka lama tidak membocorkan memori.

# KEAMANAN (U8) — verifikasi ulang
- **Header**: setiap respons dokumen memuat CSP ketat (`default-src 'self'`;
  `script-src 'self' 'nonce-…'`; `object-src 'none'`; `base-uri 'none'`; `frame-src`/`connect-src`
  hanya `http://127.0.0.1:3002`), `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`;
  **tanpa** header CORS. Dites di `e2e/security.spec.ts` + Go `TestSecurityHeadersPresent`.
- **CSP tanpa pelanggaran**: 0 `securitypolicyviolation` (console bersih) di semua halaman.
  Perbaikan: Zod v4 dimatikan mode JIT (`z.config({ jitless: true })`) agar tidak memprobe
  `new Function` (memicu pelanggaran `script-src eval`) — lihat `src/zod-setup.ts`.
- **Hanya loopback**: seluruh alur (termasuk mutasi create payment) hanya mengirim request ke
  `127.0.0.1` (`e2e/security.spec.ts` memantau setiap request).
- **DNS rebinding**: Host asing (`evil.example`, `attacker.127.0.0.1.nip.io`, `localhost.`,
  port salah, Host kosong) ditolak `403` meski koneksi datang dari loopback —
  `cmd/lab/security_u8_test.go`. Host loopback valid case-insensitive diterima.
- **Mutasi**: tanpa `Origin` lab / `X-Lab-Token` → `403` (termasuk lewat Host penyerang);
  dengan keduanya → bukan `403`. Dites di Go + `e2e/security.spec.ts`.
- **Audit dependensi & lisensi**: `npm audit` (0 high/critical), pemeriksaan lisensi npm
  (`ui/scripts/check-licenses.mjs`, hanya lisensi permisif — MIT/Apache-2.0/ISC/BSD/MPL-2.0/OFL-1.1
  dst.), `go vet` + gofmt; dibungkus `make ui-audit` / `scripts/ui-audit.sh`.

# KOMPATIBILITAS (U8)
`ui/e2e/compat.spec.ts` dijalankan di **Chromium, Firefox, WebKit** (project Playwright): axe bersih
+ tanpa overflow horizontal pada lebar **1280 / 1024 / 768** px, plus alur inti (buat pembayaran)
di tiap browser.

Batas yang diketahui:
- Breakpoint penuh dirancang untuk **desktop** (≥ 768 px). Di bawah 768 px tata letak tetap terbaca
  (tabel `TableScroll`, `/slo` & `/incidents` menumpuk satu kolom via `.split-layout` pada
  ≤ 1000 px) tetapi **bukan** target mobile-first (UI-SPEC: konsol untuk laptop trainee).
- Snapshot visual & metrik waktu hanya diukur di Chromium (baseline per-platform).
- Embed Grafana bergantung pada `GF_SECURITY_ALLOW_EMBEDDING=true`; bila diblokir, fallback
  "Buka di tab baru" tampil (lihat `docs/troubleshooting.md`).
- WSL2: akses via `http://127.0.0.1:8088` (bukan IP WSL) — lihat `docs/troubleshooting.md`.

# DEVIASI & CATATAN U8
- Rute `/scenarios` masih **placeholder**: acceptance "menyelesaikan skenario 01 sepenuhnya via UI"
  belum terpenuhi; skenario tetap dijalankan lewat CLI `lab` / halaman Insiden (alert → runbook →
  approval). Dicatat di `docs/BACKLOG.md`.
