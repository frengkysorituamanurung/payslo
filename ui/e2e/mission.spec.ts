import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';

// U7: tidak ada request ke host selain loopback.
test('mission: axe bersih + hanya request 127.0.0.1', async ({ page }) => {
  const bad: string[] = [];
  page.on('request', (req) => {
    const host = new URL(req.url()).hostname;
    if (host !== '127.0.0.1' && host !== 'localhost') bad.push(req.url());
  });
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Mission Control' })).toBeVisible();
  await expect(page.getByText('98.5%').first()).toBeVisible();
  expect(bad).toEqual([]);
  const results = await new AxeBuilder({ page }).analyze();
  const serious = results.violations.filter(
    (v) => v.impact === 'serious' || v.impact === 'critical',
  );
  expect(serious).toEqual([]);
});

test('mission: visual snapshot demo stabil', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByText('98.5%').first()).toBeVisible();
  // Tunggu font self-hosted termuat agar screenshot deterministik.
  await page.evaluate(() => document.fonts.ready);
  await expect(page).toHaveScreenshot('mission-demo.png', { fullPage: true });
});
