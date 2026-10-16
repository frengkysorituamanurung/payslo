import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    useAlertmanager,
    useAnnotations,
    useGrafanaEmbed,
    useNtfyHistory,
    useObserveStatus,
    usePrometheusAlerts,
    usePrometheusRules,
} from '../hooks/useObserve';
import { EmptyState, ErrorState } from '../components/States';
import { formatLocal, formatRelative } from '../utils/time';

function Card({ title, children }: { title: string; children: React.ReactNode }) {
    return (
        <section
            aria-label={title}
            style={{
                border: '1px solid var(--border)',
                borderRadius: 'var(--radius)',
                padding: 16,
            }}
        >
            <h2 style={{ marginTop: 0 }}>{title}</h2>
            {children}
        </section>
    );
}

// ------------------------------------------------------------------ iframe Grafana

// Iframe Grafana (mode kiosk) dengan deteksi gagal-embed: onLoad menandai
// sukses; onError ATAU timeout 10 dtk tanpa load -> fallback "Buka di tab
// baru". Fallback selalu tersedia sebagai tombol (iframe bisa diblokir
// tanpa event error yang jelas).
function GrafanaFrame({ src, title }: { src: string; title: string }) {
    const { t } = useTranslation();
    const [state, setState] = useState<'loading' | 'loaded' | 'failed'>('loading');
    const timer = useRef<number | null>(null);

    // GrafanaFrame di-remount per src (key dari induk): state awal 'loading'
    // sudah benar tanpa setState sinkron di effect. Probe fetch no-cors
    // menolak saat network gagal (onError iframe tidak andal; onLoad bisa
    // menyala untuk halaman error) -> fallback cepat dan pasti.
    useEffect(() => {
        let cancelled = false;
        void fetch(src, { mode: 'no-cors' }).then(
            () => {
                if (!cancelled) {
                    if (timer.current !== null) window.clearTimeout(timer.current);
                    timer.current = window.setTimeout(() => {
                        if (!cancelled) setState((prev) => (prev === 'loading' ? 'failed' : prev));
                    }, 10_000);
                }
            },
            () => {
                if (!cancelled) setState('failed');
            },
        );
        return () => {
            cancelled = true;
            if (timer.current !== null) window.clearTimeout(timer.current);
        };
    }, [src]);

    return (
        <div>
            {state === 'loading' && <p role="status">{t('observe.iframeLoading')}</p>}
            {state === 'failed' ? (
                <div
                    role="alert"
                    style={{
                        border: '1px solid var(--warn)',
                        borderRadius: 'var(--radius)',
                        padding: 12,
                    }}
                >
                    <p style={{ marginTop: 0 }}>{t('observe.iframeBlocked')}</p>
                    <p style={{ color: 'var(--text-muted)', fontSize: 12 }}>
                        {t('observe.iframeBlockedHint')}
                    </p>
                    <a
                        href={src.replace('&kiosk', '').replace('?kiosk', '')}
                        target="_blank"
                        rel="noreferrer"
                    >
                        {t('observe.openNewTab')}
                    </a>
                </div>
            ) : (
                <iframe
                    src={src}
                    title={title}
                    style={{
                        width: '100%',
                        height: 480,
                        border: '1px solid var(--border)',
                        borderRadius: 'var(--radius)',
                    }}
                    loading="lazy"
                    onLoad={() => {
                        if (timer.current !== null) window.clearTimeout(timer.current);
                        setState('loaded');
                    }}
                    onError={() => {
                        if (timer.current !== null) window.clearTimeout(timer.current);
                        setState('failed');
                    }}
                />
            )}
            <p style={{ marginBottom: 0 }}>
                <a
                    href={src.replace('&kiosk', '').replace('?kiosk', '')}
                    target="_blank"
                    rel="noreferrer"
                >
                    {t('observe.openNewTab')}
                </a>
            </p>
        </div>
    );
}

function GrafanaPanel() {
    const { t } = useTranslation();
    const embed = useGrafanaEmbed();
    const [dashboard, setDashboard] = useState('payment-overview');

    if (embed.isPending) return <p>{t('common.loading')}</p>;
    if (embed.isError) return <ErrorState onRetry={() => void embed.refetch()} />;
    const dashboards = embed.data.dashboards;
    const current = dashboards.find((d) => d.uid === dashboard) ?? dashboards[0];
    if (current === undefined) return <EmptyState next={t('observe.noDashboards')} />;
    const base = embed.data.url.replace(/\/$/, '');
    const src = `${base}/d/${current.uid}?orgId=1&kiosk`;

    return (
        <div style={{ display: 'grid', gap: 12 }}>
            <label>
                {t('observe.dashboard')}{' '}
                <select
                    value={current.uid}
                    onChange={(e) => {
                        setDashboard(e.target.value);
                    }}
                >
                    {dashboards.map((d) => (
                        <option key={d.uid} value={d.uid}>
                            {d.title}
                        </option>
                    ))}
                </select>
            </label>
            <GrafanaFrame key={src} src={src} title={current.title} />
        </div>
    );
}

