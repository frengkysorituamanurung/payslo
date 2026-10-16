import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { ObservePage } from './Observe';
import { handlers } from '../mocks/handlers';
import { setupServer } from 'msw/node';

const server = setupServer(...handlers);

beforeAll(() => {
    server.listen();
});
afterEach(() => {
    server.resetHandlers();
});
afterAll(() => {
    server.close();
});

function renderObserve() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
        <QueryClientProvider client={qc}>
            <MemoryRouter initialEntries={['/observe']}>
                <ObservePage />
            </MemoryRouter>
        </QueryClientProvider>,
    );
}

describe('ObservePage', () => {
    it('menampilkan banner obs-full mati + tab + anotasi', async () => {
        renderObserve();
        await screen.findByText(/Profil obs-full tidak aktif/);
        expect(screen.getByRole('tab', { name: 'Grafana' })).toBeInTheDocument();
        expect(screen.getByRole('tab', { name: 'Prometheus' })).toBeInTheDocument();
        await screen.findByText('gateway deploy good via lab');
    });

    it('tab prometheus menampilkan alert + rules', async () => {
        const user = (await import('@testing-library/user-event')).default;
        renderObserve();
        await screen.findByRole('tab', { name: 'Prometheus' });
        await user.click(screen.getByRole('tab', { name: 'Prometheus' }));
        await screen.findByText('PaySLOBurnRateHigh');
        expect(screen.getByText('payslo-payment-success-rate')).toBeInTheDocument();
    });

    it('tab ntfy menampilkan riwayat', async () => {
        const user = (await import('@testing-library/user-event')).default;
        renderObserve();
        await screen.findByRole('tab', { name: 'ntfy' });
        await user.click(screen.getByRole('tab', { name: 'ntfy' }));
        await screen.findByText(/incident opened/);
    });
});
