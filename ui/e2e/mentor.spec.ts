import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';

// U6: mode mentor — trainee tidak bisa akses /mentor (403 + UI tidak
// menampilkan menu). axe harus hijau di halaman mentor (mode mentor).

test('mentor: trainee tidak melihat menu mentor', async ({ page }) => {
  await page.goto('/');
  // Menu mentor tidak ada di navigasi trainee.
  await expect(page.getByRole('link', { name: 'Mentor' })).toHaveCount(0);
  // Tombol unlock mentor ada di topbar.
  await expect(page.getByRole('button', { name: /Mode mentor/ })).toBeVisible();
});

test('mentor: /mentor menampilkan 403 untuk trainee', async ({ page }) => {
  const response = await page.goto('/mentor');
  // Server mengembalikan 403 problem+json (bukan SPA).
  expect(response?.status()).toBe(403);
  const body = (await response?.json()) as { title?: string };
  expect(body).toHaveProperty('title');
  expect(String(body.title)).toContain('Mode mentor');
  // Menu navigasi mentor tidak ada di halaman manapun untuk trainee.
  await page.goto('/');
  await expect(page.getByRole('navigation').getByRole('link', { name: 'Mentor' })).toHaveCount(0);
});

test('learn: axe hijau + hanya request loopback', async ({ page }) => {
  const bad: string[] = [];
  page.on('request', (req) => {
    const host = new URL(req.url()).hostname;
    if (host !== '127.0.0.1' && host !== 'localhost') bad.push(req.url());
  });
  await page.goto('/learn');
  await expect(page.getByText(/day-01 — Hari 1/)).toBeVisible();
  await expect(page.getByText('Glosarium')).toBeVisible();
  expect(bad).toEqual([]);
  const results = await new AxeBuilder({ page }).analyze();
  const serious = results.violations.filter(
    (v) => v.impact === 'serious' || v.impact === 'critical',
  );
  expect(serious).toEqual([]);
});
