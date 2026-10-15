import { z } from 'zod';

// U6/P7: uang SELALU integer IDR. Tidak ada float di jalur ini.
// Intl.NumberFormat('id-ID') dengan maximumFractionDigits 0.

/** Batas atas = 1 triliun IDR; jauh di bawah Number.MAX_SAFE_INTEGER. */
export const MAX_AMOUNT = 1_000_000_000_000;

const idr = new Intl.NumberFormat('id-ID', {
  style: 'currency',
  currency: 'IDR',
  maximumFractionDigits: 0,
  minimumFractionDigits: 0,
});

/**
 * Format integer IDR. Melempar bila bukan integer aman (tidak pernah
 * diam-diam membulatkan pecahan — itu bug uang).
 */
export function formatIDR(value: number): string {
  if (!Number.isSafeInteger(value)) {
    throw new TypeError('formatIDR: nilai harus integer aman (bukan pecahan/NaN)');
  }
  return idr.format(value);
}

/** Format tanpa mata uang (kolom tabel). Tetap menolak pecahan. */
export function formatIDRPlain(value: number): string {
  if (!Number.isSafeInteger(value)) {
    throw new TypeError('formatIDRPlain: nilai harus integer aman');
  }
  return new Intl.NumberFormat('id-ID', { maximumFractionDigits: 0 }).format(value);
}

// Kode alasan (dipetakan ke i18n di komponen; BUKAN string tampilan hardcoded).
export type AmountIssue = 'empty' | 'notInteger' | 'tooSmall' | 'tooLarge' | 'notSafe';

/**
 * Validasi teks amount: hanya digit (integer positif), dalam rentang aman.
 * Menolak "1.5", "-1", "1e3", "+5", "1 000", dan nilai di atas batas aman.
 */
export const amountSchema = z
  .string()
  .trim()
  .superRefine((raw, ctx) => {
    const issue = (reason: AmountIssue): void => {
      ctx.addIssue({ code: 'custom', message: reason });
    };
    if (raw === '') {
      issue('empty');
      return;
    }
    if (!/^\d+$/.test(raw)) {
      issue('notInteger');
      return;
    }
    const n = Number(raw);
    if (!Number.isSafeInteger(n)) {
      issue('notSafe');
      return;
    }
    if (n < 1) {
      issue('tooSmall');
      return;
    }
    if (n > MAX_AMOUNT) {
      issue('tooLarge');
    }
  });

export const channelSchema = z.enum(['VA', 'EWALLET', 'QRIS'], { message: 'channelRequired' });

export const paymentFormSchema = z
  .object({
    merchantId: z
      .string()
      .trim()
      .min(1, { message: 'merchantRequired' })
      .max(128, { message: 'idTooLong' }),
    orderId: z
      .string()
      .trim()
      .min(1, { message: 'orderRequired' })
      .max(128, { message: 'idTooLong' }),
    amount: amountSchema,
    channel: channelSchema,
  })
  .transform((v) => ({
    merchantId: v.merchantId,
    orderId: v.orderId,
    amount: Number(v.amount),
    channel: v.channel,
  }));

export type PaymentForm = z.input<typeof paymentFormSchema>;
export type PaymentFormParsed = z.output<typeof paymentFormSchema>;

/** order_id unik otomatis (bisa diedit). Deterministik dari waktu + acak kecil. */
export function uniqueOrderId(now: number = Date.now(), rand: number = Math.random()): string {
  const suffix = Math.floor(rand * 1_000_000)
    .toString()
    .padStart(6, '0');
  return `order-${String(now)}-${suffix}`;
}
