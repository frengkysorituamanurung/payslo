import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { StatusBadge } from './StatusBadge';

describe('StatusBadge', () => {
    it('menampilkan ikon + teks (bukan warna saja)', () => {
        render(<StatusBadge state="up" />);
        const badge = screen.getByRole('status');
        expect(badge).toHaveTextContent('✓');
        expect(badge).toHaveTextContent('Up');
    });
    it('label kustom tetap ber-icon', () => {
        render(<StatusBadge state="down" label="Mati total" />);
        expect(screen.getByRole('status')).toHaveTextContent('✕ Mati total');
    });
});
