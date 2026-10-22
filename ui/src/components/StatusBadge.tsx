import { useTranslation } from 'react-i18next';

export type StatusKind = 'up' | 'down' | 'degraded' | 'unknown';

const GLYPH: Record<StatusKind, string> = { up: '✓', down: '✕', degraded: '!', unknown: '?' };

// U8: status = ikon + teks + warna (tidak pernah warna saja).
export function StatusBadge({ state, label }: { state: StatusKind; label?: string }) {
    const { t } = useTranslation();
    const text = label ?? t(`status.${state}`);
    return (
        <span role="status" aria-label={text} className={`badge badge-${state}`}>
            <span aria-hidden="true">{GLYPH[state]}</span> {text}
        </span>
    );
}
