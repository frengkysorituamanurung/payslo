import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';

// U4 acceptance (demo): insiden terlihat -> diagnostik jalan -> minta
// approval -> toast approval baru via SSE -> approve (ConfirmDialog) ->
// tereksekusi di timeline. axe bersih, loopback saja (U7).
test('incidents: diagnostik -> approval -> tereksekusi, toast realtime, axe bersih', async ({
  page,
}) => {
  const bad: string[] = [];
  page.on('request', (req) => {
    const host = new URL(req.url()).hostname;
    if (host !== '127.0.0.1' && host !== 'localhost') bad.push(req.url());
  });

  await page.goto('/incidents');
  await expect(page.getByRole('heading', { name: 'Insiden', exact: true })).toBeVisible();
  await page.getByRole('button', { name: /payment-success-rate on gateway-api/ }).click();
  await expect(page.getByText('Insiden: payment-success-rate on gateway-api (page)')).toBeVisible();

  // Diagnostik berjalan dan menampilkan output.
  const diag = page.getByRole('region', { name: 'profil bank-bca' });
  await diag.getByRole('button', { name: 'Jalankan diagnostik' }).click();
  await expect(diag.getByText(/demo output/)).toBeVisible();

  // Remediation: tak ada tombol jalan langsung.
  const remediation = page.getByRole('region', { name: 'nonaktifkan provider bca' });
  await expect(remediation.getByRole('button', { name: /Jalankan|Eksekusi/ })).toHaveCount(0);

  // Nama approver dari pengaturan.
  await page.getByLabel('Nama trainee').fill('sinta');
  await page.getByRole('button', { name: 'Simpan nama' }).click();
  await expect(page.getByText('Nama tersimpan.')).toBeVisible();

  await remediation.getByRole('button', { name: 'Minta persetujuan' }).click();
  await expect(remediation.getByText('Menunggu keputusan')).toBeVisible();

  // Toast approval baru tiba via SSE (≤15 detik).
  await expect(page.getByText(/Approval baru: disable provider bca/)).toBeVisible({
    timeout: 15000,
  });
  // Badge antrean di navigasi.
  await expect(page.getByRole('link', { name: /1 approval menunggu/ })).toBeVisible({
    timeout: 15000,
  });

  await remediation.getByRole('button', { name: 'Approve…' }).click();
  await page
    .getByRole('textbox', { name: 'disable provider bca', exact: true })
    .fill('disable provider bca');
  await page.getByRole('button', { name: 'Konfirmasi' }).click();
  await expect(remediation.getByText(/approved oleh sinta/)).toBeVisible();
  await expect(page.getByText(/remediation executed by sinta/)).toBeVisible();

  expect(bad).toEqual([]);
  const results = await new AxeBuilder({ page }).analyze();
  const serious = results.violations.filter(
    (v) => v.impact === 'serious' || v.impact === 'critical',
  );
  expect(serious).toEqual([]);
});
