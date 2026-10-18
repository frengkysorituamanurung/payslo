import { expect, test } from '@playwright/test';
import {
  PAGES,
  activeDescriptor,
  axeNoSerious,
  cspViolations,
  freeze,
  hOverflow,
  setLang,
  trackOutbound,
} from './lib';

// U8 — AKSESIBILITAS (WCAG 2.2 AA).
//  - axe: nol pelanggaran serious/critical di SEMUA halaman, KEDUA tema,
//    KEDUA bahasa.
//  - keyboard-only: setiap kontrol utama dapat dicapai & dioperasikan Tab/
//    Shift+Tab/Enter/Space; indikator fokus terlihat.
//  - aria-live: alert/approval diumumkan.
//  - prefers-reduced-motion dihormati; zoom 200% tanpa hilang konten.
// Catatan uji manual (TalkBack/VoiceOver/NVDA) dicatat di docs/UI-SPEC.md.

for (const p of PAGES) {
  test(`a11y ${p.name}: axe bersih (2 tema x 2 bahasa) + landmark + CSP`, async ({ page }) => {
    await freeze(page);
    const bad = trackOutbound(page);
    await page.goto(p.path);
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();

    for (const theme of ['light', 'dark'] as const) {
      await page.getByTestId('theme-select').selectOption(theme);
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
      for (const lang of ['id', 'en'] as const) {
        await setLang(page, lang);
        const heading = lang === 'id' ? p.headingId : p.headingEn;
        await expect(page.getByRole('heading', { level: 1, name: heading })).toBeVisible();
        // Tepat satu h1 per halaman (struktur judul jelas untuk pembaca layar).
        await expect(page.locator('h1')).toHaveCount(1);
        // Landmark utama + navigasi.
        await expect(page.getByRole('main')).toHaveCount(1);
        await expect(page.getByRole('navigation')).toHaveCount(1);
        await axeNoSerious(page);
      }
    }

    expect(await cspViolations(page)).toEqual([]);
    expect(bad).toEqual([]);
  });
}

test('a11y keyboard: Tab menjangkau nav, topbar, dan konten; fokus terlihat', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();

  const seen = new Set<string>();
  const invisible: string[] = [];
  const noFocusRing: string[] = [];
  for (let i = 0; i < 40; i += 1) {
    await page.keyboard.press('Tab');
    const desc = await activeDescriptor(page);
    if (desc === 'null' || desc.startsWith('body:')) continue;
    seen.add(desc);
    const info = await page.evaluate(() => {
      const el = document.activeElement;
      if (el === null) return null;
      const cs = getComputedStyle(el);
      const r = el.getBoundingClientRect();
      return {
        visible: r.width > 0 && r.height > 0 && cs.visibility !== 'hidden',
        ring: cs.outlineStyle !== 'none' || cs.boxShadow !== 'none',
      };
    });
    if (info !== null && !info.visible) invisible.push(desc);
    if (info !== null && !info.ring) noFocusRing.push(desc);
  }

  // Nav (link Mission Control), topbar (tema/bahasa), dan kontrol di main
  // semuanya tercapai lewat Tab.
  const joined = [...seen].join('|');
  expect(joined).toContain('Mission Control');
  expect(joined.toLowerCase()).toContain('select');
  expect(seen.size).toBeGreaterThanOrEqual(8);
  expect(invisible).toEqual([]);
  expect(noFocusRing).toEqual([]);
});

test('a11y keyboard-only: mitigasi routing selesai tanpa mouse', async ({ page }) => {
  await page.goto('/control');
  await expect(page.getByRole('heading', { level: 1, name: 'Chaos & Kontrol' })).toBeVisible();

  const bca = page.getByRole('checkbox', { name: 'bca Aktif' });
  await bca.focus();
  await page.keyboard.press('Space');
  await expect(bca).not.toBeChecked();

  // Simpan routing -> dialog konfirmasi (Radix mengelola fokus + Escape).
  await page.getByRole('button', { name: 'Simpan routing' }).focus();
  await page.keyboard.press('Enter');
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  await page.keyboard.type('routing');
  await page.keyboard.press('Enter');
  await expect(dialog).toBeHidden();

  // Aksi tercatat di audit (bukti tersimpan ke server).
  await expect(page.getByText('routing.set').first()).toBeVisible();
  // Bertahan setelah reload.
  await page.reload();
  await expect(page.getByRole('checkbox', { name: 'bca Aktif' })).not.toBeChecked();
});

test('a11y pembaca layar dasar: peran & nama aksesibel (tidak warna saja)', async ({ page }) => {
  await page.goto('/');
  // Nav punya nama + link bernama.
  const nav = page.getByRole('navigation');
  await expect(nav).toBeVisible();
  await expect(nav.getByRole('link', { name: 'Mission Control' })).toBeVisible();
  // Status stack = role=status dengan teks (bukan warna saja).
  await expect(page.getByRole('status').first()).toBeVisible();
  // Region live untuk toast ada dan polite.
  await expect(page.locator('[aria-live="polite"]')).toHaveCount(1);
  // Status memakai ikon + teks: aria-label memuat teks status.
  const statuses = page.getByRole('status');
  expect(await statuses.count()).toBeGreaterThan(0);
});

test('a11y aria-live: approval baru diumumkan lewat region live', async ({ page }) => {
  await page.goto('/incidents');
  await expect(page.getByRole('heading', { level: 1, name: 'Insiden' })).toBeVisible();
  await page.getByRole('button', { name: /payment-success-rate on gateway-api/ }).click();
  const remediation = page.getByRole('region', { name: 'nonaktifkan provider bca' });
  await remediation.getByRole('button', { name: 'Minta persetujuan' }).click();
  // Toast approval muncul DI DALAM region aria-live=polite.
  const live = page.locator('[aria-live="polite"]');
  await expect(live.getByText(/Approval baru: disable provider bca/)).toBeVisible({
    timeout: 15000,
  });
});

test('a11y prefers-reduced-motion: transisi/animasi dinetralkan', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  const durations = await page.evaluate(() => {
    const cs = getComputedStyle(document.body);
    return { t: parseFloat(cs.transitionDuration), a: parseFloat(cs.animationDuration) };
  });
  expect(durations.t).toBeLessThan(0.1);
  expect(durations.a).toBeLessThan(0.1);
});

test('a11y zoom 200%: tidak ada konten hilang / scroll horizontal', async ({ page }) => {
  // 200% zoom pada layar 1280 = viewport efektif 640 CSS px (definisi
  // WCAG 1.4.4 lewat zoom browser). Semua halaman harus tetap terbaca.
  await page.setViewportSize({ width: 640, height: 720 });
  for (const p of PAGES) {
    await page.goto(p.path);
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
    expect(await hOverflow(page), `overflow horizontal di ${p.path} @200%`).toBeLessThanOrEqual(1);
  }
});
