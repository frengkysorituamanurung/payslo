import { expect, test } from '@playwright/test';
import { PAGES, cspViolations, freeze, trackOutbound } from './lib';

// U8 — KEAMANAN (ulang U0 + tambahan).
//  - header keamanan di setiap respons dokumen (CSP, nosniff, referrer),
//  - CSP: NOL pelanggaran di console pada semua halaman,
//  - hanya request ke 127.0.0.1 (U7), tanpa request keluar,
//  - tanpa header CORS,
//  - mutasi tanpa token ditolak 403; dengan token lab diterima (bukan 403).
// DNS rebinding (Host asing) diuji di sisi server: cmd/lab/security_u8_test.go.

test('keamanan: header keamanan ada di setiap halaman', async ({ page }) => {
  for (const p of PAGES) {
    const res = await page.goto(p.path);
    expect(res, `respons kosong untuk ${p.path}`).not.toBeNull();
    const h = res?.headers() ?? {};
    expect(h['content-security-policy'], p.path).toContain("default-src 'self'");
    expect(h['content-security-policy'], p.path).toContain("object-src 'none'");
    expect(h['x-content-type-options'], p.path).toBe('nosniff');
    expect(h['referrer-policy'], p.path).toBe('no-referrer');
    // Tidak ada header CORS.
    for (const k of Object.keys(h)) {
      expect(k.toLowerCase().startsWith('access-control-'), `${p.path}: ${k}`).toBe(false);
    }
  }
});

test('keamanan: nol pelanggaran CSP di console (semua halaman)', async ({ page }) => {
  await freeze(page);
  const consoleCsp: string[] = [];
  page.on('console', (msg) => {
    if (msg.type() === 'error' && /content security policy/i.test(msg.text())) {
      consoleCsp.push(msg.text());
    }
  });
  const pageErrors: string[] = [];
  page.on('pageerror', (err) => pageErrors.push(err.message));

  for (const p of PAGES) {
    await page.goto(p.path);
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
    expect(await cspViolations(page), `CSP violation di ${p.path}`).toEqual([]);
  }
  expect(consoleCsp).toEqual([]);
});

test('keamanan: hanya request ke loopback di seluruh alur', async ({ page }) => {
  const bad = trackOutbound(page);
  for (const p of PAGES) {
    await page.goto(p.path);
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  }
  // Lakukan satu mutasi nyata (create payment) — tetap tanpa keluar loopback.
  await page.goto('/payments');
  await page.getByLabel('Nominal (IDR)').fill('12345');
  await page.getByRole('button', { name: 'Kirim' }).click();
  await expect(page.getByText(/Pembayaran dibuat/)).toBeVisible();
  expect(bad).toEqual([]);
});

test('keamanan: mutasi tanpa X-Lab-Token ditolak 403, dengan token lab diterima', async ({
  page,
}) => {
  await page.goto('/');
  const without = await page.evaluate(async () => {
    const r = await fetch('/api/v1/traffic', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ rate: 5 }),
    });
    return r.status;
  });
  expect(without).toBe(403);

  const withToken = await page.evaluate(async () => {
    const r = await fetch('/api/v1/traffic', {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'X-Lab-Token': window.__LAB_TOKEN__ ?? '',
      },
      body: JSON.stringify({ rate: 5 }),
    });
    return r.status;
  });
  // Guard lolos; validasi domain bisa 200/400/422 — yang penting BUKAN 403.
  expect(withToken).not.toBe(403);
});

test('keamanan: Origin asing tidak menemukan respons (tanpa CORS)', async ({ request }) => {
  // Playwright request (bukan browser) tanpa Origin: respons normal, tanpa
  // Access-Control-Allow-*. Ketiadaan CORS diuji di Go (TestNoCORSHeaders).
  const res = await request.get('/api/v1/session');
  expect(res.status()).toBe(200);
  const h = res.headers();
  expect(h['access-control-allow-origin']).toBeUndefined();
});
