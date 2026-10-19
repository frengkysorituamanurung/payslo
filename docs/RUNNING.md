# Cara Menjalankan PaySLO Lab di Lokal

Panduan langkah demi langkah dari clone bersih sampai bisa membuat transaksi,
melihat insiden, dan menjalankan skenario kurikulum. Ditulis untuk laptop
4 CPU / 8 GB RAM (Linux, macOS, Windows-WSL2; amd64 & arm64).

> ⚠️ **BUKAN UNTUK PRODUKSI.** Semua port di-bind `127.0.0.1`, semua kredensial
> bertanda `lab-only`, semua data sintetis. Jangan jalankan di server publik,
> jangan isi data asli, jangan expose port.

- Ringkasan arsitektur & daftar URL: [`../README.md`](../README.md).
- Bila macet: [`troubleshooting.md`](troubleshooting.md).
- Pin versi & cara upgrade: [`VERSIONS.md`](VERSIONS.md).

## 1. Prasyarat

| Perangkat | Versi | Dipakai untuk |
| --- | --- | --- |
| Docker + Docker Compose v2 | Compose v2 (`docker compose version`) | seluruh stack lab |
| `make`, `bash`, `curl`, `python3` | bebas | Makefile, `scripts/*.sh`, smoke/e2e |
| Rust (rustup) | dipin `rust-toolchain.toml` → **1.97.1** | CLI `lab`, PaySLO, bank-sim, dll. |
| Node.js | **≥ 22** (`ui/package.json` `engines`) | Lab Console (UI) |
| Go | **≥ 1.25** (`cmd/lab/go.mod`) | server `lab serve` (UI tersemat) |

Catatan:

- `rustup` akan otomatis memasang toolchain 1.97.1 + `rustfmt`/`clippy` saat
  pertama dipakai (jangan install Rust manual tanpa rustup).
- Node hanya perlu bila Anda memakai/membangun Lab Console (`make ui-*`, `make serve`).
  Stack pembayaran (jalur CLI murni) tidak butuh Node/Go.
- Python3 & curl wajib untuk skrip verifikasi (`make e2e`, `make smoke`).
- Beri Docker (Desktop/Colima) **≥ 6 GB** memori agar build + stack bisa jalan
  bersamaan. `CARGO_BUILD_JOBS=2` sudah ditetapkan di `Dockerfile` untuk 8 GB.

Versi yang diverifikasi penulis saat menulis dokumen ini (macOS arm64):
Docker 29.6.2, Compose v5.4.0, rustc/cargo 1.97.1, Node v26.4.0, Go 1.26.5,
Python 3.9.6.

## 2. Dapatkan kode

```sh
git clone <url-repo> payslo-lab
cd payslo-lab
```

Konfigurasi opsional (semua nilai punya default di `docker-compose.yml`, jadi
langkah ini boleh dilewati):

```sh
cp .env.example .env          # opsional; .env diabaikan git
```

> Repo ini belum punya remote/git publik pada saat penulisan; sesuaikan `<url-repo>`
> atau salin folder lokal. Lihat [`VERSIONS.md`](VERSIONS.md).

## 3. Jalankan stack — pilih salah satu

Ada dua jalur. Keduanya menyalakan set layanan yang sama
(default + `traffic` + `obs` + `payslo`), bedanya: build lokal vs tarik image.

### 3a. Jalur A — image prebuilt (tercepat, tanpa kompilasi)

```sh
PAYSLO_IMAGE_TAG=v0.1.0 make up-prebuilt
```

Image Rust/Go diambil dari GHCR (dipublikasikan oleh workflow `release.yml`
setiap tag `v*`). Fork wajib menimpa registry:
`PAYSLO_REGISTRY=ghcr.io/<org> PAYSLO_IMAGE_TAG=v0.1.0 make up-prebuilt`.

### 3b. Jalur B — build lokal (tanpa registry, butuh Rust compile)

```sh
make up-lite
```

Build bersih ± 7 menit (cold cache) di 8 GB arm64; incremental ± 50 detik.
Idempotent: menjalankan `make up-lite` dua kali tidak error.

### Setelah salah satu jalur di atas

```sh
make status      # daftar container + health
make open        # cetak semua URL lab
```

Tunggu semua service `healthy` (profil lite ± 1–2 menit setelah build). Untuk
menambahkan log/trace (Loki, Tempo, Alloy):

