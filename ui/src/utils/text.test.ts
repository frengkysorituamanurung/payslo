import { describe, expect, it } from 'vitest';
import { normalizeEditorText } from './text';

const NBSP = '\u00a0';

describe('normalizeEditorText', () => {
  it('mengganti NBSP (U+00A0) dengan spasi biasa', () => {
    expect(normalizeEditorText(`a${NBSP} b`)).toBe('a  b');
    expect(normalizeEditorText(`${NBSP} type: x`)).toBe('  type: x');
    expect(normalizeEditorText('tanpa nbsp')).toBe('tanpa nbsp');
  });
});
