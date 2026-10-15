import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it } from 'vitest';
import { PaymentsPage } from './Payments';

function renderPage() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
        <QueryClientProvider client={qc}>
            <PaymentsPage />
        </QueryClientProvider>,
    );
}

describe('PaymentsPage', () => {
    it('tabel memuat transaksi + menandai callback duplikat secara netral', async () => {
        renderPage();
        expect(await screen.findByText('demo-order-002')).toBeInTheDocument();
        expect(screen.getByText('demo-order-004')).toBeInTheDocument();
        // Netral: "callback diterima 2x" tanpa menyebut penyebab (U3).
        expect(screen.getByText('callback diterima 2x')).toBeInTheDocument();
    });

    it('detail menampilkan timeline + status bank + tautan obs-inaktif', async () => {
        const user = userEvent.setup();
        renderPage();
        const row = (await screen.findByText('demo-order-004')).closest('tr');
        if (row === null) throw new Error('baris tidak ditemukan');
        await user.click(within(row).getByRole('button', { name: 'Lihat' }));

        expect(
            await screen.findByRole('heading', { name: 'Detail transaksi' }),
        ).toBeInTheDocument();
        // Dua callback + dua webhook pada fixture duplikat.
        expect(screen.getAllByText('Callback diterima')).toHaveLength(2);
        expect(screen.getAllByText('Webhook dikirim')).toHaveLength(2);
        expect(screen.getByText('P-BCA-0004')).toBeInTheDocument();
        expect(screen.getByText(/obs-full belum aktif/)).toBeInTheDocument();

        await user.click(screen.getByRole('button', { name: 'Cek status ke bank' }));
        expect(await screen.findByText(/bank bca berkata: SUCCESS/)).toBeInTheDocument();
    });

    it('membuat pembayaran lewat form dan menampilkannya di tabel', async () => {
        const user = userEvent.setup();
        renderPage();
        await screen.findByText('demo-order-002');

        const orderInput = screen.getByLabelText<HTMLInputElement>('Order ID');
        const orderId = orderInput.value;
        await user.type(screen.getByLabelText('Nominal (IDR)'), '250000');
        await user.click(screen.getByRole('button', { name: 'Kirim' }));

        expect(await screen.findByText(/Pembayaran dibuat/)).toBeInTheDocument();
        expect(await screen.findByText(orderId)).toBeInTheDocument();
    });

    it('menolak nominal tidak valid dengan pesan jelas (tanpa memanggil API)', async () => {
        const user = userEvent.setup();
        renderPage();
        const amount = screen.getByLabelText('Nominal (IDR)');

        await user.type(amount, '-5');
        await user.click(screen.getByRole('button', { name: 'Kirim' }));
        expect(
            await screen.findByText('Nominal harus bilangan bulat (tanpa titik/koma/eksponen).'),
        ).toBeInTheDocument();

        await user.clear(amount);
        await user.type(amount, '0');
        await user.click(screen.getByRole('button', { name: 'Kirim' }));
        expect(await screen.findByText('Nominal minimal 1 IDR.')).toBeInTheDocument();

        await user.clear(amount);
        await user.type(amount, '1000000000001');
        await user.click(screen.getByRole('button', { name: 'Kirim' }));
        expect(await screen.findByText(/melebihi batas/)).toBeInTheDocument();
    });
});
