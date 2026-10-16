import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { MentorPage } from './Mentor';
import { handlers } from '../mocks/handlers';
import { setupServer } from 'msw/node';
import { HttpResponse, http } from 'msw';

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

function renderMentor() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
        <QueryClientProvider client={qc}>
            <MemoryRouter initialEntries={['/mentor']}>
                <MentorPage />
            </MemoryRouter>
        </QueryClientProvider>,
    );
}

describe('MentorPage', () => {
    it('menampilkan 403 untuk mode trainee', () => {
        renderMentor();
        expect(screen.getByText('403 — Mode mentor diperlukan.')).toBeInTheDocument();
    });

    it('menampilkan timeline, snapshot, sesi, ekspor untuk mode mentor', async () => {
        server.use(
            http.get('*/api/v1/session', () =>
                HttpResponse.json({
                    mode: 'mentor',
                    traineeName: 'Sinta',
                    scenario: null,
                    labUp: true,
                    checkedAt: '2026-10-16T07:00:00Z',
                }),
            ),
        );
        renderMentor();
        await screen.findByText('Timeline aksi trainee');
        expect(screen.getByText('Injeksi gangguan')).toBeInTheDocument();
        expect(screen.getByText('Snapshot')).toBeInTheDocument();
        expect(screen.getByText('Sesi trainee')).toBeInTheDocument();
        expect(screen.getByText('Ekspor laporan')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Keluar mode mentor' })).toBeInTheDocument();
    });
});
