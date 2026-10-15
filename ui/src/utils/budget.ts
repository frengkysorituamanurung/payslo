// Kalkulator error budget (U3): jatah error = (1 - objective) * window.
// Float untuk TAMPILAN (bukan uang; U6 tak berlaku). Menit dibulatkan 2
// desimal agar 99.9%/30d tampil "43,2" bukan "43.199999...".

/** Durasi `<int><s|m|h|d|w>` ke detik; null bila buruk (cermin payslo-model). */
export function parseDurationSecs(raw: string): number | null {
  const m = /^([0-9]+)([smhdw])$/.exec(raw);
  if (m === null) return null;
  const digits = m[1] ?? '';
  const unitKey = m[2] ?? '';
  const n = Number(digits);
  if (!Number.isSafeInteger(n) || n === 0) return null;
  const unit: Record<string, number> = { s: 1, m: 60, h: 3600, d: 86400, w: 604800 };
  const per = unit[unitKey];
  if (per === undefined) return null;
  return n * per;
}

/** Jatah error dalam detik. */
export function errorBudgetSecs(objective: number, windowSecs: number): number {
  return (1 - objective) * windowSecs;
}

/** Jatah error dalam menit, dibulatkan 2 desimal untuk tampilan. */
export function errorBudgetMinutes(objective: number, windowSecs: number): number {
  return Math.round((errorBudgetSecs(objective, windowSecs) / 60) * 100) / 100;
}

/** Format menit ala id-ID (koma desimal), mis. 43,2. */
export function formatMinutes(minutes: number, locale: string): string {
  return new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(minutes);
}
