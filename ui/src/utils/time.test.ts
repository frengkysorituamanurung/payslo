import { describe, expect, it } from 'vitest';
import { formatLocal, formatRelative } from './time';

describe('formatRelative', () => {
  const now = Date.parse('2026-10-16T07:03:00Z');
  it('hitungan detik/menit/jam/hari via Intl (en deterministik)', () => {
    expect(formatRelative('2026-10-16T07:02:40Z', 'en', now)).toBe('20 seconds ago');
    expect(formatRelative('2026-10-16T07:00:00Z', 'en', now)).toBe('3 minutes ago');
    expect(formatRelative('2026-10-16T05:00:00Z', 'en', now)).toBe('2 hours ago');
    expect(formatRelative('2026-10-14T07:00:00Z', 'en', now)).toBe('2 days ago');
  });

  it('mendukung masa depan ("in ...")', () => {
    expect(formatRelative('2026-10-16T07:05:00Z', 'en', now)).toBe('in 2 minutes');
  });

  it('mengembalikan input apa adanya bila bukan tanggal valid', () => {
    expect(formatRelative('bukan-tanggal', 'en', now)).toBe('bukan-tanggal');
  });
});

describe('formatLocal', () => {
  it('mengembalikan input apa adanya bila invalid', () => {
    expect(formatLocal('bukan-tanggal', 'id')).toBe('bukan-tanggal');
  });
});
