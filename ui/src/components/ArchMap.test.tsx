import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it } from 'vitest';
import { ArchMap } from './ArchMap';

function renderMap() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
        <QueryClientProvider client={qc}>
            <ArchMap />
        </QueryClientProvider>,
    );
}

describe('ArchMap', () => {
    it('SVG + daftar setara + panel samping via keyboard', async () => {
        const user = userEvent.setup();
        renderMap();
        expect(await screen.findByRole('group', { name: 'Peta Arsitektur' })).toBeInTheDocument();

        // Pilih node via keyboard (Tab ke toggle, Tab ke node pertama, Enter).
        await user.tab();
        await user.tab();
        await user.keyboard('{Enter}');
        expect(
            await screen.findByRole('complementary', { name: 'Detail node' }),
        ).toBeInTheDocument();

        // Esc menutup panel.
        await user.keyboard('{Escape}');
        expect(screen.queryByRole('complementary')).not.toBeInTheDocument();

        // Tampilan daftar setara isinya.
        await user.click(screen.getByRole('button', { name: 'Tampilan daftar' }));
        const items = await screen.findAllByRole('listitem');
        expect(items.length).toBe(10);
    });
});
