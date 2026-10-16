import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { SettingsPage } from './Settings';
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

function renderSettings() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
        <QueryClientProvider client={qc}>
            <MemoryRouter initialEntries={['/settings']}>
                <SettingsPage />
            </MemoryRouter>
        </QueryClientProvider>,
    );
}

describe('SettingsPage', () => {
    it('menampilkan form pengaturan + diagnostik tanpa docker', async () => {
        renderSettings();
        await screen.findByRole('heading', { name: 'Pengaturan', level: 1 });
        expect(await screen.findByLabelText(/Ambang data-sedikit/)).toBeInTheDocument();
        await screen.findByText('Diagnostik');
        expect((await screen.findAllByText(/Docker tidak tersedia/)).length).toBeGreaterThan(0);
    });

    it('ambang di luar rentang ditolak di klien', async () => {
        renderSettings();
        await screen.findByRole('heading', { name: 'Pengaturan', level: 1 });
        const input = await screen.findByLabelText(/Ambang data-sedikit/);
        fireEvent.change(input, { target: { value: '99' } });
        await waitFor(() => {
            expect(screen.getByText('Ambang harus angka 0..10.')).toBeInTheDocument();
        });
    });
});
