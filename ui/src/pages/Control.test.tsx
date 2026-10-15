import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../test/setup';
import type { RoutingConfig, TrafficState } from '../api/client';
import { ControlPage } from './Control';

function renderPage() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
        <QueryClientProvider client={qc}>
            <ControlPage />
        </QueryClientProvider>,
    );
}

async function confirmDialog(
    user: ReturnType<typeof userEvent.setup>,
    target: string,
): Promise<void> {
    const ack = await screen.findByLabelText(target);
    await user.type(ack, target);
    await user.click(screen.getByRole('button', { name: 'Konfirmasi' }));
}

describe('ControlPage', () => {
    it('menampilkan traffic, routing, release, dan audit', async () => {
        renderPage();
        expect(await screen.findByText('Laju saat ini:')).toBeInTheDocument();
        expect(await screen.findByText('2')).toBeInTheDocument();
        expect(screen.getByRole('checkbox', { name: 'bca Aktif' })).toBeChecked();
        expect(screen.getByText('routing.set')).toBeInTheDocument();
        expect(screen.getByText('Tidak diketahui (endpoint baca belum ada)')).toBeInTheDocument();
    });

    it('menonaktifkan provider lewat UI mengirim routing yang berubah (setelah konfirmasi)', async () => {
        const user = userEvent.setup();
        const captured: RoutingConfig[] = [];
        server.use(
            http.put('*/api/v1/routing', async ({ request }) => {
                captured.push((await request.json()) as RoutingConfig);
                return HttpResponse.json(captured[0]);
            }),
        );
        renderPage();

        const bca = await screen.findByRole('checkbox', { name: 'bca Aktif' });
        await user.click(bca);
        await user.click(screen.getByRole('button', { name: 'Simpan routing' }));
        await confirmDialog(user, 'routing');

        await waitFor(() => {
            expect(captured).toHaveLength(1);
        });
        expect(captured[0]?.bca.enabled).toBe(false);
        expect(captured[0]?.mandiri.enabled).toBe(true);
    });

    it('mengubah rilis ke bad lewat ConfirmDialog', async () => {
        const user = userEvent.setup();
        const captured: { version: string }[] = [];
        server.use(
            http.put('*/api/v1/release', async ({ request }) => {
                captured.push((await request.json()) as { version: string });
                return HttpResponse.json(captured[0]);
            }),
        );
        renderPage();

        await user.click(await screen.findByRole('button', { name: 'bad' }));
        await confirmDialog(user, 'release bad');

        await waitFor(() => {
            expect(captured).toHaveLength(1);
        });
        expect(captured[0]?.version).toBe('bad');
    });

    it('mengubah laju traffic lewat ConfirmDialog (1..100)', async () => {
        const user = userEvent.setup();
        const captured: TrafficState[] = [];
        server.use(
            http.put('*/api/v1/traffic', async ({ request }) => {
                captured.push((await request.json()) as TrafficState);
                return HttpResponse.json(captured[0]);
            }),
        );
        renderPage();

        const rate = await screen.findByLabelText('Laju (req/s, 1..100)');
        await user.clear(rate);
        await user.type(rate, '5');
        await user.click(screen.getByRole('button', { name: 'Terapkan' }));
        await confirmDialog(user, 'traffic');

        await waitFor(() => {
            expect(captured).toHaveLength(1);
        });
        expect(captured[0]?.ratePerSec).toBe(5);
    });

    it('menolak laju di luar 1..100 tanpa membuka konfirmasi', async () => {
        const user = userEvent.setup();
        renderPage();
        const rate = await screen.findByLabelText('Laju (req/s, 1..100)');
        await user.clear(rate);
        await user.type(rate, '0');
        await user.click(screen.getByRole('button', { name: 'Terapkan' }));
        expect(await screen.findByText('Laju harus integer 1..100.')).toBeInTheDocument();
        expect(screen.queryByLabelText('traffic')).not.toBeInTheDocument();
    });
});
