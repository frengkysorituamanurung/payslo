import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// U0: kontras token dihitung dari CSS (satu-satunya sumber), kedua tema.
// Syarat UI-SPEC: teks >= 4.5:1, komponen UI >= 3:1.

function luminance(hex: string): number {
  const c = hex.replace('#', '');
  const rgb = [0, 2, 4].map((i) => {
    const v = parseInt(c.slice(i, i + 2), 16) / 255;
    return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
  });
  return 0.2126 * (rgb[0] as number) + 0.7152 * (rgb[1] as number) + 0.0722 * (rgb[2] as number);
}

function ratio(a: string, b: string): number {
  const sorted = [luminance(a), luminance(b)].sort((x, y) => y - x);
  const l1 = sorted[0] ?? 0;
  const l2 = sorted[1] ?? 0;
  return (l1 + 0.05) / (l2 + 0.05);
}

function themeVars(css: string, theme: 'light' | 'dark'): Record<string, string> {
  const block =
    theme === 'light'
      ? (css.match(/:root,\s*\[data-theme='light'\]\s*\{([^}]*)\}/)?.[1] ?? '')
      : (css.match(/\[data-theme='dark'\]\s*\{([^}]*)\}/)?.[1] ?? '');
  const vars: Record<string, string> = {};
  for (const m of block.matchAll(/--([\w-]+):\s*(#[0-9a-fA-F]{6})/g)) {
    vars[m[1] as string] = m[2] as string;
  }
  return vars;
}

const here = dirname(fileURLToPath(import.meta.url));
const css = readFileSync(resolve(here, 'tokens.css'), 'utf8');

const textPairs: Array<[string, string]> = [
  ['text', 'surface'],
  ['text-muted', 'surface'],
  ['text', 'surface-raised'],
  ['accent-text', 'accent'],
  ['ok', 'ok-bg'],
  ['warn', 'warn-bg'],
  ['crit', 'crit-bg'],
  ['info', 'info-bg'],
  ['neutral', 'neutral-bg'],
];

describe.each(['light', 'dark'] as const)('kontras token tema %s', (theme) => {
  const vars = themeVars(css, theme);
  it.each(textPairs)('%s di atas %s >= 4.5:1', (fg, bg) => {
    const f = vars[fg];
    const b = vars[bg];
    expect(f, `token --${fg} hilang`).toBeDefined();
    expect(b, `token --${bg} hilang`).toBeDefined();
    expect(ratio(f as string, b as string)).toBeGreaterThanOrEqual(4.5);
  });
  it('border terlihat di atas surface (>= 1.5:1, bukan teks)', () => {
    expect(ratio(vars['border'] as string, vars['surface'] as string)).toBeGreaterThanOrEqual(1.5);
  });
});
