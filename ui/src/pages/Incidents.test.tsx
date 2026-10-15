import { fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it } from 'vitest';
import { IncidentsPage } from './Incidents';

function renderPage() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
        <QueryClientProvider client={qc}>
            <IncidentsPage />
        </QueryClientProvider>,
    );
}

async function openDetail(user: ReturnType<typeof userEvent.setup>): Promise<void> {
    renderPage();
    await user.click(
        await screen.findByRole('button', { name: /payment-success-rate on gateway-api/ }),
    );
    await screen.findByText('Insiden: payment-success-rate on gateway-api (page)');
}

describe('IncidentsPage', () => {
    it('daftar + timeline + alert terkorelasi', async () => {
        const user = userEvent.setup();
        await openDetail(user);
        expect(screen.getByText('Dibuka')).toBeInTheDocument();
        expect(screen.getByText('incident opened by page alert')).toBeInTheDocument();
        expect(screen.getByText('PaySLOBurnRateHigh', { exact: false })).toBeInTheDocument();
    });

    it('XSS: payload <script>/<img onerror> tampil sebagai teks, tak dieksekusi', async () => {
        const user = userEvent.setup();
        await openDetail(user);
        // Teks literal terlihat (React escape) ...
        expect(screen.getByText('<script>alert(1)</script>')).toBeInTheDocument();
        expect(screen.getByText('<img src=x onerror=alert(2)>')).toBeInTheDocument();
        // ... dan tidak ada elemen script/img sungguhan yang tercipta.
        expect(document.querySelector('script[src]')).toBeNull();
        const imgs = document.querySelectorAll('img');
        expect(imgs.length).toBe(0);
        expect(document.body.innerHTML.includes('<script>alert(1)</script>')).toBe(false);
    });

    it('tak ada jalan pintas: remediation tanpa tombol jalan; diagnostik ada', async () => {
        const user = userEvent.setup();
        await openDetail(user);
        const remediation = screen.getByRole('region', { name: 'nonaktifkan provider bca' });
        expect(
            within(remediation).queryByRole('button', { name: /Jalankan|Run|Eksekusi|Execute/ }),
        ).toBeNull();
        expect(
            within(remediation).getByRole('button', { name: 'Minta persetujuan' }),
        ).toBeInTheDocument();
        const diag = screen.getByRole('region', { name: 'success-rate per provider' });
        expect(
            within(diag).getByRole('button', { name: 'Jalankan diagnostik' }),
        ).toBeInTheDocument();
    });

    it('diagnostik menampilkan output saat dijalankan', async () => {
        const user = userEvent.setup();
        await openDetail(user);
        const diag = screen.getByRole('region', { name: 'profil bank-bca' });
        await user.click(within(diag).getByRole('button', { name: 'Jalankan diagnostik' }));
        expect(await within(diag).findByText(/demo output/)).toBeInTheDocument();
    });

    it('alur approval: minta -> setujui (ConfirmDialog) -> tereksekusi di timeline', async () => {
        const user = userEvent.setup();
        await openDetail(user);
        // Nama approver dari pengaturan.
        await user.clear(screen.getByLabelText('Nama trainee'));
        await user.type(screen.getByLabelText('Nama trainee'), 'sinta');
        await user.click(screen.getByRole('button', { name: 'Simpan nama' }));
        expect(await screen.findByText('Nama tersimpan.')).toBeInTheDocument();

        const remediation = screen.getByRole('region', { name: 'nonaktifkan provider bca' });
        await user.click(within(remediation).getByRole('button', { name: 'Minta persetujuan' }));
        expect(await within(remediation).findByText('Menunggu keputusan')).toBeInTheDocument();

        await user.click(within(remediation).getByRole('button', { name: 'Approve…' }));
        const ack = await screen.findByLabelText('disable provider bca');
        await user.type(ack, 'disable provider bca');
        await user.click(screen.getByRole('button', { name: 'Konfirmasi' }));
        expect(await within(remediation).findByText(/approved oleh sinta/)).toBeInTheDocument();
        // Eksekusi tercatat di timeline.
        expect(await screen.findByText(/remediation executed by sinta/)).toBeInTheDocument();
    }, 30000);

    it('reject tanpa alasan ditolak (client + server 422)', async () => {
        const user = userEvent.setup();
        await openDetail(user);
        await user.clear(screen.getByLabelText('Nama trainee'));
        await user.type(screen.getByLabelText('Nama trainee'), 'sinta');
        await user.click(screen.getByRole('button', { name: 'Simpan nama' }));
        expect(await screen.findByText('Nama tersimpan.')).toBeInTheDocument();
        const remediation = screen.getByRole('region', { name: 'nonaktifkan provider bca' });
        await user.click(within(remediation).getByRole('button', { name: 'Minta persetujuan' }));
        expect(await within(remediation).findByText('Menunggu keputusan')).toBeInTheDocument();
        await user.click(within(remediation).getByRole('button', { name: 'Reject' }));
        expect(await screen.findByText('Alasan wajib diisi saat Reject.')).toBeInTheDocument();
        // Status tetap pending (tidak ada keputusan terekam).
        expect(within(remediation).getByText('Menunggu keputusan')).toBeInTheDocument();
    });

    it('catatan manual masuk timeline', async () => {
        const user = userEvent.setup();
        await openDetail(user);
        const box = screen.getByLabelText(/Catatan manual/);
        fireEvent.change(box, { target: { value: 'observasi <b>penting</b>' } });
        await user.click(screen.getByRole('button', { name: 'Tambah catatan' }));
        expect(await screen.findByText('observasi <b>penting</b>')).toBeInTheDocument();
        expect(document.querySelector('b')).toBeNull();
    });

    it('postmortem: simpan + markdown valid + ekspor', async () => {
        const user = userEvent.setup();
        await openDetail(user);
        const summary = await screen.findByLabelText('Ringkasan');
        await user.type(summary, 'bca down 10 menit');
        await user.type(screen.getByLabelText(/Action items/), 'tambah alert\nperbaiki runbook');
        await user.click(screen.getByRole('button', { name: 'Simpan postmortem' }));
        expect(await screen.findByText('Postmortem tersimpan.')).toBeInTheDocument();
        const pres = document.querySelectorAll('pre');
        expect(pres.length).toBeGreaterThan(0);
        const md = pres[pres.length - 1]?.textContent ?? '';
        for (const want of [
            '# Postmortem',
            '## Ringkasan',
            'bca down 10 menit',
            '## Action items',
            '- [ ] tambah alert',
            '- [ ] perbaiki runbook',
        ]) {
            expect(md).toContain(want);
        }
        expect(screen.getByRole('button', { name: 'Ekspor Markdown' })).toBeInTheDocument();
    });
});
