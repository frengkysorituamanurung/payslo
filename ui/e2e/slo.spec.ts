import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';

// U3 acceptance (demo): edit SLO -> validate gagal berlokasi -> perbaiki ->
// plan ber-diff -> apply -> plan ulang kosong -> apply kedua no-op.
// Editor dikemudikan via keyboard nyata (Ctrl+A + ketik), axe bersih,
// hanya request loopback (U7).
test('slo: edit -> validate gagal -> plan diff -> apply -> plan kosong -> no-op', async ({
  page,
}) => {
  const bad: string[] = [];
  page.on('request', (req) => {
    const host = new URL(req.url()).hostname;
    if (host !== '127.0.0.1' && host !== 'localhost') bad.push(req.url());
  });

  const BAD = [
    'apiVersion: payslo/v1',
    'kind: SLO',
    'name: payment-success-rate',
    'service: gateway-api',
    'objective: 99',
    'window: 1h',
    'sli:',
    '  type: success-ratio',
    '  source: metric',
    '  good: payment_transactions_total{status="SUCCESS"}',
    '  total: payment_transactions_total',
    'alerting:',
    '  runbook: runbooks/payment-success.md',
    '  burnRates:',
    '    - longWindow: 5m',
    '      shortWindow: 1m',
    '      burnRate: 14.4',
    '      severity: page',
    'targets:',
    '  - backend: prometheus',
    '',
  ].join('\n');
  const FIXED = BAD.replace('objective: 99', 'objective: 0.995').replace(
    'burnRate: 14.4',
    'burnRate: 12',
  );

  await page.goto('/slo');
  await expect(page.getByRole('heading', { name: 'SLO Center' })).toBeVisible();
  await page.getByRole('button', { name: /payment-success-rate/ }).click();
  await expect(page.getByText('SLO payment-success-rate')).toBeVisible();

  const editor = page.locator('#slo-editor-wrap .cm-content');
  await expect(editor).toBeVisible();
  await editor.click();
  await page.keyboard.press('ControlOrMeta+a');
  await page.keyboard.type(BAD);
  await page.getByRole('button', { name: 'Simpan' }).click();
  await page.waitForResponse(
    (r) => r.url().includes('/api/v1/slos/payment-success-rate') && r.request().method() === 'PUT',
  );
  await expect(page.getByText(/Tersimpan/)).toBeVisible();
  await page.getByRole('button', { name: 'Validasi' }).click();
  await expect(page.getByText(/Baris 5, kolom 1/)).toBeVisible();

  await editor.click();
  await page.keyboard.press('ControlOrMeta+a');
  await page.keyboard.type(FIXED);
  await page.getByRole('button', { name: 'Simpan' }).click();
  await page.waitForResponse(
    (r) => r.url().includes('/api/v1/slos/payment-success-rate') && r.request().method() === 'PUT',
  );
  await expect(page.getByText(/Tersimpan/)).toBeVisible();
  await page.getByRole('button', { name: 'Validasi' }).click();
  await expect(page.getByText(/SLO valid/)).toBeVisible();

  await page.getByRole('button', { name: 'Plan' }).click();
  await expect(page.getByText(/akan diubah di prometheus-local/)).toBeVisible();
  await expect(page.getByRole('cell', { name: 'burnRate: 12' })).toBeVisible();

  await page.getByRole('button', { name: 'Apply…' }).click();
  await page
    .getByRole('textbox', { name: 'payment-success-rate', exact: true })
    .fill('payment-success-rate');
  await page.getByRole('button', { name: 'Konfirmasi' }).click();
  await expect(page.getByText(/applied payment-success-rate on prometheus-local/)).toBeVisible();

  await page.getByRole('button', { name: 'Plan' }).click();
  await expect(page.getByText(/Tidak ada perubahan/)).toBeVisible();
  await page.getByRole('button', { name: 'Apply…' }).click();
  await page
    .getByRole('textbox', { name: 'payment-success-rate', exact: true })
    .fill('payment-success-rate');
  await page.getByRole('button', { name: 'Konfirmasi' }).click();
  await expect(page.getByText(/no-op/)).toBeVisible();

  expect(bad).toEqual([]);
  const results = await new AxeBuilder({ page }).analyze();
  const serious = results.violations.filter(
    (v) => v.impact === 'serious' || v.impact === 'critical',
  );
  expect(serious).toEqual([]);
});
