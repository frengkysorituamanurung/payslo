import { useTranslation } from 'react-i18next';
import { useEvents } from '../hooks/useEvents';
import { useMetricsSeries, useMetricsSummary, type MetricValue } from '../hooks/useLab';
import { useSettings } from '../hooks/useIncidents';
import { LOW_SAMPLE_RPS, fmtPct, fmtRps } from '../utils/format';
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

function MetricPanel({
    title,
    metric,
    format,
    staleSince,
}: {
    title: string;
    metric: MetricValue | undefined;
    format: (v: number) => string;
    staleSince?: string | null;
}) {
    const { t } = useTranslation();
    const threshold = useLowSampleThreshold();
    if (metric === undefined) return <p>{t('common.loading')}</p>;
    if (metric.status === 'source_down') return <ErrorState onRetry={reloadPage} />;
    if (metric.status === 'no_data') return <EmptyState next={title} />;
    const lowSample = metric.sampleRate !== null && metric.sampleRate < threshold;
    return (
        <div>
            {metric.status === 'stale' && <StaleBanner lastData={metric.lastDataAt ?? '?'} />}
            {lowSample && <p role="note">⚠ {t('mission.lowSample')}</p>}
            <MetricTile
                title={title}
                value={metric.value === null ? '—' : format(metric.value)}
                dataState={metric.status === 'stale' ? 'stale' : 'ok'}
                hint={staleSince ?? undefined}
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

export function MissionControl() {
    const { t } = useTranslation();
    const summary = useMetricsSummary('5m');
    const events = useEvents();

    return (
        <section aria-labelledby="mission-title">
            <h1 id="mission-title">{t('nav.mission')}</h1>
            <p role="status">
                {events.live === 'live'
                    ? `● ${t('mission.live')}`
                    : `○ ${t('mission.reconnecting')}`}
            </p>

            {summary.isPending && <p>{t('common.loading')}</p>}
            {summary.isError && (
                <ErrorState
                    onRetry={() => {
                        void summary.refetch();
                    }}
                />
            )}
            {summary.isSuccess && (
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
                    <section aria-label={t('mission.httpPanel')}>
                        <h2>{t('mission.httpPanel')}</h2>
                        <MetricPanel
                            title={t('mission.ratio2xx')}
                            metric={summary.data.metrics['http_2xx_ratio']}
                            format={fmtPct}
                        />
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
                        <SeriesPanel name="http_2xx_ratio" />
                    </section>
                    <section aria-label={t('mission.payPanel')}>
                        <h2>{t('mission.payPanel')}</h2>
                        <p>{t('mission.whyDifferent')}</p>
                        <MetricPanel
                            title={t('mission.successRatio')}
                            metric={summary.data.metrics['payment_success_ratio']}
                            format={fmtPct}
                        />
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
                        <SeriesPanel name="payment_success_ratio" />
                    </section>
                </div>
            )}

            <h2>{t('mission.perProvider')}</h2>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 12 }}>
                {PROVIDERS.map((p) => {
                    const m = summary.data?.metrics[`payment_success_ratio_by_provider:${p}`];
                    return (
                        <div key={p}>
                            <h3>{p.toUpperCase()}</h3>
                            {summary.isSuccess && m !== undefined ? (
                                <MetricPanel title={p} metric={m} format={fmtPct} />
                            ) : summary.isError ? (
                                <ErrorState
                                    onRetry={() => {
                                        void summary.refetch();
                                    }}
                                />
                            ) : (
                                <p>{t('common.loading')}</p>
                            )}
                            {m?.status === 'ok' && (m.value ?? 1) < 0.9 && (
                                <StatusBadge state="down" />
                            )}
                            {m?.status === 'ok' && (m.value ?? 1) >= 0.9 && (
                                <StatusBadge state="up" />
                            )}
                        </div>
                    );
                })}
            </div>
        </section>
    );
}
