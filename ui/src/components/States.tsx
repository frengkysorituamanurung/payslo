import { useTranslation } from 'react-i18next';

export function EmptyState({ next }: { next: string }) {
    const { t } = useTranslation();
    return (
        <div role="status" className="panel" style={{ borderStyle: 'dashed' }}>
            <p style={{ margin: 0, fontWeight: 600 }}>{t('common.empty')}</p>
            <p style={{ color: 'var(--text-muted)' }}>{next}</p>
        </div>
    );
}

export function ErrorState({ onRetry }: { onRetry: () => void }) {
    const { t } = useTranslation();
    return (
        <div role="alert" className="panel" style={{ borderColor: 'var(--crit)' }}>
            <p style={{ margin: 0, fontWeight: 600, color: 'var(--crit)' }}>{t('error.title')}</p>
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
        <p role="status" className="note-warn">
            {t('status.stale')}: {t('common.lastData')} {lastData}
        </p>
    );
}
