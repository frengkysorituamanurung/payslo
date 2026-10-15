import { useTranslation } from 'react-i18next';

export function EmptyState({ next }: { next: string }) {
    const { t } = useTranslation();
    return (
        <div
            role="status"
            style={{
                padding: 16,
                border: '1px dashed var(--border)',
                borderRadius: 'var(--radius)',
            }}
        >
            <p style={{ margin: 0, fontWeight: 600 }}>{t('common.empty')}</p>
            <p style={{ color: 'var(--text-muted)' }}>{next}</p>
        </div>
    );
}

export function ErrorState({ onRetry }: { onRetry: () => void }) {
    const { t } = useTranslation();
    return (
        <div
            role="alert"
            style={{ padding: 16, border: '1px solid var(--crit)', borderRadius: 'var(--radius)' }}
        >
            <p style={{ margin: 0, fontWeight: 600 }}>✕ {t('error.title')}</p>
            <p style={{ color: 'var(--text-muted)' }}>{t('error.hint')}</p>
            <button type="button" onClick={onRetry}>
                {t('common.retry')}
            </button>
        </div>
    );
}

export function StaleBanner({ lastData }: { lastData: string }) {
    const { t } = useTranslation();
    return (
        <p
            role="status"
            style={{
                background: 'var(--warn-bg)',
                color: 'var(--warn)',
                padding: '6px 10px',
                borderRadius: 'var(--radius)',
            }}
        >
            ⏱ {t('status.stale')}: {t('common.lastData')} {lastData}
        </p>
    );
}
