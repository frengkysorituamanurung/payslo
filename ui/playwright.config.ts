import { defineConfig, devices } from '@playwright/test';

// U0–U8 e2e PaySLO Lab Console.
// Server: `lab serve --demo` (fixture deterministik, tanpa Docker).
// Browser hanya boleh bicara ke 127.0.0.1 (U7); tiap spec menegakkannya lewat
// e2e/lib.ts.
//
// DUA server demo:
//   :8088  spec fungsional (security, compat, perf, halaman U0-U7).
//   :8089  KHUSUS visual regression — server segar tanpa mutasi dari spec lain
//          (state demo bisa berubah saat spec lain membuat pembayaran / mengubah
//          routing, yang akan membuat snapshot tidak deterministik).
//   :8090  KHUSUS a11y — spec a11y MEMUTASI state (menonaktifkan provider bca,
//          membuat approval). Bila berbagi server dengan spec U2/U4, mutasi itu
//          merusak asumsi mereka (bca "sudah nonaktif", approval "sudah dibuat").
//          Server terpisah -> tiap suite mulai dari state bersih.
//
// Project:
//   chromium  spec fungsional KECUALI visual & a11y.
//   a11y      a11y.spec.ts vs server segar :8090.
//   visual    visual.spec.ts vs server segar :8089 (baseline per-platform).
//   firefox   compat.spec.ts (lintas-browser + axe per halaman).
//   webkit    compat.spec.ts (WebKit/Safari).
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  forbidOnly: process.env['CI'] === 'true',
  retries: process.env['CI'] === 'true' ? 1 : 0,
  reporter: process.env['CI'] === 'true' ? [['github'], ['list']] : [['list']],
  expect: {
    toHaveScreenshot: {
      animations: 'disabled',
      caret: 'hide',
      scale: 'css',
      maxDiffPixelRatio: 0.02,
    },
  },
  use: {
    baseURL: process.env['PLAYWRIGHT_BASE_URL'] ?? 'http://127.0.0.1:8088',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    viewport: { width: 1280, height: 720 },
    colorScheme: 'light',
    locale: 'id-ID',
    timezoneId: 'Asia/Jakarta',
  },
  webServer: [
    {
      command: './bin/labserve serve --demo --addr 127.0.0.1:8088',
      cwd: '..',
      url: 'http://127.0.0.1:8088/',
      reuseExistingServer: process.env['CI'] !== 'true',
      timeout: 60_000,
    },
    {
      // Server segar khusus visual. Di CI selalu mulai baru (deterministik);
      // saat lokal (non-CI) boleh memakai server yang sudah jalan.
      command: './bin/labserve serve --demo --addr 127.0.0.1:8089',
      cwd: '..',
      url: 'http://127.0.0.1:8089/',
      reuseExistingServer: process.env['CI'] !== 'true',
      timeout: 60_000,
    },
    {
      // Server segar khusus a11y (spec memutasi state: routing, approval).
      command: './bin/labserve serve --demo --addr 127.0.0.1:8090',
      cwd: '..',
      url: 'http://127.0.0.1:8090/',
      reuseExistingServer: process.env['CI'] !== 'true',
      timeout: 60_000,
    },
  ],
  projects: [
    {
      name: 'chromium',
      testIgnore: ['**/visual.spec.ts', '**/a11y.spec.ts'],
      use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 720 } },
    },
    {
      name: 'a11y',
      testMatch: ['**/a11y.spec.ts'],
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1280, height: 720 },
        baseURL: process.env['PLAYWRIGHT_A11Y_BASE_URL'] ?? 'http://127.0.0.1:8090',
      },
    },
    {
      name: 'visual',
      testMatch: ['**/visual.spec.ts'],
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1280, height: 720 },
        baseURL: process.env['PLAYWRIGHT_VISUAL_BASE_URL'] ?? 'http://127.0.0.1:8089',
      },
    },
    {
      name: 'firefox',
      testMatch: ['**/compat.spec.ts'],
      use: { ...devices['Desktop Firefox'], viewport: { width: 1280, height: 720 } },
    },
    {
      name: 'webkit',
      testMatch: ['**/compat.spec.ts'],
      use: { ...devices['Desktop Safari'], viewport: { width: 1280, height: 720 } },
    },
  ],
});
