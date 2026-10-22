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
        <section aria-label={title} className="stat">
            <p className="stat-label">{title}</p>
            {dataState === 'none' ? (
                <p className="stat-value">{t('common.empty')}</p>
            ) : (
                <p className="stat-value">
                    {value}
                    {dataState === 'stale' && (
                        <span className="stat-stale"> {t('status.stale')}</span>
                    )}
                </p>
            )}
            {dataState === 'down' && (
                <p className="stat-label" style={{ color: 'var(--crit)' }}>
                    {t('error.title')}
                </p>
            )}
            {hint !== undefined && dataState !== 'none' && (
                <p className="stat-label" style={{ marginTop: 2 }}>
                    {hint}
                </p>
            )}
        </section>
    );
}
