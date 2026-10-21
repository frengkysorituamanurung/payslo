import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    useAddNote,
    useDecideApproval,
    useIncidentDetail,
    useIncidents,
    usePostmortem,
    useRequestApproval,
    useRunStep,
    useSavePostmortem,
    useSaveSettings,
    useSettings,
    type Approval,
    type IncidentDetail,
    type Postmortem,
    type RunbookStep,
} from '../hooks/useIncidents';
import { formatLocal, formatRelative } from '../utils/time';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { EmptyState, ErrorState } from '../components/States';

function Card({ title, children }: { title: string; children: React.ReactNode }) {
    return (
        <section aria-label={title} className="panel">
            <h2 style={{ marginTop: 0 }}>{title}</h2>
            {children}
        </section>
    );
}

// U8: status = ikon + teks + warna.
function IncidentBadge({ status }: { status: string }) {
    const { t } = useTranslation();
    const map: Record<string, { glyph: string; fg: string; bg: string }> = {
        open: { glyph: '✕', fg: 'var(--crit)', bg: 'var(--crit-bg)' },
        acknowledged: { glyph: '!', fg: 'var(--warn)', bg: 'var(--warn-bg)' },
        resolved: { glyph: '✓', fg: 'var(--ok)', bg: 'var(--ok-bg)' },
    };
    const m = map[status] ?? { glyph: '?', fg: 'var(--neutral)', bg: 'var(--neutral-bg)' };
    return (
        <span
            role="status"
            style={{
                background: m.bg,
                color: m.fg,
                borderRadius: 'var(--radius)',
                padding: '2px 8px',
                fontWeight: 600,
            }}
        >
            <span aria-hidden="true">{m.glyph}</span> {t(`incident.status.${status}`, status)}
        </span>
    );
}

const KIND_GLYPH: Record<string, string> = {
    opened: '＋',
    reopened: '↻',
    resolved: '✓',
    note: '✎',
    approval_requested: '?',
    approval_approved: '✓',
    approval_rejected: '✕',
};

function TimelineList({ detail }: { detail: IncidentDetail }) {
    const { t, i18n } = useTranslation();
    return (
        <ol style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: 10 }}>
            {detail.timeline.map((e, i) => (
                <li key={`${e.ts}-${String(i)}`} style={{ display: 'flex', gap: 8 }}>
                    <span aria-hidden="true">{KIND_GLYPH[e.kind] ?? '·'}</span>
                    <div>
                        <strong>{t(`incident.kind.${e.kind}`, e.kind)}</strong>
                        <p style={{ margin: 0 }}>{e.message}</p>
                        <p style={{ margin: 0, color: 'var(--text-muted)', fontSize: 12 }}>
                            {e.actor} ·{' '}
                            <time dateTime={e.ts} title={e.ts}>
                                {formatLocal(e.ts, i18n.language)} ·{' '}
                                {formatRelative(e.ts, i18n.language)}
                            </time>
                        </p>
                    </div>
                </li>
            ))}
        </ol>
    );
}

// Render nilai payload apa adanya sebagai TEKS (React escape default).
// DILARANG dangerouslySetInnerHTML di sini (uji XSS).
function PayloadValue({ value }: { value: unknown }) {
    if (value !== null && typeof value === 'object') {
        const entries = Object.entries(value as Record<string, unknown>);
        return (
            <dl
                style={{
                    margin: 0,
                    display: 'grid',
                    gridTemplateColumns: 'auto 1fr',
                    gap: '2px 8px',
                }}
            >
                {entries.map(([k, v]) => (
                    <div key={k} style={{ display: 'contents' }}>
                        <dt style={{ color: 'var(--text-muted)' }}>{k}</dt>
                        <dd style={{ margin: 0 }}>
                            <PayloadValue value={v} />
                        </dd>
                    </div>
                ))}
            </dl>
        );
    }
    if (typeof value === 'string') return <span>{value}</span>;
    if (typeof value === 'number' || typeof value === 'boolean')
        return <span>{String(value)}</span>;
    if (value === null || value === undefined) return <span />;
    return <span>{JSON.stringify(value)}</span>;
}

