import { expect, test } from '@playwright/test';
import { PAGES, freeze, gotoThemed, settle } from './lib';

// U8 — VISUAL REGRESSION (Playwright, mode --demo).
// Waktu (Date.now) dibekukan ke demoBase dan Math.random di-seed (lihat
// freeze()) sehingga snapshot deterministik. Dua tema, halaman utama.
// Baseline per-platform (Playwright default) -> tidak bocor antar OS.
// Gagal = perubahan tampilan; perbarui dengan `npx playwright test
// visual.spec.ts --update-snapshots` DAN tinjau diff-nyas.
test.describe('visual regression demo', () => {
  test.skip(({ browserName }) => browserName !== 'chromium', 'baseline visual: Chromium saja');

  for (const p of PAGES) {
    for (const theme of ['light', 'dark'] as const) {
      test(`${p.name} (${theme})`, async ({ page }) => {
        await freeze(page);
        // Halaman /observe menyematkan Grafana eksternal (loopback). Status
        // iframe bergantung pada waktu probe; untuk snapshot yang deterministik
        // paksa jalur fallback (state yang selalu tersedia dan stabil).
        if (p.name === 'observe') {
          await page.route(/127\.0\.0\.1:3002\/d\//, (route) => route.abort());
        }
        await gotoThemed(page, p.path, theme);
        // Default locale = id, jadi judul h1 ada di headingId.
        await expect(page.getByRole('heading', { level: 1, name: p.headingId })).toBeVisible();
        await settle(page);
        if (p.name === 'observe') {
          await expect(page.getByText(/Embed Grafana diblokir/)).toBeVisible();
        }
        await expect(page).toHaveScreenshot(`${p.name}-${theme}.png`, { fullPage: true });
      });
    }
  }
});
