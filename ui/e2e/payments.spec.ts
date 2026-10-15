import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';

// U2 acceptance: buat -> tabel -> detail/timeline -> status bank, axe bersih,
// dan hanya request loopback (U7).
test('payments: buat -> tabel -> timeline -> bank, axe bersih, loopback saja', async ({ page }) => {
  const bad: string[] = [];
  page.on('request', (req) => {
    const host = new URL(req.url()).hostname;
    if (host !== '127.0.0.1' && host !== 'localhost') bad.push(req.url());
  });

  await page.goto('/payments');
  await expect(page.getByRole('heading', { name: 'Pembayaran', exact: true })).toBeVisible();
  await expect(page.getByText('demo-order-002')).toBeVisible();
  // Tandai duplikat secara netral (fixture demo 004 punya 2 callback).
  await expect(page.getByText('callback diterima 2x')).toBeVisible();

  // Buat pembayaran uji -> muncul di tabel.
  await page.getByLabel('Nominal (IDR)').fill('250000');
  const order = await page.getByLabel('Order ID').inputValue();
  await page.getByRole('button', { name: 'Kirim' }).click();
  await expect(page.getByText(/Pembayaran dibuat/)).toBeVisible();
  await expect(page.getByRole('row', { name: new RegExp(order) })).toBeVisible();

  // Detail + timeline + status bank.
  await page
    .getByRole('row', { name: /demo-order-004/ })
    .getByRole('button', { name: 'Lihat' })
    .click();
  await expect(page.getByRole('heading', { name: 'Detail transaksi' })).toBeVisible();
  await expect(page.getByText('Callback diterima').first()).toBeVisible();
  await expect(page.getByText('Webhook dikirim').first()).toBeVisible();
  await page.getByRole('button', { name: 'Cek status ke bank' }).click();
  await expect(page.getByText(/bank bca berkata: SUCCESS/)).toBeVisible();

  expect(bad).toEqual([]);
  const results = await new AxeBuilder({ page }).analyze();
  const serious = results.violations.filter(
    (v) => v.impact === 'serious' || v.impact === 'critical',
  );
  expect(serious).toEqual([]);
});