function AlertsPanel({ detail }: { detail: IncidentDetail }) {
    const { t } = useTranslation();
    if (detail.alerts.length === 0)
        return <p style={{ color: 'var(--text-muted)' }}>{t('incident.alertsEmpty')}</p>;
    return (
        <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: 8 }}>
            {detail.alerts.map((a) => (
                <li
                    key={a.fingerprint}
                    style={{
                        border: '1px solid var(--border)',
                        borderRadius: 'var(--radius)',
                        padding: 8,
                    }}
                >
                    <p style={{ margin: 0, fontWeight: 700 }}>
                        {a.status === 'firing' ? '✕' : '✓'} {a.source}, {a.severity}, {a.status}
                    </p>
                    <PayloadValue value={a.payload} />
                </li>
            ))}
        </ul>
    );
}

function DiagnosticRunner({ incidentId, step }: { incidentId: string; step: RunbookStep }) {
    const { t } = useTranslation();
    const run = useRunStep();
    return (
        <div>
            <button
                type="button"
                onClick={() => {
                    run.mutate({ id: incidentId, idx: step.idx });
                }}
                disabled={run.isPending}
            >
                {run.isPending ? t('incident.running') : t('incident.runDiagnostic')}
            </button>
            {run.isError && (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {run.error.detail}
                </p>
            )}
            {run.isSuccess && (
                <div role="status" style={{ marginTop: 8 }}>
                    <p style={{ margin: 0, fontWeight: 700 }}>
                        {run.data.status === 'ok' ? '✓' : '!'} {run.data.status}
                        {run.data.detail !== undefined && run.data.detail !== ''
                            ? ` — ${run.data.detail}`
                            : ''}
                    </p>
                    {run.data.values !== undefined && (
                        <pre
                            style={{
                                background: 'var(--neutral-bg)',
                                padding: 8,
                                overflowX: 'auto',
                            }}
                        >
                            {JSON.stringify(run.data.values, null, 2)}
                        </pre>
                    )}
                </div>
            )}
        </div>
    );
}

function ApprovalCard({ incidentId, step }: { incidentId: string; step: RunbookStep }) {
    const { t } = useTranslation();
    const settings = useSettings();
    const request = useRequestApproval();
    const decide = useDecideApproval();
    const [approver, setApprover] = useState<string | null>(null);
    const [reason, setReason] = useState('');
    const [confirmApprove, setConfirmApprove] = useState(false);
    const [reasonError, setReasonError] = useState(false);

    const name = approver ?? settings.data?.traineeName ?? '';
    const approval: Approval | undefined = step.approval;

    function setName(v: string): void {
        setApprover(v);
    }

    if (approval === undefined) {
        return (
            <div>
                <p style={{ color: 'var(--text-muted)', fontSize: 12 }}>
                    {t('incident.noApprovalYet')}
                </p>
                <button
                    type="button"
                    onClick={() => {
                        request.mutate({ id: incidentId, action: step.action, step: step.idx });
                    }}
                    disabled={request.isPending}
                >
                    {request.isPending ? t('incident.requesting') : t('incident.requestApproval')}
                </button>
                {request.isError && (
                    <p role="alert" style={{ color: 'var(--crit)' }}>
                        {request.error.detail}
                    </p>
                )}
            </div>
        );
    }

    if (approval.status !== 'pending') {
        return (
            <p role="status">
                {approval.status === 'approved' ? '✓' : '✕'}{' '}
                {t('incident.decided', {
                    status: t(`incident.approval.${approval.status}`, approval.status),
                    by: approval.decidedBy ?? '?',
                    reason: approval.reason ?? '',
                })}
            </p>
        );
    }

    return (
        <div
            style={{ border: '1px solid var(--warn)', borderRadius: 'var(--radius)', padding: 12 }}
        >
            <p style={{ marginTop: 0 }}>
                <strong>{t('incident.pendingApproval')}</strong>
            </p>
            <dl style={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '4px 12px' }}>
                <dt>{t('incident.command')}</dt>
                <dd>{approval.action}</dd>
                <dt>{t('incident.impact')}</dt>
                <dd>{step.impact}</dd>
            </dl>
            <div style={{ display: 'grid', gap: 8, maxWidth: 420 }}>
                <label>
                    {t('incident.approver')}
                    <input
                        value={name}
                        onChange={(e) => {
                            setName(e.target.value);
                        }}
                        aria-invalid={name.trim() === ''}
                    />
                </label>
                <label>
                    {t('incident.reason')}
                    <input
                        value={reason}
                        onChange={(e) => {
                            setReason(e.target.value);
                            setReasonError(false);
                        }}
                        placeholder={t('incident.reasonHint')}
                    />
                </label>
                {reasonError && (
                    <span role="alert" style={{ color: 'var(--crit)' }}>
                        {t('incident.reasonRequired')}
                    </span>
                )}
            </div>
            <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
                <button
                    type="button"
                    onClick={() => {
                        setConfirmApprove(true);
                    }}
                    disabled={decide.isPending || name.trim() === ''}
                    className="btn-primary"
                >
                    {t('incident.approve')}
                </button>
                <button
                    type="button"
                    onClick={() => {
                        if (reason.trim() === '') {
                            setReasonError(true);
                            return;
                        }
                        decide.mutate({
                            aid: approval.id,
                            incidentId,
                            decision: 'rejected',
                            by: name.trim(),
                            reason: reason.trim(),
                        });
                    }}
                    disabled={decide.isPending || name.trim() === ''}
                    className="btn-danger"
                >
                    {t('incident.reject')}
                </button>
            </div>
            {decide.isError && (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {decide.error.detail}
                </p>
            )}
            <ConfirmDialog
                target={approval.action}
                open={confirmApprove}
                onOpenChange={setConfirmApprove}
                onConfirm={() => {
                    decide.mutate({
                        aid: approval.id,
                        incidentId,
                        decision: 'approved',
                        by: name.trim(),
                    });
                }}
            />
        </div>
    );
}

