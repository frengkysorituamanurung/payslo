import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';

// U2 acceptance: menonaktifkan provider lewat UI mengubah routing (diverifikasi
// round-trip via API: bertahan setelah reload) + tercatat di audit; axe bersih.
test('control: nonaktifkan provider -> routing berubah + audit, axe bersih', async ({ page }) => {
  const bad: string[] = [];
  page.on('request', (req) => {
    const host = new URL(req.url()).hostname;
    if (host !== '127.0.0.1' && host !== 'localhost') bad.push(req.url());
  });

  await page.goto('/control');
  await expect(page.getByRole('heading', { name: 'Chaos & Kontrol' })).toBeVisible();

  const bca = page.getByRole('checkbox', { name: 'bca Aktif' });
  await expect(bca).toBeChecked();

  await bca.click();
  await page.getByRole('button', { name: 'Simpan routing' }).click();
  await page.getByRole('textbox', { name: 'routing', exact: true }).fill('routing');
  await page.getByRole('button', { name: 'Konfirmasi' }).click();

  // Tercatat di audit (aktor + aksi).
  await expect(page.getByText('routing.set').first()).toBeVisible();

  // Bertahan setelah reload: membuktikan perubahan benar-benar tersimpan di API.
  await page.reload();
  await expect(page.getByRole('checkbox', { name: 'bca Aktif' })).not.toBeChecked();

  expect(bad).toEqual([]);
  const results = await new AxeBuilder({ page }).analyze();
  const serious = results.violations.filter(
    (v) => v.impact === 'serious' || v.impact === 'critical',
  );
  expect(serious).toEqual([]);
});
