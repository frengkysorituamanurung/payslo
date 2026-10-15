import { useTranslation } from 'react-i18next';

export type DataState = 'ok' | 'stale' | 'none' | 'down';

// U1: bedakan nilai 0 dari tidak-ada-data dari data-usang dari sumber-down.
export function MetricTile({
    title,
    value,
    dataState,
    hint,
}: {
    title: string;
    value: string;
    dataState: DataState;
    hint?: string;
}) {
    const { t } = useTranslation();
    return (
        <section
            aria-label={title}
            style={{
                border: '1px solid var(--border)',
                borderRadius: 'var(--radius)',
                padding: 12,
                background: 'var(--surface-raised)',
            }}
        >
            <h3 style={{ margin: 0, fontSize: 13, color: 'var(--text-muted)' }}>{title}</h3>
            {dataState === 'none' ? (
                <p style={{ fontSize: 20, margin: '4px 0' }}>{t('common.empty')}</p>
            ) : (
                <p style={{ fontSize: 24, margin: '4px 0', fontWeight: 700 }}>
                    {value}
                    {dataState === 'stale' && (
                        <span style={{ fontSize: 13, color: 'var(--warn)' }}>
                            {' '}
                            ⏱ {t('status.stale')}
                        </span>
                    )}
                </p>
            )}
            {dataState === 'down' && <p style={{ color: 'var(--crit)' }}>{t('error.title')}</p>}
            {hint !== undefined && dataState !== 'none' && (
                <p style={{ fontSize: 12, color: 'var(--text-muted)', margin: 0 }}>{hint}</p>
            )}
        </section>
    );
}
