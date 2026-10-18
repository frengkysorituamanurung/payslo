# PaySLO Lab Console (UI)

Dashboard web **100% lokal** untuk trainee/mentor PaySLO Lab. Disajikan oleh
binary Go `lab serve` (`cmd/lab`) yang meng-`go:embed` hasil build `ui/dist` —
tanpa CDN, tanpa font eksternal, tanpa request ke luar `127.0.0.1`.

> ⚠️ Bukan untuk produksi. Semua data sintetis. Semua port di-bind `127.0.0.1`.

## Fungsi

Konsol ini adalah satu-satunya antarmuka trainee ke lab (selain CLI `lab`).
Halaman (lihat `docs/UI-SPEC.md`):

| Rute         | Halaman                 | Isi singkat                                                                                      |
| ------------ | ----------------------- | ------------------------------------------------------------------------------------------------ |
| `/`          | Mission Control         | HTTP success vs Payment success, status stack, insiden terbuka                                   |
| `/map`       | Peta Arsitektur         | Alur layanan + status per service                                                                |
| `/payments`  | Pembayaran              | buat pembayaran, tabel kursor, detail/timeline, cek status bank, tautan Loki                     |
| `/control`   | Chaos & Kontrol         | traffic/routing/release + audit (konfirmasi + token)                                             |
| `/slo`       | SLO Center              | daftar/detail SLO, editor YAML, validate/plan/apply, drift, kapabilitas, kalkulator error budget |
| `/incidents` | Insiden                 | daftar/detail, timeline, runbook + approval, catatan, postmortem, toast realtime                 |
| `/mentor`    | Mentor                  | timeline audit, inject, snapshot, penilaian, ekspor (butuh mode mentor)                          |
| `/learn`     | Belajar                 | kurikulum 1–5, checklist, progres, glosarium                                                     |
| `/observe`   | Observability           | iframe Grafana + tab Prometheus/Alertmanager/ntfy                                                |
| `/settings`  | Pengaturan & Diagnostik | nama/bahasa/tema/ambang, versi + koneksi service                                                 |
| `/:page`     | Placeholder             | rute skenario (belum diimplementasi — lihat `docs/BACKLOG.md`)                                   |

## Cara menjalankan

### Produksi lokal (embed, direkomendasikan)

```sh
make ui-install   # npm ci (pin eksak, package-lock.json)
make ui-build     # tsc + vite -> ui/dist, lalu salin ke cmd/lab/web/dist
make serve        # API live + UI embed di http://127.0.0.1:8088
# atau tanpa Docker/stack:
make serve-demo   # fixture deterministik (mode --demo) di :8088
```

Buka <http://127.0.0.1:8088>.

### Pengembangan (HMR)

```sh
make ui-install
# terminal 1: server API + proxy ke Vite
cd cmd/lab && go run . serve --dev-proxy http://127.0.0.1:5173
# terminal 2: Vite dev (:5173)
make ui-dev
```

## Perintah npm

| Perintah          | Fungsi                                        |
| ----------------- | --------------------------------------------- |
| `npm run dev`     | Vite dev server `127.0.0.1:5173`              |
| `npm run build`   | `tsc --noEmit` + `vite build` → `ui/dist`     |
| `npm test`        | Vitest (unit/komponen)                        |
| `npm run lint`    | ESLint (termasuk `jsx-a11y`)                  |
| `npm run format`  | Prettier `--check`                            |
| `npm run gen:api` | Regenerasi tipe TS dari `../api/openapi.yaml` |
| `npm run e2e`     | Playwright (lihat `e2e/`)                     |

Gate kualitas lengkap: `make ui-test` (tsc + eslint + vitest + prettier) dan
`make ui-e2e` (Playwright vs `lab serve --demo`). Audit + performa:
`make ui-audit`, `make ui-perf`.

> **Catatan e2e:** `make ui-e2e` mengharapkan **server demo segar**. Secara lokal
> Playwright akan memakai ulang server yang sudah ada di `:8088`; jadi JANGAN
> mengarahkan `PLAYWRIGHT_BASE_URL` ke `make serve` yang sudah lama berjalan
> (state demo bisa berubah antar-run: routing dinonaktifkan, approval ter-dedup).
> Untuk hasil deterministik, hentikan dulu `lab serve` di `:8088`, atau jalankan
> dengan `CI=true` (server selalu start baru). Project `visual` selalu memakai
> server segar sendiri di `:8089`.

## Hasil audit U8

Ringkasan (detail + angka di `docs/UI-SPEC.md`):