// ------------------------------------------------------------------ tab Prometheus

function PrometheusPanel() {
    const { t, i18n } = useTranslation();
    const alerts = usePrometheusAlerts();
    const rules = usePrometheusRules();

    return (
        <div style={{ display: 'grid', gap: 16 }}>
            <div>
                <h3>{t('observe.firingAlerts')}</h3>
                {alerts.isPending && <p>{t('common.loading')}</p>}
                {alerts.isError && <ErrorState onRetry={() => void alerts.refetch()} />}
                {alerts.isSuccess && alerts.data.length === 0 && (
                    <EmptyState next={t('observe.noAlerts')} />
                )}
                {alerts.isSuccess && alerts.data.length > 0 && (
                    <ul
                        style={{
                            listStyle: 'none',
                            padding: 0,
                            margin: 0,
                            display: 'grid',
                            gap: 8,
                        }}
                    >
                        {alerts.data.map((a, i) => (
                            <li
                                key={`${a.labels['alertname'] ?? 'alert'}-${String(i)}`}
                                style={{
                                    border: '1px solid var(--border)',
                                    borderRadius: 'var(--radius)',
                                    padding: 8,
                                }}
                            >
                                <p style={{ margin: 0, fontWeight: 700 }}>
                                    ✕ {a.labels['alertname'] ?? t('observe.unknownAlert')} ·{' '}
                                    {a.state}
                                </p>
                                <p style={{ margin: 0, color: 'var(--text-muted)', fontSize: 12 }}>
                                    {Object.entries(a.labels)
                                        .filter(([k]) => k !== 'alertname')
                                        .map(([k, v]) => `${k}=${v}`)
                                        .join(' ')}
                                    {' · '}
                                    <time dateTime={a.activeAt} title={a.activeAt}>
                                        {formatRelative(a.activeAt, i18n.language)}
                                    </time>
                                </p>
                            </li>
                        ))}
                    </ul>
                )}
            </div>
            <div>
                <h3>{t('observe.promRules')}</h3>
                {rules.isPending && <p>{t('common.loading')}</p>}
                {rules.isError && <ErrorState onRetry={() => void rules.refetch()} />}
                {rules.isSuccess && (
                    <ul
                        style={{
                            listStyle: 'none',
                            padding: 0,
                            margin: 0,
                            display: 'grid',
                            gap: 8,
                        }}
                    >
                        {rules.data.map((g) => (
                            <li key={g.name}>
                                <strong>{g.name}</strong>
                                <ul>
                                    {g.rules.map((r) => (
                                        <li key={r}>{r}</li>
                                    ))}
                                </ul>
                            </li>
                        ))}
                    </ul>
                )}
            </div>
        </div>
    );
}

function AlertmanagerPanel() {
    const { t } = useTranslation();
    const am = useAlertmanager();
    if (am.isPending) return <p>{t('common.loading')}</p>;
    if (am.isError) return <ErrorState onRetry={() => void am.refetch()} />;
    if (am.data.length === 0) return <EmptyState next={t('observe.noAlerts')} />;
    return (
        <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: 8 }}>
            {am.data.map((a, i) => (
                <li
                    key={`${a.labels['alertname'] ?? 'alert'}-${String(i)}`}
                    style={{
                        border: '1px solid var(--border)',
                        borderRadius: 'var(--radius)',
                        padding: 8,
                    }}
                >
                    <p style={{ margin: 0, fontWeight: 700 }}>
                        {a.state === 'active' ? '✕' : '✓'}{' '}
                        {a.labels['alertname'] ?? t('observe.unknownAlert')} · {a.state}
                    </p>
                </li>
            ))}
        </ul>
    );
}

