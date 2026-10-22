import { useTranslation } from 'react-i18next';
import { useEvents } from '../hooks/useEvents';
import { useMetricsSeries, useMetricsSummary, type MetricValue } from '../hooks/useLab';
import { useSettings } from '../hooks/useIncidents';
import { LOW_SAMPLE_RPS, fmtMs, fmtPct, fmtRps } from '../utils/format';
import { MetricTile } from './MetricTile';
import { EmptyState, ErrorState, StaleBanner } from './States';
import { StatusBadge } from './StatusBadge';
import { TimeSeriesChart } from './TimeSeriesChart';

function reloadPage(): void {
    window.location.reload();
}

function useLowSampleThreshold(): number {
    const settings = useSettings();
    const v = settings.data?.lowSampleThreshold;
    if (typeof v === 'number' && Number.isFinite(v)) return v;
    return LOW_SAMPLE_RPS;
}

function Meter({ value }: { value: number }) {
    const pct = Math.min(100, Math.max(0, value * 100));
    return (
        <div className="signal-bar" aria-hidden="true">
            <span style={{ width: `${String(pct)}%` }} />
        </div>
    );
}

function MetricPanel({
    title,
    metric,
    format,
    variant = 'compact',
    meter = false,
}: {
    title: string;
    metric: MetricValue | undefined;
    format: (v: number) => string;
    variant?: 'primary' | 'compact';
    meter?: boolean;
}) {
    const { t } = useTranslation();
    const threshold = useLowSampleThreshold();
    if (metric === undefined) return <p>{t('common.loading')}</p>;
    if (metric.status === 'source_down') return <ErrorState onRetry={reloadPage} />;
    if (metric.status === 'no_data') return <EmptyState next={title} />;
    const lowSample = metric.sampleRate !== null && metric.sampleRate < threshold;
    const value = metric.value === null ? '—' : format(metric.value);

    if (variant === 'primary') {
        return (
            <section aria-label={title} className="signal-primary">
                {metric.status === 'stale' && <StaleBanner lastData={metric.lastDataAt ?? '?'} />}
                {lowSample && <p className="note-warn">{t('mission.lowSample')}</p>}
                <p className="signal-label">{title}</p>
                <p className="signal-value">{value}</p>
                {meter && metric.value !== null && <Meter value={metric.value} />}
            </section>
        );
    }

    return (
        <div>
            <MetricTile
                title={title}
                value={value}
                dataState={metric.status === 'stale' ? 'stale' : 'ok'}
            />
        </div>
    );
}

function SeriesPanel({
    name,
    provider,
}: {
    name: 'http_2xx_ratio' | 'payment_success_ratio';
    provider?: 'bca' | 'mandiri' | 'ewallet';
}) {
    const { t } = useTranslation();
    const q = useMetricsSeries(name, '15m', provider);
    if (q.isPending) return <p>{t('common.loading')}</p>;
    if (q.isError)
        return (
            <ErrorState
                onRetry={() => {
                    void q.refetch();
                }}
            />
        );
    if (q.data.status === 'source_down')
        return (
            <ErrorState
                onRetry={() => {
                    void q.refetch();
                }}
            />
        );
    if (q.data.status === 'no_data' || q.data.points.length === 0)
        return <EmptyState next={name} />;
    return (
        <div>
            {q.data.status === 'stale' && <StaleBanner lastData={q.data.lastDataAt ?? '?'} />}
            <TimeSeriesChart series={q.data} />
        </div>
    );
}

const PROVIDERS = ['bca', 'mandiri', 'ewallet'] as const;
const PROVIDER_LABEL: Record<(typeof PROVIDERS)[number], string> = {
    bca: 'BCA',
    mandiri: 'Mandiri',
    ewallet: 'E-wallet',
};