function RunbookPanel({ detail }: { detail: IncidentDetail }) {
    const { t } = useTranslation();
    if (!detail.runbook.present) {
        return <p style={{ color: 'var(--text-muted)' }}>{t('incident.noRunbook')}</p>;
    }
    return (
        <div style={{ display: 'grid', gap: 12 }}>
            {detail.runbook.steps.map((step) => (
                <section
                    key={step.idx}
                    aria-label={step.name}
                    style={{
                        border: '1px solid var(--border)',
                        borderRadius: 'var(--radius)',
                        padding: 12,
                    }}
                >
                    <h3 style={{ margin: '0 0 4px' }}>
                        {step.kind === 'remediation' ? '⛔' : step.kind === 'manual' ? '✎' : '▶'}{' '}
                        {step.name}
                    </h3>
                    <p style={{ margin: '0 0 8px', color: 'var(--text-muted)', fontSize: 12 }}>
                        {t(`incident.step.${step.kind}`, step.kind)}
                    </p>
                    {step.kind === 'diagnostic' && (
                        <DiagnosticRunner incidentId={detail.incident.id} step={step} />
                    )}
                    {step.kind === 'manual' && step.detail !== undefined && step.detail !== '' && (
                        <p>{step.detail}</p>
                    )}
                    {step.kind === 'remediation' && (
                        <ApprovalCard incidentId={detail.incident.id} step={step} />
                    )}
                </section>
            ))}
        </div>
    );
}

function NotesPanel({ detail }: { detail: IncidentDetail }) {
    const { t } = useTranslation();
    const [text, setText] = useState('');
    const add = useAddNote();
    return (
        <div>
            <form
                onSubmit={(e) => {
                    e.preventDefault();
                    if (text.trim() === '') return;
                    add.mutate(
                        { id: detail.incident.id, text: text.trim() },
                        {
                            onSuccess: () => {
                                setText('');
                            },
                        },
                    );
                }}
            >
                <label>
                    {t('incident.noteLabel')}
                    <textarea
                        rows={3}
                        style={{ width: '100%' }}
                        value={text}
                        onChange={(e) => {
                            setText(e.target.value);
                        }}
                    />
                </label>
                <div style={{ marginTop: 8 }}>
                    <button type="submit" disabled={add.isPending || text.trim() === ''}>
                        {add.isPending ? t('incident.saving') : t('incident.addNote')}
                    </button>
                </div>
            </form>
            {add.isError && (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {add.error.detail}
                </p>
            )}
        </div>
    );
}

const PM_FIELDS = ['summary', 'impact', 'timeline', 'rootCause', 'wentWell', 'wentWrong'] as const;

