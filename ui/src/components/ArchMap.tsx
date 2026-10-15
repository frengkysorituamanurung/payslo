import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useStackHealth } from '../hooks/useLab';
import { StatusBadge, type StatusKind } from './StatusBadge';
import { EmptyState, ErrorState } from './States';

// Peta Arsitektur U1: SVG tetap + tampilan daftar setara + panel samping.
// Edge HANYA yang ber-metric nyata (U1: sync request, async callback).
// Navigasi keyboard: Tab antar node (button), Enter memilih, Esc menutup panel.

type NodeId =
    | 'loadgen'
    | 'envoy'
    | 'gateway'
    | 'bca'
    | 'mandiri'
    | 'ewallet'
    | 'merchant'
    | 'postgres'
    | 'prometheus'
    | 'payslo';

const NODES: Array<{ id: NodeId; x: number; y: number }> = [
    { id: 'loadgen', x: 20, y: 60 },
    { id: 'envoy', x: 140, y: 60 },
    { id: 'gateway', x: 260, y: 60 },
    { id: 'bca', x: 400, y: 10 },
    { id: 'mandiri', x: 400, y: 60 },
    { id: 'ewallet', x: 400, y: 110 },
    { id: 'merchant', x: 540, y: 110 },
    { id: 'postgres', x: 260, y: 150 },
    { id: 'prometheus', x: 140, y: 150 },
    { id: 'payslo', x: 540, y: 40 },
];

// Edge sync (garis penuh) vs async callback (garis putus-putus). Tanpa laju
// per edge yang tak diukur (dilarang UI-SPEC).
const EDGES: Array<{ from: NodeId; to: NodeId; async: boolean }> = [
    { from: 'loadgen', to: 'envoy', async: false },
    { from: 'envoy', to: 'gateway', async: false },
    { from: 'gateway', to: 'bca', async: false },
    { from: 'gateway', to: 'mandiri', async: false },
    { from: 'gateway', to: 'ewallet', async: false },
    { from: 'bca', to: 'gateway', async: true },
    { from: 'mandiri', to: 'gateway', async: true },
    { from: 'ewallet', to: 'gateway', async: true },
    { from: 'gateway', to: 'merchant', async: true },
    { from: 'gateway', to: 'postgres', async: false },
];

const posOf = (id: NodeId): { x: number; y: number } => {
    const n = NODES.find((v) => v.id === id);
    return { x: n?.x ?? 0, y: n?.y ?? 0 };
};

function stackStateFor(id: NodeId, services: Array<{ name: string; state: string }>): StatusKind {
    const match = (frag: string): string =>
        services.find((s) => s.name.includes(frag))?.state ?? 'unknown';
    const raw = match(
        id === 'gateway'
            ? 'gateway-api'
            : id === 'merchant'
              ? 'merchant-sink'
              : id === 'bca' || id === 'mandiri' || id === 'ewallet'
                ? 'bank'
                : id,
    );
    if (raw === 'up' || raw === 'down' || raw === 'degraded' || raw === 'unknown') return raw;
    return 'unknown';
}

export function ArchMap() {
    const { t } = useTranslation();
    const stack = useStackHealth();
    const [selected, setSelected] = useState<NodeId | null>(null);
    const [listView, setListView] = useState(false);
    const panelRef = useRef<HTMLElement>(null);
    const services = stack.data?.services ?? [];

    useEffect(() => {
        if (selected !== null) panelRef.current?.focus();
    }, [selected]);

    useEffect(() => {
        if (selected === null) return;
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') setSelected(null);
        };
        document.addEventListener('keydown', onKey);
        return () => {
            document.removeEventListener('keydown', onKey);
        };
    }, [selected]);

    const panel = (id: NodeId): string => {
        const st = stackStateFor(id, services);
        return `${id}: ${st}`;
    };

    return (
        <section aria-labelledby="map-title">
            <h1 id="map-title">{t('map.title')}</h1>
            <button
                type="button"
                onClick={() => {
                    setListView((v) => !v);
                }}
            >
                {listView ? t('map.graphView') : t('map.listView')}
            </button>

            {stack.isPending && <p>{t('common.loading')}</p>}
            {stack.isError && (
                <ErrorState
                    onRetry={() => {
                        void stack.refetch();
                    }}
                />
            )}
            {stack.isSuccess && stack.data.services.length === 0 && (
                <EmptyState next={t('map.title')} />
            )}

            {stack.isSuccess && !listView && (
                <svg
                    viewBox="0 0 660 190"
                    role="group"
                    aria-label={t('map.title')}
                    style={{ width: '100%', height: 'auto' }}
                >
                    {EDGES.map((e) => {
                        const a = posOf(e.from);
                        const b = posOf(e.to);
                        return (
                            <line
                                key={`${e.from}-${e.to}`}
                                x1={a.x + 44}
                                y1={a.y + 14}
                                x2={b.x}
                                y2={b.y + 14}
                                stroke="var(--border)"
                                strokeWidth={2}
                                strokeDasharray={e.async ? '6 4' : undefined}
                            >
                                <title>{e.async ? t('map.async') : t('map.sync')}</title>
                            </line>
                        );
                    })}
                    {NODES.map((n) => (
                        <g key={n.id}>
                            <rect
                                x={n.x}
                                y={n.y}
                                width={88}
                                height={28}
                                rx={6}
                                fill="var(--surface-raised)"
                                stroke="var(--border)"
                                strokeWidth={selected === n.id ? 3 : 1}
                            />
                            <foreignObject x={n.x} y={n.y} width={88} height={28}>
                                <button
                                    type="button"
                                    aria-pressed={selected === n.id}
                                    onClick={() => {
                                        setSelected(selected === n.id ? null : n.id);
                                    }}
                                    style={{
                                        width: '100%',
                                        height: '100%',
                                        background: 'transparent',
                                        border: 0,
                                        cursor: 'pointer',
                                        fontSize: 12,
                                    }}
                                >
                                    {n.id}
                                </button>
                            </foreignObject>
                        </g>
                    ))}
                </svg>
            )}

            {stack.isSuccess && listView && (
                <ul>
                    {NODES.map((n) => (
                        <li key={n.id}>
                            <button
                                type="button"
                                aria-pressed={selected === n.id}
                                onClick={() => {
                                    setSelected(selected === n.id ? null : n.id);
                                }}
                            >
                                {panel(n.id)}
                            </button>
                        </li>
                    ))}
                </ul>
            )}

            {selected !== null && (
                <aside ref={panelRef} aria-label={t('map.details')} tabIndex={-1}>
                    <h2>{selected}</h2>
                    <StatusBadge state={stackStateFor(selected, services)} />
                    <button
                        type="button"
                        onClick={() => {
                            setSelected(null);
                        }}
                    >
                        {t('common.close')}
                    </button>
                </aside>
            )}
        </section>
    );
}
