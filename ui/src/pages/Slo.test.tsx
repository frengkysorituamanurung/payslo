import { fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it, vi } from 'vitest';
import { SloPage } from './Slo';

// CodeMirror 6 butuh API layout browser (getClientRects) yang tak ada di
// jsdom; di sini diganti textarea (logika alur tetap diuji lewat UI).
// Pengetikan di editor asli dicakup e2e Playwright (Chromium nyata).
vi.mock('../components/SloCodeMirror', () => ({
    default: ({ value, onChange }: { value: string; onChange: (v: string) => void }) => (
        <textarea
            aria-label="SLO YAML"
            value={value}
            onChange={(e) => {
                onChange(e.target.value);
            }}
        />
    ),
}));

function renderPage() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
        <QueryClientProvider client={qc}>
            <SloPage />
        </QueryClientProvider>,
    );
}

async function selectSlo(user: ReturnType<typeof userEvent.setup>, name: string): Promise<void> {
    await user.click(await screen.findByRole('button', { name: new RegExp(name) }));
}

describe('SloPage', () => {
    it('daftar + detail: budget, burn multi-window, deployment, alert aktif', async () => {
        const user = userEvent.setup();
        renderPage();
        await selectSlo(user, 'payment-latency-p99');
        expect(await screen.findByText('SLO payment-latency-p99')).toBeInTheDocument();
        // Budget 99%/1h = 0,6 menit.
        expect(screen.getByText(/Jatah error: 0,6 menit per 1h/)).toBeInTheDocument();
        // Burn multi-window + ambang (= (1-0.99)*8 = 8%).
        expect(screen.getByText('15m')).toBeInTheDocument();
        expect(screen.getByText('8.000%')).toBeInTheDocument();
        // Deployment: drifted di datadog-mock (contoh edit manual).
        expect(screen.getByText('Drift')).toBeInTheDocument();
        // Alert aktif firing.
        expect(screen.getByText(/PaySLOBurnRateHigh/)).toBeInTheDocument();
        // Matriks kapabilitas + peringatan datadog.
        expect(await screen.findByText('recording rules')).toBeInTheDocument();
        expect(screen.getByText(/hanya mengenal timeframe 7d/)).toBeInTheDocument();
    });

    it('kalkulator: 99.9%/30d ≈ 43,2 dan 99.99%/30d ≈ 4,32', async () => {
        const user = userEvent.setup();
        renderPage();
        const obj = await screen.findByLabelText('Objektif (%)');
        const win = screen.getByLabelText(/Window/);
        await user.clear(obj);
        await user.type(obj, '99.9');
        await user.clear(win);
        await user.type(win, '30d');
        expect(await screen.findByText('Jatah error: 43,2 menit per 30d.')).toBeInTheDocument();
        await user.clear(obj);
        await user.type(obj, '99.99');
        expect(await screen.findByText('Jatah error: 4,32 menit per 30d.')).toBeInTheDocument();
    });

    it('alur E2E: save buruk -> validate gagal berlokasi -> perbaiki -> plan diff -> apply -> plan kosong -> no-op', async () => {
        const user = userEvent.setup();
        renderPage();
        await selectSlo(user, 'payment-success-rate');
        await screen.findByText('SLO payment-success-rate');

        const editor = await screen.findByLabelText('SLO YAML');

        // Rusakkan objective lewat editor.
        const bad = `apiVersion: payslo/v1
kind: SLO
name: payment-success-rate
service: gateway-api
objective: 99
window: 1h
sli:
  type: success-ratio
  source: metric
  good: payment_transactions_total{status="SUCCESS"}
  total: payment_transactions_total
alerting:
  runbook: runbooks/payment-success.md
  burnRates:
    - longWindow: 5m
      shortWindow: 1m
      burnRate: 14.4
      severity: page
targets:
  - backend: prometheus
`;
        fireEvent.change(editor, { target: { value: bad } });
        await user.click(screen.getByRole('button', { name: 'Simpan' }));
        expect(await screen.findByText(/Tersimpan/)).toBeInTheDocument();
        await user.click(screen.getByRole('button', { name: 'Validasi' }));
        const alerts = await screen.findAllByRole('alert');
        const texts = alerts.map((a) => a.textContent);
        expect(texts.some((x) => /Baris 5, kolom 1/.test(x))).toBe(true);
        expect(texts.some((x) => /objective/.test(x))).toBe(true);

        // Perbaiki (sekaligus ubah burnRate agar ada diff).
        const fixed = bad
            .replace('objective: 99', 'objective: 0.995')
            .replace('burnRate: 14.4', 'burnRate: 12');
        fireEvent.change(editor, { target: { value: fixed } });
        await user.click(screen.getByRole('button', { name: 'Simpan' }));
        expect(await screen.findByText(/Tersimpan/)).toBeInTheDocument();
        await user.click(screen.getByRole('button', { name: 'Validasi' }));
        expect(await screen.findByText(/SLO valid/)).toBeInTheDocument();

        // Plan menunjukkan diff prometheus.
        await user.click(screen.getByRole('button', { name: 'Plan' }));
        expect(await screen.findByText(/akan diubah di prometheus-local/)).toBeInTheDocument();
        const diffRegion = await screen.findByText(/baris tambah/);
        expect(diffRegion).toBeInTheDocument();
        const table = diffRegion.closest('div')?.querySelector('table');
        expect(table).not.toBeNull();
        expect(
            within(table as HTMLElement).getByRole('cell', { name: 'burnRate: 12' }),
        ).toBeInTheDocument();

        // Apply (ConfirmDialog ketik nama) -> applied.
        await user.click(screen.getByRole('button', { name: 'Apply…' }));
        const ack = await screen.findByLabelText('payment-success-rate');
        await user.type(ack, 'payment-success-rate');
        await user.click(screen.getByRole('button', { name: 'Konfirmasi' }));
        expect(
            await screen.findByText(/applied payment-success-rate on prometheus-local/),
        ).toBeInTheDocument();

        // Plan ulang kosong + apply kedua no-op jelas.
        await user.click(screen.getByRole('button', { name: 'Plan' }));
        expect(await screen.findByText(/Tidak ada perubahan/)).toBeInTheDocument();
        await user.click(screen.getByRole('button', { name: 'Apply…' }));
        const ack2 = await screen.findByLabelText('payment-success-rate');
        await user.type(ack2, 'payment-success-rate');
        await user.click(screen.getByRole('button', { name: 'Konfirmasi' }));
        expect(await screen.findByText(/no-op/)).toBeInTheDocument();
    }, 30000);

    it('drift panel menampilkan contoh edit manual datadog-mock', async () => {
        renderPage();
        expect(await screen.findByText(/payment-latency-p99 di datadog-mock/)).toBeInTheDocument();
    });
});
