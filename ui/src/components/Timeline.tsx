import { useTranslation } from 'react-i18next';
import type { PaymentEvent } from '../api/client';
import { formatLocal, formatRelative } from '../utils/time';

// Ikon per tipe langkah. Hanya simbol (bukan huruf) — warna/ikon tidak pernah
// satu-satunya pembawa makna (U8); teks status tetap ditampilkan.
const GLYPH: Record<PaymentEvent['type'], string> = {
    created: '＋',
    pending: '⋯',
    callback: '↩',
    final: '⏹',
    webhook: '→',
};

// U1: waktu nyata bila ada; langkah turunan tanpa timestamp ditandai jujur
// (tidak dikarang). Timeline memakai <ol> agar urutan diumumkan pembaca layar.
export function Timeline({ events }: { events: PaymentEvent[] }) {
    const { t, i18n } = useTranslation();
    return (
        <ol style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: 10 }}>
            {events.map((e, i) => (
                <li key={`${e.type}-${String(i)}`} style={{ display: 'flex', gap: 8 }}>
                    <span aria-hidden="true">{GLYPH[e.type]}</span>
                    <div>
                        <strong>{t(`payments.event.${e.type}`)}</strong>
                        {e.status !== null && e.status !== undefined && (
                            <>
                                {' '}
                                <span>{e.status}</span>
                            </>
                        )}
                        {e.provider !== null && e.provider !== undefined && (
                            <>
                                {' '}
                                <span>{e.provider}</span>
                            </>
                        )}
                        {e.attempt !== null && e.attempt !== undefined && (
                            <>
                                {' '}
                                <span>
                                    {t('payments.attempt')} {e.attempt}
                                </span>
                            </>
                        )}
                        {e.httpStatus !== null && e.httpStatus !== undefined && (
                            <>
                                {' '}
                                <span>
                                    {t('payments.httpLabel')} {e.httpStatus}
                                </span>
                            </>
                        )}
                        {e.detail !== null && e.detail !== undefined && (
                            <p style={{ margin: 0 }}>{e.detail}</p>
                        )}
                        <p style={{ margin: 0, color: 'var(--text-muted)', fontSize: 12 }}>
                            {e.at === null ? (
                                <span title={t('payments.derivedHint')}>
                                    {t('payments.noTimestamp')}
                                </span>
                            ) : (
                                <time dateTime={e.at} title={e.at}>
                                    {formatLocal(e.at, i18n.language)} ·{' '}
                                    {formatRelative(e.at, i18n.language)}
                                </time>
                            )}
                        </p>
                    </div>
                </li>
            ))}
        </ol>
    );
}
