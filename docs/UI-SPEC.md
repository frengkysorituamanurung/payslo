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