export function MissionControl() {
    const { t } = useTranslation();
    const summary = useMetricsSummary('5m');
    const events = useEvents();
    const live = events.live === 'live';

    const http = summary.data?.metrics['http_2xx_ratio'];
    const pay = summary.data?.metrics['payment_success_ratio'];
    const gap =
        http?.status === 'ok' &&
        pay?.status === 'ok' &&
        typeof http.value === 'number' &&
        typeof pay.value === 'number'
            ? (http.value - pay.value) * 100
            : null;
    const gapText = gap === null ? '' : `${(Math.abs(gap) < 0.05 ? 0 : gap).toFixed(1)} pp`;

    return (
        <section className="page" aria-labelledby="mission-title">
            <header className="page-header">
                <div>
                    <h1 id="mission-title">{t('nav.mission')}</h1>
                    <p className="page-lede">{t('mission.subtitle')}</p>
                </div>
                <p
                    role="status"
                    className={live ? 'chip chip-live live-chip is-live' : 'chip live-chip'}
                >
                    <span className="live-dot" aria-hidden="true" />
                    {live ? t('mission.live') : t('mission.reconnecting')}
                </p>
            </header>

            {summary.isPending && <p>{t('common.loading')}</p>}
            {summary.isError && (
                <ErrorState
                    onRetry={() => {
                        void summary.refetch();
                    }}
                />
            )}

            {summary.isSuccess && (
                <div className="signal-block">
                    <div className="signal-grid">
                        <section className="signal" aria-label={t('mission.httpPanel')}>
                            <div className="signal-head">
                                <h2>{t('mission.httpPanel')}</h2>
                                <span className="signal-source">{t('mission.edge')}</span>
                            </div>
                            <MetricPanel
                                variant="primary"
                                meter
                                title={t('mission.ratio2xx')}
                                metric={summary.data.metrics['http_2xx_ratio']}
                                format={fmtPct}
                            />
                            <div className="stat-grid">
                                <MetricPanel
                                    title={t('mission.rps')}
                                    metric={summary.data.metrics['http_rps']}
                                    format={fmtRps}
                                />
                                <MetricPanel
                                    title={t('mission.ratio5xx')}
                                    metric={summary.data.metrics['http_5xx_ratio']}
                                    format={fmtPct}
                                />
                                <MetricPanel
                                    title={t('mission.p99')}
                                    metric={summary.data.metrics['http_p99_ms']}
                                    format={fmtMs}
                                />
                            </div>
                            <div className="signal-chart">
                                <p className="signal-chart-label">{t('mission.trend')}</p>
                                <SeriesPanel name="http_2xx_ratio" />
                            </div>
                        </section>

                        <section className="signal" aria-label={t('mission.payPanel')}>
                            <div className="signal-head">
                                <h2>{t('mission.payPanel')}</h2>
                                <span className="signal-source">{t('mission.ledger')}</span>
                            </div>
                            <MetricPanel
                                variant="primary"
                                meter
                                title={t('mission.successRatio')}
                                metric={summary.data.metrics['payment_success_ratio']}
                                format={fmtPct}
                            />
                            <div className="stat-grid">
                                <MetricPanel
                                    title={t('mission.rps')}
                                    metric={summary.data.metrics['payment_rps']}
                                    format={fmtRps}
                                />
                                <MetricPanel
                                    title={t('mission.pending')}
                                    metric={summary.data.metrics['payment_pending_stuck']}
                                    format={(v) => String(Math.round(v))}
                                />
                                <MetricPanel
                                    title={t('mission.webhook')}
                                    metric={summary.data.metrics['webhook_delivery_ratio']}
                                    format={fmtPct}
                                />
                            </div>
                            <p className="signal-note">{t('mission.whyDifferent')}</p>
                            <div className="signal-chart">
                                <p className="signal-chart-label">{t('mission.trend')}</p>
                                <SeriesPanel name="payment_success_ratio" />
                            </div>
                        </section>
                    </div>
                    {gap !== null && (
                        <p className="signal-gap" role="note">
                            <span className="signal-gap-label">{t('mission.gapLabel')}</span>
                            <span className="signal-gap-value">{gapText}</span>
                            <span className="signal-gap-note">{t('mission.gapNote')}</span>
                        </p>
                    )}
                </div>
            )}

            <section className="panel providers" aria-labelledby="providers-title">
                <div className="section-head">
                    <h2 id="providers-title">{t('mission.perProvider')}</h2>
                    <span className="section-meta">{t('mission.window5m')}</span>
                </div>
                <div className="provider-list">
                    {PROVIDERS.map((p) => {
                        const m = summary.data?.metrics[`payment_success_ratio_by_provider:${p}`];
                        const value =
                            m?.status === 'ok' && typeof m.value === 'number' ? m.value : null;
                        const healthy = value === null ? null : value >= 0.9;
                        return (
                            <div className="provider-row" key={p}>
                                <span className="provider-name">{PROVIDER_LABEL[p]}</span>
                                <span className="provider-meter" aria-hidden="true">
                                    {value !== null && (
                                        <span
                                            style={{
                                                width: `${String(Math.min(100, Math.max(0, value * 100)))}%`,
                                                background: healthy ? 'var(--ok)' : 'var(--crit)',
                                            }}
                                        />
                                    )}
                                </span>
                                <span className="provider-value">
                                    {value !== null
                                        ? fmtPct(value)
                                        : m === undefined
                                          ? t('common.loading')
                                          : m.status === 'source_down'
                                            ? t('error.title')
                                            : t('common.empty')}
                                </span>
                                {healthy === null ? (
                                    <span className="section-meta">—</span>
                                ) : (
                                    <StatusBadge state={healthy ? 'up' : 'down'} />
                                )}
                            </div>
                        );
                    })}
                </div>
            </section>
        </section>
    );
}
