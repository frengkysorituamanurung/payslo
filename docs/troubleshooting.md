# Troubleshooting — PaySLO Lab

## Port bentrok (sudah dipakai)

Semua port di-bind ke `127.0.0.1`. Bila `up` gagal dengan `address already
in use`, cari pemakainya (`lsof -iTCP:<port> -sTCP:LISTEN`) dan hentikan,
atau ubah mapping di `docker-compose.yml`. Port lab: 3000 (gateway), 3001 (admin gateway), 3002 (grafana), 4001–4003,
5000, 5432, 8080, 8081, 8088 (lab console), 8126, 9090, 9093, 9399, 9901, 3100, 3200.

## RAM kurang (build OOM / container mati exit 137)

- `CARGO_BUILD_JOBS=2` sudah diset di Dockerfile; jangan naikkan di 8 GB.
- Batasi Docker Desktop/colima ≥ 6 GB. `make reset` bila build berhenti
  di tengah (lapisan cargo-chef di-cache, lanjutan cepat).

## Apple Silicon / arm64

- Semua image multi-arch (amd64+arm64) dan terverifikasi via manifest.
- Bila build memilih arsitektur salah (TARGETARCH kosong), Dockerfile memakai
  fallback `uname -m`. Pastikan `docker buildx` terinstal.

## Windows WSL2

- Jalankan dari dalam WSL2 (bukan PowerShell): Docker Desktop dengan integrasi
  WSL2 aktif. Clone repo di filesystem Linux (`~/`), bukan `/mnt/c` (lambat
  + masalah permission).
- `make` tersedia? Bila tidak, jalankan target Makefile manual (lihat isinya).

## docker.sock permission (Alloy)

- Alloy me-mount `/var/run/docker.sock:ro`. Bila Alloy error permission,
  pastikan user Docker group atau jalankan ulang runtime container.
- PERINGATAN (P4): mount ini read-only dan hanya untuk Alloy.

## Prometheus tidak reload rules

- `payslo-workspace/generated` di-mount ke `/etc/prometheus/payslo:ro`.
  Setelah render manual: `curl -X POST 127.0.0.1:9090/-/reload`
  (`--web.enable-lifecycle` aktif). Cek `/-/ready`.
- `payslo apply --connection prometheus-local` me-reload otomatis.

## payslo.db "malformed" (macOS + colima)

- JANGAN buka `payslo-workspace/data/payslo.db` dari host (SQLite di atas
  bind-mount sshfs inkohéren/korup). Server adalah satu-satunya accessor
  (named volume); CLI membaca via HTTP `/incidents`.
- Bila sudah korup: `docker compose --profile payslo up -d --force-recreate
  payslo` (volume bernama dibuat ulang kosong).

## Loadgen berhenti (tidak ada trafik baru)

- `k6 run` berdurasi 30 menit lalu exit (normal). `scenario start` me-restart
  otomatis; atau `docker compose --profile traffic up -d loadgen`.

## Skenario macet di tengah

- `lab scenario stop <id>` menjalankan teardown (reset bank/routing/TTL).
- Darurat: `lab chaos reset` (bank normal, merchant 200, lock DB lepas) atau
  `lab snapshot load <nama>` bila sempat save.

## Alert tak kunjung menyala

- Burn-rate butuh jendela penuh (5m) + `for` terpenuhi; TTL transaksi default
  600 dtk (skenario memakai 60 dtk agar cepat). Tunggu 4–6 menit pasca-trigger.
- Pastikan loadgen jalan (sumber sampel) dan payslo service up (insiden).

## Halaman Observability kosong / log & trace nonaktif (profil obs-full mati)
Gejala: banner "Profil obs-full tidak aktif" di /observe; tautan log/trace
di /payments disembunyikan dengan penjelasan.
Solusi: nyalakan `make up-obs-full` (loki, tempo, alloy), lalu muat ulang
/observe. Ini perilaku yang dimaksud (degradasi anggun), bukan error —
`GET /api/v1/observe/status` melaporkan `composeProfiles.obs-full: false`.

## Embed Grafana diblokir (fallback "Buka di tab baru")
Gejala: panel fallback di /observe alih-alih dasbor tertanam.
Penyebab umum: `GF_SECURITY_ALLOW_EMBEDDING=false` (compose default `true`;
cek `docker compose config | grep ALLOW_EMBEDDING`) atau header
X-Frame-Options dari reverse proxy di depan Grafana.
Solusi: pastikan compose memakai nilai default repo, restart grafana, lalu
muat ulang. Fallback "Buka di tab baru" selalu tersedia sementara itu.

## Lab Console (U8)

### `lab serve` menampilkan "Placeholder" / UI kosong
Binary `lab serve` menyematkan (embed) hasil build UI. Bila `ui/dist` belum
di-build, yang tersemat hanya `web/dist/PLACEHOLDER.html`.
Solusi: `make ui-install` (sekali), lalu `make ui-build`, lalu jalankan ulang
`make serve`. Untuk pengembangan cepat, `make ui-dev` (Vite :5173) memproksikan
API ke `lab serve` sehingga tidak perlu rebuild setiap perubahan.

### Mutasi ditolak `403` di UI
Setiap mutasi (POST/PUT/DELETE) butuh header `X-Lab-Token` (dari `GET
/api/v1/session`) **dan** `X-Lab-Actor`, serta `Origin` lab. Bila memanggil API
langsung (mis. `curl`), sertakan keduanya; halaman UI mengisinya otomatis.
Bila `403` hanya kadang-kadang, cek apakah DSN/Origin yang dipakai bukan
`http://127.0.0.1:8088` (Host/Origin asing ditolak — proteksi DNS rebinding).

### `make ui-e2e` gagal: browser Playwright belum ada
Gejala: error "Executable doesn't exist … chromium".
Solusi: `cd ui && npx playwright install --with-deps chromium firefox webkit`
(atau minimal `chromium`; visual/kompatibilitas butuh ketiganya). Di CI, browser
dipasang otomatis oleh job `ui-e2e`.
Catatan baseline visual: snapshot bersufiks platform (`*-visual-darwin.png`,
`*-visual-linux.png`). Bila menjalankan di platform/OS lain, atau UI sengaja
berubah, perbarui dengan `cd ui && npx playwright test visual.spec.ts --project=visual --update-snapshots`.

### `npm audit` / lisensi gagal (gate `make ui-audit`)
`make ui-audit` menolak dependensi high/critical dan lisensi di luar allowlist
permisif. Perbaiki: `cd ui && npm audit fix` (hati-hati: ini mengubah
`package-lock.json`), atau catat pengecualian baru di
`ui/scripts/check-licenses.mjs` **beserta alasan**.