function NtfyPanel() {
    const { t, i18n } = useTranslation();
    const history = useNtfyHistory();
    if (history.isPending) return <p>{t('common.loading')}</p>;
    if (history.isError) return <ErrorState onRetry={() => void history.refetch()} />;
    if (history.data.messages.length === 0) return <EmptyState next={t('observe.noNtfy')} />;
    return (
        <div>
            <p style={{ color: 'var(--text-muted)', fontSize: 12 }}>
                {t('observe.ntfySource', { source: history.data.source })}
            </p>
            <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: 8 }}>
                {history.data.messages.map((m) => (
                    <li
                        key={m.id}
                        style={{
                            border: '1px solid var(--border)',
                            borderRadius: 'var(--radius)',
                            padding: 8,
                        }}
                    >
                        <p style={{ margin: 0 }}>{m.message}</p>
                        <p style={{ margin: 0, color: 'var(--text-muted)', fontSize: 12 }}>
                            {m.topic} ·{' '}
                            <time dateTime={m.time} title={m.time}>
                                {formatLocal(m.time, i18n.language)}
                            </time>
                        </p>
                    </li>
                ))}
            </ul>
        </div>
    );
}

function AnnotationsPanel() {
    const { t, i18n } = useTranslation();
    const annotations = useAnnotations();
    if (annotations.isPending) return <p>{t('common.loading')}</p>;
    if (annotations.isError) return <ErrorState onRetry={() => void annotations.refetch()} />;
    if (annotations.data.length === 0) return <EmptyState next={t('observe.noAnnotations')} />;
    return (
        <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: 6 }}>
            {annotations.data.map((a) => (
                <li key={a.id} style={{ display: 'flex', gap: 8 }}>
                    <span aria-hidden="true">📌</span>
                    <div>
                        <span>{a.text}</span>{' '}
                        <span style={{ color: 'var(--text-muted)', fontSize: 12 }}>
                            {a.tags.join(' ')} ·{' '}
                            <time dateTime={a.time} title={a.time}>
                                {formatLocal(a.time, i18n.language)}
                            </time>
                        </span>
                    </div>
                </li>
            ))}
        </ul>
    );
}

// ------------------------------------------------------------------ halaman

type ObserveTab = 'grafana' | 'prometheus' | 'alertmanager' | 'ntfy';

export function ObservePage() {
    const { t } = useTranslation();
    const status = useObserveStatus();
    const [tab, setTab] = useState<ObserveTab>('grafana');

    const obsFull = status.data?.composeProfiles['obs-full'] ?? true;
    const lokiDown =
        status.isSuccess &&
        status.data.services.find((s) => s.name === 'loki')?.reachable === false;

    return (
        <section aria-labelledby="observe-title">
            <h1 id="observe-title">{t('nav.observe')}</h1>
            {(lokiDown || !obsFull) && status.isSuccess && (
                <div
                    role="note"
                    style={{
                        border: '1px solid var(--warn)',
                        borderRadius: 'var(--radius)',
                        padding: 12,
                        marginBottom: 16,
                    }}
                >
                    <p style={{ marginTop: 0 }}>
                        <strong>⏱ {t('observe.obsFullOff')}</strong>
                    </p>
                    <p style={{ marginBottom: 0, color: 'var(--text-muted)', fontSize: 12 }}>
                        {t('observe.obsFullOffHint')}
                    </p>
                </div>
            )}
            <div
                role="tablist"
                aria-label={t('nav.observe')}
                style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}
            >
                {(['grafana', 'prometheus', 'alertmanager', 'ntfy'] as const).map((name) => (
                    <button
                        key={name}
                        role="tab"
                        aria-selected={tab === name}
                        type="button"
                        onClick={() => {
                            setTab(name);
                        }}
                        style={{
                            border: '1px solid var(--border)',
                            borderRadius: 'var(--radius)',
                            padding: '6px 12px',
                            background: tab === name ? 'var(--neutral-bg)' : undefined,
                            fontWeight: tab === name ? 700 : undefined,
                        }}
                    >
                        {t(`observe.tab.${name}`)}
                    </button>
                ))}
            </div>
            <div role="tabpanel">
                {tab === 'grafana' && (
                    <Card title={t('observe.tab.grafana')}>
                        <GrafanaPanel />
                    </Card>
                )}
                {tab === 'prometheus' && (
                    <Card title={t('observe.tab.prometheus')}>
                        <PrometheusPanel />
                    </Card>
                )}
                {tab === 'alertmanager' && (
                    <Card title={t('observe.tab.alertmanager')}>
                        <AlertmanagerPanel />
                    </Card>
                )}
                {tab === 'ntfy' && (
                    <Card title={t('observe.tab.ntfy')}>
                        <NtfyPanel />
                    </Card>
                )}
            </div>
            <div style={{ marginTop: 16 }}>
                <Card title={t('observe.annotationsTitle')}>
                    <AnnotationsPanel />
                </Card>
            </div>
        </section>
    );
}
