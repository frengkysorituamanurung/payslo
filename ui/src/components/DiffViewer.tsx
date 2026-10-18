import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { diffLines, type Change } from 'diff';
import { TableScroll } from './TableScroll';

type Row = { kind: 'same'; no: string; text: string } | { kind: 'added' | 'removed'; text: string };

// DiffViewer baris actual -> desired (dari `payslo plan` backend prometheus).
// Render tabel dua kolom + ringkasan; baris berubah memakai ikon -/+ dan teks
// (tidak pernah warna saja, U8). Diff dihitung di klien dari teks yang
// disuplai backend (jujur: bila teks kosong, tampilkan catatan backend).
export function DiffViewer({
    actual,
    desired,
    note,
}: {
    actual: string;
    desired: string;
    note?: string;
}) {
    const { t } = useTranslation();
    const { parts, rows } = useMemo(() => {
        const ps: Change[] = actual === '' && desired === '' ? [] : diffLines(actual, desired);
        const rs: Row[] = [];
        let no = 0;
        for (const part of ps) {
            const lines = part.value.split('\n');
            if (lines.length > 0 && lines[lines.length - 1] === '') lines.pop();
            for (const text of lines) {
                if (!part.added && !part.removed) {
                    no += 1;
                    rs.push({ kind: 'same', no: String(no), text });
                } else {
                    rs.push({ kind: part.added ? 'added' : 'removed', text });
                }
            }
        }
        return { parts: ps, rows: rs };
    }, [actual, desired]);
    const added = parts.filter((p) => p.added).length;
    const removed = parts.filter((p) => p.removed).length;

    if (parts.length === 0) {
        return <p style={{ color: 'var(--text-muted)' }}>{note ?? t('slo.noDiff')}</p>;
    }

    return (
        <div>
            <p role="status">
                {t('slo.diffSummary', { added, removed })}
                {note !== undefined && note !== '' ? ` — ${note}` : ''}
            </p>
            <TableScroll label={t('slo.diffCaption')}>
                <table
                    style={{
                        borderCollapse: 'collapse',
                        width: '100%',
                        fontFamily: 'monospace',
                        fontSize: 12,
                    }}
                >
                    <caption style={{ textAlign: 'start', color: 'var(--text-muted)' }}>
                        {t('slo.diffCaption')}
                    </caption>
                    <tbody>
                        {rows.map((row, i) => {
                            const key = `r${String(i)}`;
                            if (row.kind === 'same') {
                                return (
                                    <tr key={key}>
                                        <td style={{ width: 32, color: 'var(--text-muted)' }}>
                                            {row.no}
                                        </td>
                                        <td style={{ width: 24 }} aria-hidden="true">
                                            {' '}
                                        </td>
                                        <td>{row.text}</td>
                                    </tr>
                                );
                            }
                            const isAdded = row.kind === 'added';
                            const bg = isAdded ? 'var(--ok-bg)' : 'var(--crit-bg)';
                            const fg = isAdded ? 'var(--ok)' : 'var(--crit)';
                            return (
                                <tr key={key} style={{ background: bg, color: fg }}>
                                    <td style={{ width: 32 }}>…</td>
                                    <td style={{ width: 24 }} aria-hidden="true">
                                        {isAdded ? '+' : '-'}
                                    </td>
                                    <td>{row.text}</td>
                                </tr>
                            );
                        })}
                    </tbody>
                </table>
            </TableScroll>
        </div>
    );
}
