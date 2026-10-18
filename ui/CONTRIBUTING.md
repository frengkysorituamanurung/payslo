# Panduan Kontributor — PaySLO Lab Console

Aturan umum proyek ada di [`../AGENTS.MD`](../AGENTS.MD); spesifikasi UI di
[`../docs/UI-SPEC.md`](../docs/UI-SPEC.md). Halaman ini khusus kontribusi ke
`ui/`.

## Prasyarat

- Node.js ≥ 22, npm.
- Go ≥ versi di `cmd/lab/go.mod` (untuk menjalankan `lab serve`).
- Untuk e2e: browser Playwright (`npx playwright install chromium firefox webkit`).

```sh
make ui-install   # npm ci
```

## Alur kerja wajib

1. **Contract-first.** Bila mengubah API, ubah **dulu** `api/openapi.yaml`.
2. Regenerasi tipe TS: `cd ui && npm run gen:api` (menulis `src/api/schema.d.ts`).
3. Implementasi handler Go di `cmd/lab/` **dan** UI.
4. Tulis/ubah test bersamaan (Vitest untuk logika/komponen; uji kontrak Go;
   Playwright untuk alur/a11y).
5. Jalankan gate sebelum commit:
    ```sh
    make ui-test   # tsc + eslint + vitest + prettier
    make ui-e2e    # playwright (chromium/firefox/webkit)
    cd cmd/lab && go test ./...
    ```
6. Satu perubahan = satu commit kecil (Conventional Commits), DCO sign-off.

## Menambah halaman

1. Buat `src/pages/Nama.tsx` yang meng-export komponen `NamaPage`. Pola status
   data konsisten: `isPending` → `<p>{t('common.loading')}</p>`, `isError` →
   `<ErrorState onRetry=… />`, data kosong → `<EmptyState next=… />`.
2. Daftarkan rute di `src/main.tsx`:
    ```tsx
    { path: 'nama', element: <NamaPage /> },
    ```
3. Tambahkan entri navigasi di `src/components/AppShell.tsx` (array `NAV`) dan
   judul halaman. Sembunyikan dari trainee bila mentor-only (contoh `/mentor`).
4. Tambahkan string i18n **di kedua locale** `src/i18n/locales/{id,en}.json`
   (`nav.nama`, dst.). DILARANG string hardcoded di komponen (aturan UI-SPEC 8).
5. Judul halaman: tepat **satu `<h1>`** dengan `id` unik, `aria-labelledby` pada
   `<section>` pembungkus.
6. Test: `src/pages/Nama.test.tsx` (Vitest + MSW) dan tambahkan entri ke
   `PAGES` di `e2e/lib.ts` (nama, path, `headingId`, `headingEn`) agar matriks
   a11y/kompatibilitas ikut menguji halaman baru.

## Menambah komponen

- Taruh di `src/components/` (satu file per komponen, tanpa state global).
- Pakai token desain dari `src/styles/tokens.css` (`var(--ok)`, `var(--surface)`,
  dst.) — jangan nilai warna literal agar tema light/dark konsisten.
- Tabel dengan banyak kolom: bungkus dengan `src/components/TableScroll.tsx`
  agar bisa di-scroll horizontal (fokusabel, aksesibel) pada layar sempit.
- Aksesibilitas: beri `role`/`aria-label`; status **jangan** hanya warna
  (sertakan teks/ikon + `aria-label`). Uji dengan `axe` (lihat `e2e/a11y.spec.ts`).
- Uang selalu integer IDR (`src/utils/money.ts`); jangan `f32`/`f64`/pecahan.
- Keadaan khusus: loading/empty/error/stale/partial (UI-SPEC §STATE).

## Menambah endpoint

1. Tambahkan operasi di `api/openapi.yaml` (path, method, skema request/response,
   error RFC 9457 `application/problem+json`).
2. Regenerasi tipe: `cd ui && npm run gen:api`.
3. Implementasi handler Go di `cmd/lab/` dan daftarkan di router
   (`server.go`). Endpoint mutasi lewat `mutationGuard` otomatis (Origin + token).
4. **Uji kontrak**: `cmd/lab/server_test.go` membandingkan rute handler vs
   `openapi.yaml` (`loadSpecOps`). Tambahkan test spesifik perilaku (baca:
   `u2_test.go`, `u4_test.go`, `u7_test.go`).
5. Buat hook React di `src/hooks/` (React Query) yang memakai klien
   `src/api/client.ts`; jangan memanggil `fetch` langsung dari komponen.
6. E2E: tambahkan skenario di `e2e/` bila alurnya penting bagi trainee.

## Aksesibilitas (WAJIB, WCAG 2.2 AA)

- Keyboard-only: semua kontrol dapat dicapai Tab/Shift+Tab dan dioperasikan
  Enter/Space; indikator fokus terlihat (lihat `e2e/a11y.spec.ts`).
- Dialog konfirmasi memakai Radix (`@radix-ui/react-dialog`) → focus trap +
  Escape otomatis.
- Region `aria-live="polite"` (toast) untuk pengumuman insiden/approval.
- `prefers-reduced-motion` dihormati; `document.documentElement.lang` mengikuti
  bahasa aktif.
- Jalankan `npx playwright test a11y.spec.ts --project=a11y` — harus 0
  pelanggaran `serious`/`critical` axe di semua halaman × tema × bahasa. Project
  `a11y` memakai server demo segar sendiri (`:8090`) karena spec ini memutasi
  state (routing dinonaktifkan, approval dibuat).

## Keamanan & privasi (WAJIB)

- Tanpa request ke luar `127.0.0.1` (dites otomatis, `e2e/security.spec.ts`).
- Tanpa CDN/font eksternal/analytics.
- Jangan memakai `eval`/`new Function` (CSP `script-src` tanpa `unsafe-eval`).
  Zod dijalankan mode `jitless` lewat `src/zod-setup.ts` (impor pertama di
  `main.tsx`) untuk mematikan probe `new Function`-nya.
- Jangan menulis token/secret ke `localStorage`; token lab hanya di
  `window.__LAB_TOKEN__` (disuntikkan per-request dengan nonce).

## Visual regression

- Snapshot Playwright per-platform (`*-visual-<platform>.png`). Halaman utama ×
  2 tema. Server demo segar (:8089) via `webServer` agar deterministik.
- Perbarui baseline dengan sengaja, tinjau diff:
    - macOS: `npx playwright test visual.spec.ts --project=visual --update-snapshots`
    - Linux (dipakai CI): jalankan di image `mcr.microsoft.com/playwright:v1.64.1-noble`
      (lihat `docs/DECISIONS.md` §U8-visual-linux).
- Jangan menaikkan `maxDiffPixelRatio` tanpa alasan; perbaiki sumber
  non-determinisme (bekukan waktu/acak via `freeze()` di `e2e/lib.ts`).
