// Format tampilan metric (U1). U6: uang integer via Intl di fase payments.
export function fmtPct(v: number): string {
  return `${(v * 100).toFixed(1)}%`;
}

export function fmtRps(v: number): string {
  return `${v.toFixed(2)}/s`;
}

export function fmtMs(v: number): string {
  return `${String(Math.round(v))} ms`;
}

// Ambang data-sedikit (UI-SPEC default 0.2/s; konfigurasi menyusul di /settings).
export const LOW_SAMPLE_RPS = 0.2;