```sh
make up-obs-full    # aktifkan profil obs-full di atas stack yang berjalan
```

## 4. Verifikasi cepat (opsional, ± 30 detik)

Dari kondisi stack hidup:

```sh
make e2e    # 1 pembayaran -> callback bank -> 1 webhook merchant via Envoy
```

Keluaran sukses berakhir dengan `e2e PASS: ... status=SUCCESS|FAILED webhooks=1 envoy=ok`.
Ingat prinsip lab: HTTP 200 ≠ pembayaran sukses (status final datang via callback).

Pipeline penuh (apply SLO → skenario 01 → insiden → notifikasi ntfy → resolved → down)
memakai `make smoke` (lihat §7).

## 5. Lab Console (UI web di `127.0.0.1:8088`)

Konsol UI adalah antarmuka trainee (Go `lab serve` menyematkan `ui/dist`).
Bisa dicoba **tanpa Docker** (mode `--demo`, data deterministik) atau terhubung
ke stack live.

### 5a. Tanpa Docker (paling cepat untuk melihat UI)

```sh
make ui-install     # npm ci (sekali)
make ui-build       # tsc + vite -> ui/dist, disalin ke embed Go
make serve-demo     # buka http://127.0.0.1:8088
```

### 5b. Terhubung stack live

```sh
make serve          # API live + UI embed di :8088
# bila memakai fitur SLO live (apply ke datadog-mock):
#   DD_API_KEY=lab-only-dd-api-key make serve
```

`make serve` menjalankan `go build -o ../../bin/labserve .` lalu
`./bin/labserve serve`. Jalankan `make ui-build` lebih dulu, kalau tidak UI yang
tersemat masih placeholder.

### 5c. Mode pengembangan (HMR Vite)

```sh
# terminal 1: server API + proxy ke Vite
cd cmd/lab && go run . serve --dev-proxy http://127.0.0.1:5173
# terminal 2: Vite dev server
make ui-dev         # http://127.0.0.1:5173
```

Opsi flag `lab serve`: `--addr` (wajib loopback), `--demo`, `--dev-proxy`,
`--web-dir`. Mode mentor diaktifkan dengan env `MENTOR_PIN` (mis.
`MENTOR_PIN=1234 make serve`); kosong = nonaktif (halaman `/mentor` tetap ada
tapi terkunci — pembatas spoiler, bukan keamanan).

Setelah terbuka: buat pembayaran di **Pembayaran**, pantau **Insiden**
(alert → runbook → approval). Rute `/scenarios` masih placeholder; skenario
kurikulum dijalankan lewat CLI `lab` (§6) — lihat [`BACKLOG.md`](BACKLOG.md).

Detail arsitektur UI, env var, dan contoh `curl`:
[`../ui/README.md`](../ui/README.md).

### 5d. Binary konsol tanpa toolchain

Release git tag menyertakan binary `labserve` (UI tersemat) untuk
linux/macOS/Windows × amd64/arm64. Unduh dari GitHub Release, ekstrak, lalu:

```sh
./labserve serve --demo        # atau: ./labserve serve   (stack live)
```

## 6. Menjalankan skenario (jalur CLI `lab`)

```sh
cargo build -p lab                       # atau langsung pakai cargo run -p lab --
./target/debug/lab scenario list
./target/debug/lab scenario start 01
./target/debug/lab hint 01
# ... deteksi insiden, lalu mitigasi, mis.:
./target/debug/lab routing disable bca
./target/debug/lab answer diagnosis "provider bca down pada callback"
./target/debug/lab verify 01
./target/debug/lab scenario stop 01
```

Perintah lain yang berguna: `up`, `down`, `reset`, `status`, `open`,
`chaos <target> ...`, `traffic set --rate N`, `routing enable|disable <provider>`,
`deploy gateway --version good|bad`, `mentor ...`, `snapshot save|load`,
`completion bash|zsh|fish`. Semua memanggil `docker compose` sebagai subprocess
(butuh Docker).

## 7. Smoke penuh (end-to-end)

```sh
make smoke
```

Menjalankan: `down -v` → `up` → tunggu healthy → cek konsol `lab serve`
(health + guard mutasi) → traffic e2e → apply SLO (Prometheus + datadog-mock) →
solver skenario 01 → verifikasi insiden + notifikasi ntfy + resolved → `down`.

