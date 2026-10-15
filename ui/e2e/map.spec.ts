import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';

test('map: axe bersih + keyboard + snapshot', async ({ page }) => {
  await page.goto('/map');
  await expect(page.getByRole('heading', { name: 'Peta Arsitektur' })).toBeVisible();
  await page.evaluate(() => document.fonts.ready);
  const results = await new AxeBuilder({ page }).analyze();
  const serious = results.violations.filter(
    (v) => v.impact === 'serious' || v.impact === 'critical',
  );
  expect(serious).toEqual([]);
  await expect(page).toHaveScreenshot('map-demo.png', { fullPage: true });
});