function PostmortemPanel({ incidentId }: { incidentId: string }) {
    const { t } = useTranslation();
    const pm = usePostmortem(incidentId);
    const save = useSavePostmortem();
    const [draft, setDraft] = useState<Postmortem['sections'] | null>(null);
    const [status, setStatus] = useState<'draft' | 'published'>('draft');
    const [actionsText, setActionsText] = useState<string | null>(null);

    if (pm.isPending) return <p>{t('common.loading')}</p>;
    if (pm.isError) return <ErrorState onRetry={() => void pm.refetch()} />;
    const shown = draft ?? pm.data.sections;
    const shownActions = actionsText ?? (shown.actions ?? []).join('\n');

    function field(key: (typeof PM_FIELDS)[number], value: string): void {
        setDraft({ ...shown, [key]: value });
    }

    return (
        <div>
            <div style={{ display: 'grid', gap: 8 }}>
                {PM_FIELDS.map((key) => (
                    <label key={key}>
                        {t(`incident.pm.${key}`)}
                        <textarea
                            rows={3}
                            style={{ width: '100%' }}
                            value={shown[key] ?? ''}
                            onChange={(e) => {
                                field(key, e.target.value);
                            }}
                        />
                    </label>
                ))}
                <label>
                    {t('incident.pm.actions')}
                    <textarea
                        rows={3}
                        style={{ width: '100%' }}
                        value={shownActions}
                        onChange={(e) => {
                            setActionsText(e.target.value);
                        }}
                    />
                </label>
                <label>
                    {t('incident.pm.status')}
                    <select
                        value={status}
                        onChange={(e) => {
                            setStatus(e.target.value === 'published' ? 'published' : 'draft');
                        }}
                    >
                        <option value="draft">{t('incident.pm.draft')}</option>
                        <option value="published">{t('incident.pm.published')}</option>
                    </select>
                </label>
            </div>
            <div style={{ display: 'flex', gap: 8, marginTop: 8, flexWrap: 'wrap' }}>
                <button
                    type="button"
                    onClick={() => {
                        save.mutate({
                            id: incidentId,
                            status,
                            sections: {
                                ...shown,
                                actions: shownActions
                                    .split('\n')
                                    .map((a) => a.trim())
                                    .filter((a) => a !== ''),
                            },
                        });
                    }}
                    disabled={save.isPending}
                >
                    {save.isPending ? t('incident.saving') : t('incident.pm.save')}
                </button>
                <button
                    type="button"
                    onClick={() => {
                        const blob = new Blob([pm.data.markdown], { type: 'text/markdown' });
                        const url = URL.createObjectURL(blob);
                        const a = document.createElement('a');
                        a.href = url;
                        a.download = `postmortem-${incidentId}.md`;
                        document.body.appendChild(a);
                        a.click();
                        a.remove();
                        URL.revokeObjectURL(url);
                    }}
                >
                    {t('incident.pm.export')}
                </button>
            </div>
            {save.isError && (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {save.error.detail}
                </p>
            )}
            {save.isSuccess && <p role="status">{t('incident.pm.saved')}</p>}
            <h3>{t('incident.pm.preview')}</h3>
            <pre
                style={{
                    background: 'var(--neutral-bg)',
                    padding: 12,
                    overflowX: 'auto',
                    whiteSpace: 'pre-wrap',
                }}
            >
                {pm.data.markdown}
            </pre>
        </div>
    );
}

function SettingsPanel() {
    const { t } = useTranslation();
    const settings = useSettings();
    const save = useSaveSettings();
    const [name, setName] = useState<string | null>(null);
    const shown = name ?? settings.data?.traineeName ?? '';
    return (
        <Card title={t('incident.settingsTitle')}>
            <p style={{ color: 'var(--text-muted)', fontSize: 12 }}>{t('incident.settingsHint')}</p>
            {settings.isPending && <p>{t('common.loading')}</p>}
            {settings.isError && <ErrorState onRetry={() => void settings.refetch()} />}
            {settings.isSuccess && (
                <div style={{ display: 'flex', gap: 8, alignItems: 'end', flexWrap: 'wrap' }}>
                    <label>
                        {t('incident.traineeName')}
                        <input
                            value={shown}
                            onChange={(e) => {
                                setName(e.target.value);
                            }}
                        />
                    </label>
                    <button
                        type="button"
                        className="btn-primary"
                        onClick={() => {
                            save.mutate(
                                { traineeName: shown.trim() },
                                {
                                    onSuccess: () => {
                                        setName(null);
                                    },
                                },
                            );
                        }}
                        disabled={save.isPending || shown.trim() === ''}
                    >
                        {t('incident.saveName')}
                    </button>
                </div>
            )}
            {save.isError && (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {save.error.detail}
                </p>
            )}
            {save.isSuccess && <p role="status">{t('incident.nameSaved')}</p>}
        </Card>
    );
}

