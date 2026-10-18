import AxeBuilder from '@axe-core/playwright';
import { expect, type Page } from '@playwright/test';

// U8: utilitas bersama untuk audit aksesibilitas, visual, keamanan, dan
// kompatibilitas. Bukan spec (tidak diakhiri .spec.ts) sehingga tak dijalankan
// sebagai test.

// Waktu dasar fixture --demo (identik dengan cmd/lab demoBase).
export const DEMO_TIME = new Date('2026-10-16T07:00:00.000Z');

export type PageDef = {
  name: string;
  path: string;
  /** Judul h1 di locale id (default). */
  headingId: string;
  /** Judul h1 di locale en. */
  headingEn: string;
};

/** Halaman utama (IA/UI-SPEC). /mentor 403 untuk trainee, jadi tak di sini. */
export const PAGES: PageDef[] = [
  { name: 'mission', path: '/', headingId: 'Mission Control', headingEn: 'Mission Control' },
  { name: 'map', path: '/map', headingId: 'Peta Arsitektur', headingEn: 'Architecture Map' },
  { name: 'payments', path: '/payments', headingId: 'Pembayaran', headingEn: 'Payments' },
  { name: 'control', path: '/control', headingId: 'Chaos & Kontrol', headingEn: 'Chaos & Control' },
  { name: 'slo', path: '/slo', headingId: 'SLO Center', headingEn: 'SLO Center' },
  { name: 'incidents', path: '/incidents', headingId: 'Insiden', headingEn: 'Incidents' },
  { name: 'observe', path: '/observe', headingId: 'Observability', headingEn: 'Observability' },
  { name: 'learn', path: '/learn', headingId: 'Belajar', headingEn: 'Learn' },
  { name: 'settings', path: '/settings', headingId: 'Pengaturan', headingEn: 'Settings' },
];

declare global {
  interface Window {
    __cspViolations?: string[];
  }
}

/**
 * Bekukan waktu (Date.now/new Date) pada waktu fixture demo, seed Math.random
 * (mulberry32), dan pasang perekam pelanggaran CSP. Timer tetap berjalan
 * (setFixedTime, bukan install) supaya loading data tetap selesai.
 */
export async function freeze(page: Page): Promise<void> {
  await page.clock.setFixedTime(DEMO_TIME);
  await page.addInitScript(() => {
    let s = 0x9e3779b9;
    Math.random = (): number => {
      s |= 0;
      s = (s + 0x6d2b79f5) | 0;
      let t = Math.imul(s ^ (s >>> 15), 1 | s);
      t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
  });
  await page.addInitScript(() => {
    const list: string[] = [];
    window.__cspViolations = list;
    document.addEventListener('securitypolicyviolation', (e) => {
      list.push(`${e.violatedDirective} ${e.blockedURI}`);
    });
  });
}

/** Set tema lewat localStorage (dibaca initialTheme saat mount). */
export async function setTheme(page: Page, theme: 'light' | 'dark'): Promise<void> {
  await page.addInitScript((t) => {
    try {
      localStorage.setItem('lab-theme', t);
    } catch {
      /* storage diblokir: biarkan default */
    }
  }, theme);
}

/** Ganti bahasa lewat select di topbar (id default -> en). */
export async function setLang(page: Page, lang: 'id' | 'en'): Promise<void> {
  await page.getByTestId('lang-select').selectOption(lang);
  await expect(page.locator('html')).toHaveAttribute('lang', lang);
}

/** Tunggu font self-hosted + jeda settle singkat agar snapshot deterministik. */
export async function settle(page: Page): Promise<void> {
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(200);
}

/** Buka halaman dengan tema tertentu (dan waktu beku). */
export async function gotoThemed(page: Page, path: string, theme: 'light' | 'dark'): Promise<void> {
  await setTheme(page, theme);
  await page.goto(path);
}

/** Kumpulkan request ke host di luar loopback (U7). */
export function trackOutbound(page: Page): string[] {
  const bad: string[] = [];
  page.on('request', (req) => {
    const host = new URL(req.url()).hostname;
    if (host !== '127.0.0.1' && host !== 'localhost' && host !== '' && host !== '[::1]') {
      bad.push(req.url());
    }
  });
  return bad;
}

/** axe: nol pelanggaran serious/critical (target U8). */
export async function axeNoSerious(page: Page): Promise<void> {
  const results = await new AxeBuilder({ page }).analyze();
  const bad = results.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
  expect(
    bad,
    `axe serious/critical: ${bad.map((v) => `${v.id} (${String(v.nodes.length)})`).join(', ')}`,
  ).toEqual([]);
}

/** Pelanggaran CSP yang terekam sejak halaman dimuat. */
export async function cspViolations(page: Page): Promise<string[]> {
  return page.evaluate(() => window.__cspViolations ?? []);
}

/** Lebar overflow horizontal dokumen (px); > 0 berarti ada scroll horizontal. */
export async function hOverflow(page: Page): Promise<number> {
  return page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  );
}

/** Deskriptor elemen yang sedang fokus (untuk audit keyboard). */
export async function activeDescriptor(page: Page): Promise<string> {
  return page.evaluate(() => {
    const el = document.activeElement;
    if (el === null) return 'null';
    const id = el.id === '' ? '' : `#${el.id}`;
    let label = el.getAttribute('aria-label') ?? '';
    if (label === '') label = (el as HTMLElement).innerText.trim().slice(0, 40);
    if (label === '') label = el.getAttribute('name') ?? '';
    return `${el.tagName.toLowerCase()}${id}:${label}`;
  });
}