Durasi tipikal ~12–75 menit (bergantung drain burn rate 1h/10m). Env:

- `SMOKE_MODE=lite|prebuilt` (default `lite`)
- `SMOKE_KEEP_UP=1` (lewati `down` di akhir, untuk debug)
- `PAYSLO_IMAGE_TAG` / `PAYSLO_REGISTRY` (untuk `prebuilt`)

## 8. PaySLO CLI (SLO-as-code)

```sh
cargo run -p payslo-cli -- validate payslo-workspace/slo --workspace payslo-workspace
cargo run -p payslo-cli -- render   payslo-workspace/slo --workspace payslo-workspace --out payslo-workspace/generated
cargo run -p payslo-cli -- plan     payslo-workspace/slo --workspace payslo-workspace
export DD_API_KEY=lab-only-dd-api-key                     # untuk apply ke datadog-mock
cargo run -p payslo-cli -- apply    payslo-workspace/slo --workspace payslo-workspace
cargo run -p payslo-cli -- drift    --workspace payslo-workspace
cargo run -p payslo-cli -- capabilities
```

`payslo serve` (port 8081) menerima webhook Alertmanager/Datadog dan menjalankan
runbook/incident; dashboard minimal di <http://127.0.0.1:8081/incidents>.

## 9. Gate kualitas (kontributor)

```sh
# Rust
make fmt        # cargo fmt --all
make lint       # fmt --check + clippy -D warnings
make test       # cargo test --workspace
make deny       # cargo deny check

# UI (butuh Node ≥ 22)
make ui-install
make ui-test    # tsc + eslint + vitest + prettier
make ui-e2e     # Playwright (a11y/visual/keamanan/kompatibilitas) vs lab serve --demo
make ui-audit   # npm audit + lisensi + anggaran bundle + gofmt/go vet
make ui-perf    # anggaran bundle + FCP/DCL (PERF_SOAK_SECONDS untuk soak SSE)

# Go (cmd/lab)
cd cmd/lab && gofmt -l . && go vet ./... && go test ./...
```

`make ui-e2e` memasang browser Playwright bila belum ada dan menjalankan tiga
server demo (`:8088` fungsional, `:8089` visual, `:8090` a11y).

## 10. Menghentikan & reset

```sh
make down       # hentikan semua profil (container berhenti, volume tetap)
make reset      # down -v + up-lite: hapus volume & seed ulang data sintetis
```

`make reset` berguna bila state lab (DB, insiden PaySLO) ingin dikembalikan ke
kondisi bersih. Semua perintah idempotent.

## 11. Port (semua `127.0.0.1`)

Jalankan `make open` untuk daftar terkini. Ringkas:

| Layanan | Port |
| --- | --- |
| Envoy (trafik) | 8080 |
| gateway-api | 3000 (healthz/readyz), 3001 (admin) |
| bank-sim (bca/mandiri/ewallet) | 4001 / 4002 / 4003 |
| merchant-sink | 5000 |
| Prometheus | 9090 |
| Alertmanager | 9093 |
| Grafana | 3002 (`admin` / `lab-only-grafana`) |
| sql_exporter | 9399 |
| Loki | 3100 |
| Tempo | 3200 |
| PaySLO | 8081 |
| datadog-mock | 8126 |
| ntfy | 8022 |
| Lab Console (`lab serve`) | 8088 |

## 12. Troubleshooting singkat

- **UI kosong / "Placeholder"** → `ui/dist` belum di-build: `make ui-build`, lalu
  ulangi `make serve`/`make serve-demo`.
- **Mutasi di UI `403`** → header `X-Lab-Token`/`Origin` lab tidak sesuai;
  Host asing ditolak (proteksi DNS rebinding).
- **`make ui-e2e` gagal "Executable doesn't exist"** → `cd ui && npx playwright install --with-deps chromium firefox webkit`.
- **Build Docker OOM di 8 GB** → jangan naikkan `CARGO_BUILD_JOBS`; beri Docker ≥ 6 GB.
- **`make smoke` timeout menunggu incident resolved** → wajar bila burn 1h/10m
  belum reda; lihat FAQ di [`../README.md`](../README.md).

Detail lengkap: [`troubleshooting.md`](troubleshooting.md).