export function IncidentsPage() {
    const { t, i18n } = useTranslation();
    const [selected, setSelected] = useState<string | null>(null);
    const [statusFilter, setStatusFilter] = useState<'open' | 'resolved' | ''>('');
    const list = useIncidents();
    const detail = useIncidentDetail(selected);

    const items = (list.data ?? []).filter((i) => statusFilter === '' || i.status === statusFilter);

    return (
        <section className="page" aria-labelledby="incidents-title">
            <h1 id="incidents-title">{t('nav.incidents')}</h1>
            <div className="split-layout">
                <div style={{ display: 'grid', gap: 16 }}>
                    <Card title={t('incident.listTitle')}>
                        <label>
                            {t('incident.filterStatus')}
                            <select
                                value={statusFilter}
                                onChange={(e) => {
                                    setStatusFilter(e.target.value as '' | 'open' | 'resolved');
                                }}
                            >
                                <option value="">{t('incident.allStatus')}</option>
                                <option value="open">{t('incident.status.open')}</option>
                                <option value="resolved">{t('incident.status.resolved')}</option>
                            </select>
                        </label>
                        {list.isPending && <p>{t('common.loading')}</p>}
                        {list.isError && <ErrorState onRetry={() => void list.refetch()} />}
                        {list.isSuccess && items.length === 0 && (
                            <EmptyState next={t('incident.emptyNext')} />
                        )}
                        <ul
                            style={{
                                listStyle: 'none',
                                padding: 0,
                                margin: '12px 0 0',
                                display: 'grid',
                                gap: 8,
                            }}
                        >
                            {items.map((i) => (
                                <li key={i.id}>
                                    <button
                                        type="button"
                                        onClick={() => {
                                            setSelected(i.id);
                                        }}
                                        aria-current={selected === i.id}
                                        className={
                                            selected === i.id ? 'list-item active' : 'list-item'
                                        }
                                    >
                                        <strong>{i.title}</strong>
                                        <span className="item-meta">
                                            {i.slo}, {i.service}, {i.severity},{' '}
                                            <time dateTime={i.createdAt} title={i.createdAt}>
                                                {formatRelative(i.createdAt, i18n.language)}
                                            </time>
                                        </span>
                                        <span className="item-state">
                                            <IncidentBadge status={i.status} />
                                        </span>
                                    </button>
                                </li>
                            ))}
                        </ul>
                    </Card>
                    <SettingsPanel />
                </div>
                <div style={{ display: 'grid', gap: 16 }}>
                    {selected === null && <EmptyState next={t('incident.pickNext')} />}
                    {selected !== null && detail.isPending && <p>{t('common.loading')}</p>}
                    {selected !== null && detail.isError && (
                        <ErrorState onRetry={() => void detail.refetch()} />
                    )}
                    {selected !== null && detail.isSuccess && (
                        <>
                            <Card
                                title={t('incident.detailTitle', {
                                    title: detail.data.incident.title,
                                })}
                            >
                                <p>
                                    <IncidentBadge status={detail.data.incident.status} />{' '}
                                    <time dateTime={detail.data.incident.createdAt}>
                                        {formatLocal(detail.data.incident.createdAt, i18n.language)}
                                    </time>
                                </p>
                                <h3>{t('incident.timeline')}</h3>
                                <TimelineList detail={detail.data} />
                            </Card>
                            <Card title={t('incident.alertsTitle')}>
                                <AlertsPanel detail={detail.data} />
                            </Card>
                            <Card title={t('incident.runbookTitle')}>
                                <RunbookPanel detail={detail.data} />
                            </Card>
                            <Card title={t('incident.notesTitle')}>
                                <NotesPanel detail={detail.data} />
                            </Card>
                            <Card title={t('incident.pm.title')}>
                                <PostmortemPanel incidentId={selected} />
                            </Card>
                        </>
                    )}
                </div>
            </div>
        </section>
    );
}
