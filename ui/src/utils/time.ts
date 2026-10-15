// U1: waktu ditampilkan di zona lokal, tooltip ISO-8601 UTC, relatif.
// Semua format memakai Intl (locale-aware) supaya tidak ada string hardcoded.

/** Waktu lokal singkat, mis. "16 Okt 2026, 14.00.00". Fallback ke ISO bila invalid. */
export function formatLocal(iso: string, locale: string): string {
  const ms = Date.parse(iso);
  if (Number.isNaN(ms)) return iso;
  return new Date(ms).toLocaleString(locale, { dateStyle: 'medium', timeStyle: 'medium' });
}

/**
 * Waktu relatif ("3 menit yang lalu"). `now` disuntik agar test deterministik.
 * Di luar jam/hari tetap memakai RelativeTimeFormat (tidak ada batas buatan).
 */
export function formatRelative(iso: string, locale: string, now: number = Date.now()): string {
  const ms = Date.parse(iso);
  if (Number.isNaN(ms)) return iso;
  const diffSec = Math.round((ms - now) / 1000);
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
  const abs = Math.abs(diffSec);
  if (abs < 60) return rtf.format(diffSec, 'second');
  if (abs < 3600) return rtf.format(Math.round(diffSec / 60), 'minute');
  if (abs < 86_400) return rtf.format(Math.round(diffSec / 3600), 'hour');
  return rtf.format(Math.round(diffSec / 86_400), 'day');
}
