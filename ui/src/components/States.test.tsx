import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { EmptyState, ErrorState, StaleBanner } from './States';

describe('states', () => {
    it('EmptyState menampilkan langkah berikutnya', () => {
        render(<EmptyState next="Jalankan loadgen dulu" />);
        expect(screen.getByRole('status')).toHaveTextContent('Jalankan loadgen dulu');
    });
    it('ErrorState memanggil retry', async () => {
        const user = userEvent.setup();
        const onRetry = vi.fn();
        render(<ErrorState onRetry={onRetry} />);
        await user.click(screen.getByRole('button', { name: 'Coba lagi' }));
        expect(onRetry).toHaveBeenCalledOnce();
    });
    it('StaleBanner memuat waktu data terakhir', () => {
        render(<StaleBanner lastData="07:00 UTC" />);
        expect(screen.getByRole('status')).toHaveTextContent('07:00 UTC');
    });
});
