import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../test/setup';
import { AppShell } from './AppShell';
import { demoStack } from '../mocks/handlers';

function renderShell(path = '/') {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const router = createMemoryRouter([{ path: '/', element: <AppShell /> }], {
        initialEntries: [path],
    });
    return render(
        <QueryClientProvider client={qc}>
            <RouterProvider router={router} />
        </QueryClientProvider>,
    );
}

describe('AppShell', () => {
    it('navigasi 11 rute + topbar status', async () => {
        renderShell();
        expect(await screen.findByText('Mission Control')).toBeInTheDocument();
        expect(screen.getByText('Peta Arsitektur')).toBeInTheDocument();
        expect(screen.getByText('Pengaturan')).toBeInTheDocument();
        expect(await screen.findByText('Stack up')).toBeInTheDocument();
    });
    it('onboarding lab-belum-up dengan CopyCommand make up-lite', async () => {
        server.use(
            http.get('/api/v1/health/stack', () =>
                HttpResponse.json({ ...demoStack, labUp: false }),
            ),
        );
        const user = userEvent.setup();
        Object.defineProperty(navigator, 'clipboard', {
            value: { writeText: () => Promise.resolve() },
            configurable: true,
        });
        renderShell();
        expect(await screen.findByText('Lab belum up')).toBeInTheDocument();
        await user.click(screen.getByRole('button', { name: 'Salin perintah' }));
        expect(await screen.findByRole('button', { name: 'Disalin' })).toBeInTheDocument();
    });
});
