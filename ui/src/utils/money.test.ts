import { describe, expect, it } from 'vitest';
import {
  MAX_AMOUNT,
  amountSchema,
  formatIDR,
  formatIDRPlain,
  paymentFormSchema,
  uniqueOrderId,
} from './money';

describe('formatIDR', () => {
  it('memformat integer IDR dengan Intl (pemisah ribuan id-ID)', () => {
    expect(formatIDR(150_000)).toMatch(/150\.000/);
    expect(formatIDRPlain(1_250_000)).toBe('1.250.000');
    expect(formatIDRPlain(0)).toBe('0');
  });

  it('MELEMPAR pada pecahan/NaN (uang tidak pernah dibulatkan diam-diam)', () => {
    expect(() => formatIDR(1.5)).toThrow(TypeError);
    expect(() => formatIDR(Number.NaN)).toThrow(TypeError);
    expect(() => formatIDRPlain(0.1)).toThrow(TypeError);
  });
});

describe('amountSchema (U6/P7: integer IDR)', () => {
  it('menerima integer positif dalam rentang', () => {
    expect(amountSchema.safeParse('1').success).toBe(true);
    expect(amountSchema.safeParse('150000').success).toBe(true);
    expect(amountSchema.safeParse(String(MAX_AMOUNT)).success).toBe(true);
  });

  it('menolak kosong, pecahan, tanda, eksponen, spasi, nol, dan terlalu besar', () => {
    const code = (v: string): string | undefined => {
      const r = amountSchema.safeParse(v);
      return r.success ? undefined : r.error.issues[0]?.message;
    };
    expect(code('')).toBe('empty');
    expect(code('   ')).toBe('empty');
    expect(code('1.5')).toBe('notInteger');
    expect(code('-1')).toBe('notInteger');
    expect(code('+5')).toBe('notInteger');
    expect(code('1e3')).toBe('notInteger');
    expect(code('1 000')).toBe('notInteger');
    expect(code('0')).toBe('tooSmall');
    expect(code(String(MAX_AMOUNT + 1))).toBe('tooLarge');
    expect(code('999999999999999999999')).toBe('notSafe');
  });
});

describe('paymentFormSchema', () => {
  it('mentransformasi amount menjadi number (bukan string)', () => {
    const r = paymentFormSchema.safeParse({
      merchantId: 'merchant-demo',
      orderId: 'order-1',
      amount: '150000',
      channel: 'VA',
    });
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.amount).toBe(150_000);
      expect(Number.isInteger(r.data.amount)).toBe(true);
      expect(r.data.channel).toBe('VA');
    }
  });

  it('menolak channel di luar enum dan id kosong', () => {
    expect(
      paymentFormSchema.safeParse({ merchantId: '', orderId: 'x', amount: '1', channel: 'VA' })
        .success,
    ).toBe(false);
    expect(
      paymentFormSchema.safeParse({ merchantId: 'm', orderId: '', amount: '1', channel: 'VA' })
        .success,
    ).toBe(false);
  });
});

describe('uniqueOrderId', () => {
  it('deterministik diberi now+rand dan berbentuk order-<ts>-<6 digit>', () => {
    expect(uniqueOrderId(1_700_000_000_000, 0.123456)).toBe('order-1700000000000-123456');
    expect(uniqueOrderId(1, 0)).toMatch(/^order-1-000000$/);
  });
});
