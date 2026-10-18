import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { LearnPage } from './Learn';
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

function renderLearn() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
        <QueryClientProvider client={qc}>
            <MemoryRouter initialEntries={['/learn']}>
                <LearnPage />
            </MemoryRouter>
        </QueryClientProvider>,
    );
}

describe('LearnPage', () => {
    it('menampilkan kurikulum, checklist, dan glosarium', async () => {
        renderLearn();
        await screen.findByText(/day-01 — Hari 1/);
        expect(screen.getByText(/day-02 — Hari 2/)).toBeInTheDocument();
        expect(screen.getAllByText('Checklist').length).toBeGreaterThan(0);
        expect(screen.getByText('Glosarium')).toBeInTheDocument();
    });

    it('checklist bisa dicentang dan progres tersimpan', async () => {
        renderLearn();
        await screen.findByText(/day-01 — Hari 1/);
        const checkbox = screen.getByRole('checkbox', { name: /Baca README/ });
        expect(checkbox).not.toBeChecked();
        fireEvent.change(checkbox, { target: { checked: true } });
        await waitFor(() => {
            expect(checkbox).toBeChecked();
        });
    });

    // U8 (WCAG 1.3.1 / 2.4.6): HTML modul kurikulum adalah dokumen markdown
    // mandiri berjudul h1; saat disematkan judulnya harus diturunkan sehingga
    // halaman tetap punya TEPAT SATU h1.
    it('menurunkan heading modul (h1 -> h2) agar satu h1 per halaman', async () => {
        renderLearn();
        await screen.findByText(/day-01 — Hari 1/);
        const h1s = screen.getAllByRole('heading', { level: 1 });
        expect(h1s).toHaveLength(1);
        expect(h1s[0]).toHaveTextContent('Belajar');
        const h2s = screen.getAllByRole('heading', { level: 2 });
        expect(h2s.some((h) => h.textContent === 'Hari 1')).toBe(true);
    });
});
