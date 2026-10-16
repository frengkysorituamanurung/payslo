import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';

// U7 acceptance: iframe Grafana termuat di Chromium; bila embedding gagal,
// fallback muncul (disimulasikan dengan memblokir request ke Grafana);
// hanya request loopback; axe hijau.

test('observe: axe hijau + hanya request loopback', async ({ page }) => {
  const bad: string[] = [];
  page.on('request', (req) => {
    const host = new URL(req.url()).hostname;
    if (host !== '127.0.0.1' && host !== 'localhost') bad.push(req.url());
  });
  await page.goto('/observe');
  await expect(page.getByRole('heading', { name: 'Observability' })).toBeVisible();
  await expect(page.getByRole('tab', { name: 'Grafana' })).toBeVisible();
  expect(bad).toEqual([]);
  const results = await new AxeBuilder({ page }).analyze();
  const serious = results.violations.filter(
    (v) => v.impact === 'serious' || v.impact === 'critical',
  );
  expect(serious).toEqual([]);
});

test('observe: banner obs-full mati + anotasi tampil', async ({ page }) => {
  await page.goto('/observe');
  await expect(page.getByText(/Profil obs-full tidak aktif/)).toBeVisible();
  await expect(page.getByText('gateway deploy good via lab')).toBeVisible();
});

test('observe: iframe diblokir -> fallback buka tab baru muncul', async ({ page }) => {
  // Simulasikan embedding salah dikonfigurasi: semua request iframe Grafana gagal.
  await page.route(/127\.0\.0\.1:3002\/d\//, (route) => route.abort());
  await page.goto('/observe');
  await expect(page.getByRole('heading', { name: 'Observability' })).toBeVisible();
  // Fallback muncul maksimal setelah timeout deteksi 10 dtk (+ margin).
  await expect(page.getByText('Embed Grafana diblokir atau tak termuat.')).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByRole('link', { name: 'Buka di tab baru' }).first()).toBeVisible();
});

test('settings: axe hijau + diagnostik tanpa docker', async ({ page }) => {
  await page.goto('/settings');
  await expect(page.getByRole('heading', { name: 'Pengaturan', level: 1 })).toBeVisible();
  await expect(page.getByRole('note')).toContainText('Docker tidak tersedia');
  const results = await new AxeBuilder({ page }).analyze();
  const serious = results.violations.filter(
    (v) => v.impact === 'serious' || v.impact === 'critical',
  );
  expect(serious).toEqual([]);
});