- **a11y**: axe 0 serious/critical pada 9 halaman × 2 tema × 2 bahasa; keyboard-only,
  `aria-live` approval, `prefers-reduced-motion`, zoom 200% — `e2e/a11y.spec.ts`.
- **Visual**: 18 snapshot (9 halaman × 2 tema) + baseline darwin/linux — `e2e/visual.spec.ts`.
- **Keamanan**: CSP tanpa pelanggaran, hanya loopback, mutasi tanpa token 403,
  Host asing ditolak — `e2e/security.spec.ts` + `cmd/lab/security_u8_test.go`.
- **Kompatibilitas**: Chromium/Firefox/WebKit, lebar 1280/1024/768 — `e2e/compat.spec.ts`.
- **Performa**: JS initial ≈ 225 KB gzip (anggaran 350 KB), FCP/DCL lokal, soak SSE 1 jam —
  `e2e/perf.spec.ts` + `scripts/check-bundle.mjs`.

## Variabel lingkungan

| Variabel                     | Dipakai oleh           | Arti                                                                      | Default                 |
| ---------------------------- | ---------------------- | ------------------------------------------------------------------------- | ----------------------- |
| `PLAYWRIGHT_BASE_URL`        | `playwright.config.ts` | URL server fungsional                                                     | `http://127.0.0.1:8088` |
| `PLAYWRIGHT_A11Y_BASE_URL`   | project `a11y`         | URL server segar untuk a11y (spec memutasi state)                         | `http://127.0.0.1:8090` |
| `PLAYWRIGHT_VISUAL_BASE_URL` | project `visual`       | URL server segar untuk snapshot                                           | `http://127.0.0.1:8089` |
| `CI`                         | `playwright.config.ts` | `true` → `forbidOnly`, retry 1, reporter github, server selalu start baru | unset                   |
| `PERF_SOAK_SECONDS`          | `perf.spec.ts`         | `>0` → jalankan soak SSE selama N detik                                   | `0` (skip)              |
| `VITE_*`                     | Vite                   | hanya variabel berprefiks `VITE_` yang terekspos ke klien                 | —                       |

Playwright menjalankan **tiga** `lab serve --demo`: `:8088` fungsional, `:8089`
visual (snapshot deterministik), `:8090` a11y (spec a11y memutasi routing/approval,
jadi harus terpisah dari spec U2/U4).

## Contoh curl (API yang dipakai konsol)

Browser **hanya** bicara ke `lab serve`. Mutasi butuh `Origin` lab dan header
`X-Lab-Token` (token disuntikkan ke halaman sebagai `window.__LAB_TOKEN__`,
bukan localStorage). `X-Lab-Actor` dipakai untuk atribusi audit bila ada.

```sh
BASE=http://127.0.0.1:8088

# baca (GET, tanpa token)
curl -s "$BASE/api/v1/session"
curl -s "$BASE/api/v1/health/stack"
curl -s "$BASE/api/v1/payments?limit=5"
curl -s "$BASE/api/v1/routing"
curl -s "$BASE/api/v1/slos"

# token dari halaman HTML (script nonce)
TOKEN=$(curl -s "$BASE/" | sed -n 's/.*window.__LAB_TOKEN__="\([^"]*\)".*/\1/p' | head -1)

# mutasi (PUT, butuh Origin + token) — contoh set laju traffic
curl -s -X PUT "$BASE/api/v1/traffic" \
  -H "Origin: $BASE" -H "X-Lab-Token: $TOKEN" -H "Content-Type: application/json" \
  -d '{"ratePerSec":5,"source":"loadgen"}'

# mutasi tanpa token -> 403 problem+json
curl -s -o /dev/null -w '%{http_code}\n' -X PUT "$BASE/api/v1/traffic" \
  -H "Origin: $BASE" -H "Content-Type: application/json" \
  -d '{"ratePerSec":5,"source":"loadgen"}'
```

Kontrak lengkap: [`../api/openapi.yaml`](../api/openapi.yaml).

## Keamanan & privasi

- CSP ketat (tanpa `unsafe-eval`/`unsafe-inline`; `script-src` via nonce),
  `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, tanpa CORS.
- Host allowlist (`127.0.0.1`/`localhost`/`[::1]` + port) → menolak DNS rebinding.
- Mutasi wajib `Origin` lab + `X-Lab-Token` (konstanta-waktu), tercatat append-only.
- Embed/`connect` hanya ke Grafana loopback (`http://127.0.0.1:3002`).

Detail + hasil uji: `docs/UI-SPEC.md` §KEAMANAN.
