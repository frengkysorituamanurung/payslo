import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../test/setup';
import { demoSummary } from '../mocks/handlers';
import { MissionControl } from './MissionControl';

function renderMission() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
        <QueryClientProvider client={qc}>
            <MissionControl />
        </QueryClientProvider>,
    );
}

describe('MissionControl', () => {
    it('panel HTTP vs Payment berdampingan + penjelasan U2', async () => {
        renderMission();
        expect(
            await screen.findByRole('heading', { name: 'HTTP success (Envoy)' }),
        ).toBeInTheDocument();
        expect(
            await screen.findByRole('heading', { name: 'Payment success (DB)' }),
        ).toBeInTheDocument();
        expect(screen.getByText(/Mengapa bisa berbeda/)).toBeInTheDocument();
        expect(await screen.findByText('98.5%')).toBeInTheDocument();
    });
    it('ubin per provider + ambang data-sedikit', async () => {
        renderMission();
        expect(await screen.findByText('BCA')).toBeInTheDocument();
        expect(await screen.findByText('99.0%')).toBeInTheDocument();
    });
    it('isolasi error: sumber down hanya di panel metrik', async () => {
        server.use(
            http.get('*/api/v1/metrics/summary', () =>
                HttpResponse.json({
                    ...demoSummary,
                    metrics: {
                        ...demoSummary.metrics,
                        payment_success_ratio: {
                            status: 'source_down',
                            value: null,
                            sampleRate: null,
                            lastDataAt: null,
                        },
                    },
                }),
            ),
        );
        renderMission();
        // Tepat satu panel error (rasio sukses pembayaran); panel HTTP tetap angka.
        expect(await screen.findByText('99.9%')).toBeInTheDocument();
        expect(screen.getAllByText(/Gagal memuat/)).toHaveLength(1);
        // Navigasi/header tetap hidup (diuji di AppShell); halaman tidak blank.
        expect(screen.getByText('Mission Control')).toBeInTheDocument();
    });
});
