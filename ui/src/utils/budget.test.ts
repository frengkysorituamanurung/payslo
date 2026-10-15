import { describe, expect, it } from 'vitest';
import { errorBudgetMinutes, errorBudgetSecs, formatMinutes, parseDurationSecs } from './budget';

describe('parseDurationSecs', () => {
  it('menerima s/m/h/d/w dan menolak nol/buruk (cermin payslo-model)', () => {
    expect(parseDurationSecs('5s')).toBe(5);
    expect(parseDurationSecs('15m')).toBe(900);
    expect(parseDurationSecs('1h')).toBe(3600);
    expect(parseDurationSecs('30d')).toBe(2_592_000);
    expect(parseDurationSecs('2w')).toBe(1_209_600);
    expect(parseDurationSecs('0m')).toBeNull();
    expect(parseDurationSecs('m')).toBeNull();
    expect(parseDurationSecs('10x')).toBeNull();
    expect(parseDurationSecs('')).toBeNull();
    expect(parseDurationSecs('1.5h')).toBeNull();
  });
});

describe('errorBudget (acceptance U3)', () => {
  const month = 30 * 86400;

  it('99.9%/30 hari ≈ 43,2 menit', () => {
    expect(errorBudgetSecs(0.999, month)).toBeCloseTo(2592, 9);
    expect(errorBudgetMinutes(0.999, month)).toBeCloseTo(43.2, 9);
  });

  it('99.99%/30 hari ≈ 4,32 menit', () => {
    expect(errorBudgetSecs(0.9999, month)).toBeCloseTo(259.2, 9);
    expect(errorBudgetMinutes(0.9999, month)).toBeCloseTo(4.32, 9);
  });

  it('kasus lab: 99.5%/1h = 18 detik; 99%/1h = 36 detik', () => {
    expect(errorBudgetSecs(0.995, 3600)).toBeCloseTo(18, 9);
    expect(errorBudgetSecs(0.99, 3600)).toBeCloseTo(36, 9);
  });

  it('format id-ID memakai koma desimal', () => {
    expect(formatMinutes(43.2, 'id')).toBe('43,2');
    expect(formatMinutes(4.32, 'en')).toBe('4.32');
  });
});
