import { useTranslation } from 'react-i18next';

export type StatusKind = 'up' | 'down' | 'degraded' | 'unknown';

const GLYPH: Record<StatusKind, string> = { up: '✓', down: '✕', degraded: '!', unknown: '?' };

// U8: status = ikon + teks + warna (tidak pernah warna saja).
export function StatusBadge({ state, label }: { state: StatusKind; label?: string }) {
    const { t } = useTranslation();
    const text = label ?? t(`status.${state}`);
    const bg: Record<StatusKind, string> = {
        up: 'var(--ok-bg)',
        down: 'var(--crit-bg)',
        degraded: 'var(--warn-bg)',
        unknown: 'var(--neutral-bg)',
    };
    const fg: Record<StatusKind, string> = {
        up: 'var(--ok)',
        down: 'var(--crit)',
        degraded: 'var(--warn)',
        unknown: 'var(--neutral)',
    };
    return (
        <span
            role="status"
            aria-label={text}
            style={{
                background: bg[state],
                color: fg[state],
                borderRadius: 'var(--radius)',
                padding: '2px 8px',
                fontWeight: 600,
            }}
        >
            <span aria-hidden="true">{GLYPH[state]}</span> {text}
        </span>
    );
}
