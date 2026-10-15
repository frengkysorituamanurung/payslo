import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import uPlot from 'uplot';
import 'uplot/dist/uPlot.min.css';
import type { MetricsSeries } from '../hooks/useLab';

function canvas2dAvailable(): boolean {
    try {
        const c = document.createElement('canvas');
        return c.getContext('2d') !== null;
    } catch {
        return false;
    }
}

// Grafik deret waktu (uPlot) + tabel data alternatif (aksesibilitas).
// Gap (null) dirender sebagai jeda garis (spanGaps:false) — jujur, U1.
// Tanpa canvas (jsdom, canvas dimatikan): langsung tabel, tanpa throw.
export function TimeSeriesChart({
    series,
    height = 180,
}: {
    series: MetricsSeries;
    height?: number;
}) {
    const { t } = useTranslation();
    const ref = useRef<HTMLDivElement>(null);
    const plot = useRef<uPlot | null>(null);
    const [showTable, setShowTable] = useState(false);
    const [noCanvas] = useState(() => !canvas2dAvailable());

    const xs: number[] = [];
    const ys: Array<number | null> = [];
    for (const p of series.points) {
        xs.push(Date.parse(p[0]) / 1000);
        ys.push(p[1]);
    }

    useEffect(() => {
        if (ref.current === null || showTable || noCanvas) return;
        const dark = document.documentElement.dataset['theme'] === 'dark';
        const fg = dark ? '#f9fafb' : '#111827';
        const grid = dark ? '#374151' : '#e5e7eb';
        const opts: uPlot.Options = {
            width: ref.current.clientWidth || 600,
            height,
            series: [{}, { stroke: dark ? '#93c5fd' : '#1d4ed8', width: 2, spanGaps: false }],
            axes: [
                {
                    stroke: fg,
                    grid: { stroke: grid },
                    values: (_u, vals) => vals.map((v) => new Date(v * 1000).toLocaleTimeString()),
                },
                { stroke: fg, grid: { stroke: grid } },
            ],
        };
        plot.current = new uPlot(opts, [xs, ys] as uPlot.AlignedData, ref.current);
        const el = ref.current;
        const onResize = () => plot.current?.setSize({ width: el.clientWidth || 600, height });
        window.addEventListener('resize', onResize);
        return () => {
            window.removeEventListener('resize', onResize);
            plot.current?.destroy();
            plot.current = null;
        };
        // Render ulang saat data/tema berubah (throttle di hook SSE).
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [series, showTable, height, noCanvas]);

    return (
        <div>
            <button
                type="button"
                onClick={() => {
                    setShowTable((v) => !v);
                }}
            >
                {showTable ? t('mission.showChart') : t('mission.showTable')}
            </button>
            {showTable || noCanvas ? (
                <table>
                    <caption>{series.name}</caption>
                    <tbody>
                        {series.points.map((p) => (
                            <tr key={p[0]}>
                                <td>{p[0]}</td>
                                <td>{p[1] === null ? '—' : String(p[1])}</td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            ) : (
                <div ref={ref} role="img" aria-label={series.name} />
            )}
        </div>
    );
}
