import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { MetricTile } from './MetricTile';

describe('MetricTile', () => {
    it('menampilkan 0 apa adanya (jujur, bukan empty)', () => {
        render(<MetricTile title="T" value="0%" dataState="ok" />);
        expect(screen.getByLabelText('T')).toHaveTextContent('0%');
    });
    it('total nol data menampilkan empty, bukan 0%', () => {
        render(<MetricTile title="T" value="0%" dataState="none" />);
        expect(screen.getByLabelText('T')).toHaveTextContent('Belum ada data');
    });
    it('data usang ditandai stale', () => {
        render(<MetricTile title="T" value="99%" dataState="stale" />);
        expect(screen.getByLabelText('T')).toHaveTextContent('Usang');
    });
});
