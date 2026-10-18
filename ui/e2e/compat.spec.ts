import { expect, test } from '@playwright/test';
import { PAGES, axeNoSerious, freeze, hOverflow, settle } from './lib';

// U8 — KOMPATIBILITAS.
// Dijalankan di chromium, firefox, dan webkit (Safari) via project di
// playwright.config.ts. Setiap halaman: axe bersih + tanpa overflow
// horizontal pada lebar 1280 / 1024 / 768 (UI-SPEC: layar utama 1280, tablet
// 768 harus dapat dipakai). Batas yang diketahui dicatat di docs/UI-SPEC.md.
// Waktu + acak dibekukan agar layout deterministik (chart live bisa menggeser
// lebar sesaat dan membuat snapshot overflow flaky).

for (const p of PAGES) {
  test(`compat ${p.name}: axe + layout 1280/1024/768`, async ({ page, browserName }) => {
    await freeze(page);
    await page.goto(p.path);
    await expect(page.getByRole('heading', { level: 1, name: p.headingId })).toBeVisible();
    await settle(page);
    await axeNoSerious(page);
    for (const width of [1280, 1024, 768]) {
      await page.setViewportSize({ width, height: 800 });
      await expect(page.getByRole('heading', { level: 1, name: p.headingId })).toBeVisible();
      await settle(page);
      expect(
        await hOverflow(page),
        `${browserName} ${p.path} @${String(width)}px overflow horizontal`,
      ).toBeLessThanOrEqual(1);
    }
  });
}

test('compat: alur inti (buat pembayaran) bekerja di semua browser', async ({ page }) => {
  await page.goto('/payments');
  await expect(page.getByRole('heading', { level: 1, name: 'Pembayaran' })).toBeVisible();
  await page.getByLabel('Nominal (IDR)').fill('99000');
  await page.getByRole('button', { name: 'Kirim' }).click();
  await expect(page.getByText(/Pembayaran dibuat/)).toBeVisible();
  await expect(page.getByRole('table')).toBeVisible();
});
