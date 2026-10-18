import { expect, test } from '@playwright/test';

// Trace dimatikan untuk file ini: soak 1 jam menghasilkan trace raksasa dan
// bisa gagal menulis artefak. Screenshot-on-failure tetap berguna; cukup trace off.
test.use({ trace: 'off' });

// U8 — PERFORMA.
//  - Pengukuran bundle gzip: ui/scripts/check-bundle.mjs (budget UI-SPEC
//    PERFORMA). Di sini diuji konsekuensinya: code-splitting per rute benar
//    (chunk CodeMirror TIDAK dimuat di halaman non-editor).
//  - Metrik setara Lighthouse (FCP, DCL, transfer) via performance API lokal.
//  - Soak SSE: opt-in PERF_SOAK_SECONDS (>0). Uji memori tab saat SSE terbuka
//    (mendeteksi kebocoran). Laporan angka ada di docs/UI-SPEC.md.

test('perf: rute non-editor tidak memuat chunk CodeMirror (code splitting)', async ({ page }) => {
  const scripts: string[] = [];
  page.on('request', (req) => {
    if (req.resourceType() === 'script') scripts.push(new URL(req.url()).pathname);
  });
  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  await page.waitForTimeout(500);
  expect(scripts.some((s) => /SloCodeMirror/i.test(s))).toBe(false);

  // Membuka /slo-lah yang memuat chunk itu (lazy).
  await page.goto('/slo');
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  await page.getByRole('button', { name: /payment-success-rate/ }).click();
  await expect(page.locator('#slo-editor-wrap .cm-content')).toBeVisible();
  expect(scripts.some((s) => /SloCodeMirror/i.test(s))).toBe(true);
});

test('perf: FCP + DCL lokal dalam anggaran (setara Lighthouse)', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  // Paint timing diposting setelah DCL dan bisa sedikit terlambat pada browser
  // yang sudah "panas" dari spec sebelumnya; tunggu entri FCP muncul dulu
  // (bukan menaikkan ambang) supaya pengukuran tidak balapan.
  await expect
    .poll(
      () =>
        page.evaluate(() =>
          performance.getEntriesByType('paint').some((p) => p.name === 'first-contentful-paint'),
        ),
      { timeout: 5000, message: 'entri FCP tidak pernah muncul' },
    )
    .toBe(true);
  const m = await page.evaluate(() => {
    const paint = performance.getEntriesByType('paint');
    const fcp = paint.find((p) => p.name === 'first-contentful-paint');
    const nav = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming;
    return {
      fcp: fcp?.startTime ?? -1,
      dcl: nav.domContentLoadedEventEnd,
      load: nav.loadEventEnd,
      resources: performance.getEntriesByType('resource').length,
    };
  });
  console.log(
    `[perf-fcp] fcp=${m.fcp.toFixed(0)}ms dcl=${m.dcl.toFixed(0)}ms ` +
      `load=${m.load.toFixed(0)}ms resources=${String(m.resources)}`,
  );
  // Ambang lokal yang longgar tapi bermakna (bukan Lighthouse penuh yang
  // butuh Chrome+audit network; ini metrik setara: FCP/DCL/resource count).
  expect(m.fcp).toBeGreaterThan(0);
  expect(m.fcp).toBeLessThan(2500);
  expect(m.dcl).toBeLessThan(3500);
  expect(m.resources).toBeLessThan(80);
});

test('perf soak: SSE terbuka tanpa kebocoran memori berarti', async ({ page }) => {
  const seconds = Number(process.env['PERF_SOAK_SECONDS'] ?? '0');
  test.skip(!Number.isFinite(seconds) || seconds <= 0, 'opt-in: PERF_SOAK_SECONDS=3600');
  test.setTimeout(seconds * 1000 + 120_000);

  const cdp = await page.context().newCDPSession(page);
  const heap = async (): Promise<number> => {
    const u = (await cdp.send('Runtime.getHeapUsage')) as { usedSize: number };
    return u.usedSize;
  };

  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  // SSE harus benar-benar terbuka.
  await expect(page.getByText('Live')).toBeVisible({ timeout: 10000 });

  // Warmup 60 dtk (buang alokasi awal), lalu ukur tiap 30 dtk.
  const warmupMs = 60_000;
  const stepMs = 30_000;
  await page.waitForTimeout(warmupMs);
  const start = await heap();
  const samples: number[] = [start];
  const steps = Math.max(1, Math.floor((seconds * 1000 - warmupMs) / stepMs));
  for (let i = 0; i < steps; i += 1) {
    await page.waitForTimeout(stepMs);
    samples.push(await heap());
  }
  const end = samples[samples.length - 1] ?? start;
  const grewMB = (end - start) / (1024 * 1024);
  console.log(
    `[perf-soak] seconds=${String(seconds)} start=${(start / 1048576).toFixed(1)}MB ` +
      `end=${(end / 1048576).toFixed(1)}MB grow=${grewMB.toFixed(1)}MB ` +
      `samples=${samples.map((s) => (s / 1048576).toFixed(0)).join(',')}`,
  );
  // Toleransi 20MB untuk soak 1 jam (deteksi kebocoran kasar).
  expect(grewMB).toBeLessThan(20);
});
