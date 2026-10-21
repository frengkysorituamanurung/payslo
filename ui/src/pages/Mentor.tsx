import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    useMentorAssessment,
    useMentorAssessmentPut,
    useMentorInjectTraffic,
    useMentorLock,
    useMentorSessionSummary,
    useMentorSessions,
    useMentorTimeline,
    useSnapshotLoad,
    useSnapshotSave,
    useSnapshots,
    type MentorTimelineEntry,
    type SnapshotInfo,
} from '../hooks/useMentor';
import { formatLocal, formatRelative } from '../utils/time';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { EmptyState, ErrorState } from '../components/States';
import { useSession } from '../hooks/useLab';

// Unduh laporan via fetch (bukan <a href>) agar header token ikut terkirim.
async function downloadReport(scenario: string, format: 'markdown' | 'json'): Promise<void> {
    const token = typeof window !== 'undefined' ? (window.__LAB_TOKEN__ ?? '') : '';
    const res = await fetch(
        `/api/v1/mentor/export?scenario=${encodeURIComponent(scenario)}&format=${format}`,
        {
            headers: { 'X-Mentor-Token': token },
        },
    );
    if (!res.ok) return;
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `mentor-report-${scenario}.${format}`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
}

function Card({ title, children }: { title: string; children: React.ReactNode }) {
    return (
        <section aria-label={title} className="panel">
            <h2 style={{ marginTop: 0 }}>{title}</h2>
            {children}
        </section>
    );
}

// Sumber badge: payslo vs lab.
function SourceBadge({ source }: { source: string }) {
    const { t } = useTranslation();
    const isPayslo = source === 'payslo';
    return (
        <span
            style={{
                background: isPayslo ? 'var(--info-bg)' : 'var(--neutral-bg)',
                color: isPayslo ? 'var(--info)' : 'var(--neutral)',
                borderRadius: 'var(--radius)',
                padding: '1px 6px',
                fontSize: 11,
                fontWeight: 700,
            }}
        >
            {isPayslo ? t('mentor.sourcePayslo') : t('mentor.sourceLab')}
        </span>
    );
}

function TimelineView({ entries }: { entries: MentorTimelineEntry[] }) {
    const { t, i18n } = useTranslation();
    if (entries.length === 0)
        return <p style={{ color: 'var(--text-muted)' }}>{t('mentor.timelineEmpty')}</p>;
    return (
        <ol style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: 10 }}>
            {entries.map((e, i) => (
                <li key={`${e.ts}-${String(i)}`} style={{ display: 'flex', gap: 8 }}>
                    <SourceBadge source={e.source} />
                    <div>
                        <strong>{e.action}</strong>
                        {e.detail !== '' && (
                            <span style={{ color: 'var(--text-muted)' }}> — {e.detail}</span>
                        )}
                        <p style={{ margin: 0, color: 'var(--text-muted)', fontSize: 12 }}>
                            {e.actor} ·{' '}
                            <time dateTime={e.ts} title={e.ts}>
                                {formatLocal(e.ts, i18n.language)}
                            </time>
                        </p>
                    </div>
                </li>
            ))}
        </ol>
    );
}

function SnapshotPanel() {
    const { t, i18n } = useTranslation();
    const snapshots = useSnapshots();
    const save = useSnapshotSave();
    const load = useSnapshotLoad();
    const [name, setName] = useState('');
    const [pendingLoad, setPendingLoad] = useState<SnapshotInfo | null>(null);

    return (
        <Card title={t('mentor.snapshotTitle')}>
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'end' }}>
                <label>
                    {t('mentor.snapshotName')}
                    <input
                        value={name}
                        onChange={(e) => {
                            setName(e.target.value);
                        }}
                        placeholder={t('mentor.snapshotPlaceholder')}
                    />
                </label>
                <button
                    type="button"
                    disabled={save.isPending || name.trim() === ''}
                    onClick={() => {
                        save.mutate(name.trim());
                    }}
                >
                    {save.isPending ? t('common.loading') : t('mentor.snapshotSave')}
                </button>
            </div>
            {save.isError && (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {save.error.detail}
                </p>
            )}
            {save.isSuccess && (
                <p role="status">{t('mentor.snapshotSaved', { name: save.data.name })}</p>
            )}
            {snapshots.isPending && <p>{t('common.loading')}</p>}
            {snapshots.isError && <ErrorState onRetry={() => void snapshots.refetch()} />}
            {snapshots.isSuccess && (
                <div style={{ marginTop: 8 }}>
                    {snapshots.data.length === 0 ? (
                        <p style={{ color: 'var(--text-muted)' }}>{t('mentor.snapshotEmpty')}</p>
                    ) : (
                        <ul
                            style={{
                                listStyle: 'none',
                                padding: 0,
                                margin: 0,
                                display: 'grid',
                                gap: 6,
                            }}
                        >
                            {snapshots.data.map((s) => (
                                <li
                                    key={s.name}
                                    style={{ display: 'flex', gap: 8, alignItems: 'center' }}
                                >
                                    <span style={{ fontWeight: 600 }}>{s.name}</span>
                                    <span style={{ color: 'var(--text-muted)', fontSize: 12 }}>
                                        {s.dataTables} {t('mentor.tables')} ·{' '}
                                        <time dateTime={s.savedAt}>
                                            {formatRelative(s.savedAt, i18n.language)}
                                        </time>
                                    </span>
                                    <button
                                        type="button"
                                        onClick={() => {
                                            setPendingLoad(s);
                                        }}
                                        style={{ marginLeft: 'auto' }}
                                    >
                                        {t('mentor.snapshotLoad')}
                                    </button>
                                </li>
                            ))}
                        </ul>
                    )}
                </div>
            )}
            <ConfirmDialog
                target={pendingLoad?.name ?? ''}
                open={pendingLoad !== null}
                onOpenChange={(open) => {
                    if (!open) setPendingLoad(null);
                }}
                onConfirm={() => {
                    if (pendingLoad !== null) load.mutate(pendingLoad.name);
                    setPendingLoad(null);
                }}
            />
            {load.isError && (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {load.error.detail}
                </p>
            )}
            {load.isSuccess && (
                <p role="status">
                    {t('mentor.snapshotLoaded', { restored: load.data.restored.join(', ') })}
                </p>
            )}
        </Card>
    );
}

function InjectPanel() {
    const { t } = useTranslation();
    const inject = useMentorInjectTraffic();
    const [rate, setRate] = useState('6');
    const [secs, setSecs] = useState('');
    const [confirm, setConfirm] = useState(false);
    const parsed = Number.parseInt(rate, 10);
    const rateOK = Number.isInteger(parsed) && parsed >= 1 && parsed <= 100;

    return (
        <Card title={t('mentor.injectTitle')}>
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'end' }}>
                <label>
                    {t('mentor.injectRate')}
                    <input
                        value={rate}
                        onChange={(e) => {
                            setRate(e.target.value);
                        }}
                        inputMode="numeric"
                        style={{ width: 80 }}
                    />
                </label>
                <label>
                    {t('mentor.injectSecs')}
                    <input
                        value={secs}
                        onChange={(e) => {
                            setSecs(e.target.value);
                        }}
                        placeholder={t('mentor.injectSecsHint')}
                        inputMode="numeric"
                        style={{ width: 80 }}
                    />
                </label>
                <button
                    type="button"
                    disabled={inject.isPending || !rateOK}
                    onClick={() => {
                        setConfirm(true);
                    }}
                >
                    {t('mentor.injectButton')}
                </button>
            </div>
            <p style={{ color: 'var(--text-muted)', fontSize: 12 }}>
                {t('mentor.injectConfirmHint')}
            </p>
            <ConfirmDialog
                target={t('mentor.injectConfirmTarget', { rate })}
                open={confirm}
                onOpenChange={setConfirm}
                onConfirm={() => {
                    inject.mutate({
                        ratePerSec: parsed,
                        ...(secs.trim() === '' ? {} : { secs: Number.parseInt(secs, 10) }),
                    });
                    setConfirm(false);
                }}
            />
            {inject.isError && (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {inject.error.detail}
                </p>
            )}
            {inject.isSuccess && (
                <p role="status">
                    {t('mentor.injectDone', { rate: inject.data.ratePerSec ?? '?' })}
                </p>
            )}
        </Card>
    );
}

function scorePlaceholder(score: number | null | undefined): string {
    return score === undefined || score === null ? '0-10' : String(score);
}

function SessionPanel({ scenario }: { scenario: string }) {
    const { t } = useTranslation();
    const summary = useMentorSessionSummary(scenario);
    const assessment = useMentorAssessment(scenario);
    const putAssessment = useMentorAssessmentPut();
    const [commScore, setCommScore] = useState('');
    const [commNote, setCommNote] = useState('');
    const [pmScore, setPmScore] = useState('');
    const [pmNote, setPmNote] = useState('');

    if (summary.isPending) return <p>{t('common.loading')}</p>;
    if (summary.isError) return <ErrorState onRetry={() => void summary.refetch()} />;
    const s = summary.data;
    const existing = assessment.data;

    return (
        <Card title={t('mentor.sessionTitle', { id: scenario })}>
            <dl style={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '4px 12px' }}>
                <dt>{t('mentor.ttd')}</dt>
                <dd style={{ margin: 0 }}>{s.ttdSecs !== null ? `${String(s.ttdSecs)}s` : '-'}</dd>
                <dt>{t('mentor.actionCount')}</dt>
                <dd style={{ margin: 0 }}>{s.actionCount}</dd>
                <dt>{t('mentor.answerCount')}</dt>
                <dd style={{ margin: 0 }}>{s.answerCount}</dd>
                <dt>{t('mentor.hintsUsed')}</dt>
                <dd style={{ margin: 0 }}>{s.hintsUsed}</dd>
            </dl>
            <h3>{t('mentor.answers')}</h3>
            {Object.keys(s.answers).length === 0 ? (
                <p style={{ color: 'var(--text-muted)' }}>{t('mentor.answersEmpty')}</p>
            ) : (
                <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: 6 }}>
                    {Object.entries(s.answers).map(([q, a]) => (
                        <li key={q}>
                            <strong>{q}</strong>
                            <p style={{ margin: 0, color: 'var(--text-muted)' }}>{a}</p>
                        </li>
                    ))}
                </ul>
            )}
            <h3>{t('mentor.assessmentTitle')}</h3>
            <div style={{ display: 'grid', gap: 8, maxWidth: 480 }}>
                <label>
                    {t('mentor.commScore')}
                    <input
                        type="number"
                        min={0}
                        max={10}
                        value={commScore}
                        onChange={(e) => {
                            setCommScore(e.target.value);
                        }}
                        placeholder={scorePlaceholder(existing?.communicationScore)}
                    />
                </label>
                <label>
                    {t('mentor.commNote')}
                    <textarea
                        rows={2}
                        style={{ width: '100%' }}
                        value={commNote}
                        onChange={(e) => {
                            setCommNote(e.target.value);
                        }}
                    />
                </label>
                <label>
                    {t('mentor.pmScore')}
                    <input
                        type="number"
                        min={0}
                        max={10}
                        value={pmScore}
                        onChange={(e) => {
                            setPmScore(e.target.value);
                        }}
                        placeholder={scorePlaceholder(existing?.postmortemScore)}
                    />
                </label>
                <label>
                    {t('mentor.pmNote')}
                    <textarea
                        rows={2}
                        style={{ width: '100%' }}
                        value={pmNote}
                        onChange={(e) => {
                            setPmNote(e.target.value);
                        }}
                    />
                </label>
                <div>
                    <button
                        type="button"
                        disabled={putAssessment.isPending}
                        onClick={() => {
                            const cs =
                                commScore === ''
                                    ? (existing?.communicationScore ?? null)
                                    : Number.parseInt(commScore, 10);
                            const ps =
                                pmScore === ''
                                    ? (existing?.postmortemScore ?? null)
                                    : Number.parseInt(pmScore, 10);
                            putAssessment.mutate({
                                scenario,
                                communicationScore: cs === null ? undefined : cs,
                                communicationNote: commNote,
                                postmortemScore: ps === null ? undefined : ps,
                                postmortemNote: pmNote,
                            });
                        }}
                    >
                        {t('mentor.assessmentSave')}
                    </button>
                </div>
                {putAssessment.isError && (
                    <p role="alert" style={{ color: 'var(--crit)' }}>
                        {putAssessment.error.detail}
                    </p>
                )}
                {putAssessment.isSuccess && <p role="status">{t('mentor.assessmentSaved')}</p>}
            </div>
        </Card>
    );
}

// Halaman utama mentor. Jika mode bukan mentor, tampilkan 403.
export function MentorPage() {
    const { t, i18n } = useTranslation();
    const session = useSession();
    const timeline = useMentorTimeline();
    const sessions = useMentorSessions();
    const lock = useMentorLock();
    const [selectedSession, setSelectedSession] = useState<string | null>(null);

    // Mode trainee: 403 (sesuai acceptance: UI tidak menampilkan menu + 403).
    if (session.data?.mode !== 'mentor') {
        return (
            <section className="page" aria-labelledby="mentor-forbidden">
                <h1 id="mentor-forbidden">{t('nav.mentor')}</h1>
                <div
                    role="alert"
                    style={{
                        border: '1px solid var(--warn)',
                        borderRadius: 'var(--radius)',
                        padding: 16,
                    }}
                >
                    <p style={{ marginTop: 0 }}>{t('mentor.forbidden')}</p>
                    <p style={{ color: 'var(--text-muted)', fontSize: 12 }}>
                        {t('mentor.forbiddenHint')}
                    </p>
                </div>
            </section>
        );
    }

    return (
        <section className="page" aria-labelledby="mentor-title">
            <h1 id="mentor-title">{t('nav.mentor')}</h1>
            <div style={{ display: 'grid', gap: 16, alignItems: 'start' }}>
                <Card title={t('mentor.timelineTitle')}>
                    {timeline.isPending && <p>{t('common.loading')}</p>}
                    {timeline.isError && <ErrorState onRetry={() => void timeline.refetch()} />}
                    {timeline.isSuccess && <TimelineView entries={timeline.data} />}
                </Card>
                <InjectPanel />
                <SnapshotPanel />
                <Card title={t('mentor.sessionsTitle')}>
                    {sessions.isPending && <p>{t('common.loading')}</p>}
                    {sessions.isSuccess && sessions.data.length === 0 && (
                        <EmptyState next={t('mentor.sessionsEmpty')} />
                    )}
                    {sessions.isSuccess && sessions.data.length > 0 && (
                        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                            {sessions.data.map((s) => (
                                <button
                                    key={s.scenarioId}
                                    type="button"
                                    onClick={() => {
                                        setSelectedSession(s.scenarioId);
                                    }}
                                    aria-current={selectedSession === s.scenarioId}
                                    style={{
                                        border: '1px solid var(--border)',
                                        borderRadius: 'var(--radius)',
                                        padding: '6px 12px',
                                        background:
                                            selectedSession === s.scenarioId
                                                ? 'var(--neutral-bg)'
                                                : undefined,
                                    }}
                                >
                                    {s.scenarioId}
                                    <span
                                        style={{
                                            display: 'block',
                                            fontSize: 12,
                                            color: 'var(--text-muted)',
                                        }}
                                    >
                                        <time dateTime={s.startedAt}>
                                            {formatLocal(s.startedAt, i18n.language)}
                                        </time>
                                    </span>
                                </button>
                            ))}
                        </div>
                    )}
                </Card>
                {selectedSession !== null && <SessionPanel scenario={selectedSession} />}
                <Card title={t('mentor.exportTitle')}>
                    {selectedSession === null ? (
                        <p style={{ color: 'var(--text-muted)' }}>{t('mentor.exportPick')}</p>
                    ) : (
                        <div style={{ display: 'flex', gap: 8 }}>
                            <button
                                type="button"
                                onClick={() => void downloadReport(selectedSession, 'markdown')}
                            >
                                {t('mentor.exportMd')}
                            </button>
                            <button
                                type="button"
                                onClick={() => void downloadReport(selectedSession, 'json')}
                            >
                                {t('mentor.exportJson')}
                            </button>
                        </div>
                    )}
                    <div style={{ marginTop: 8 }}>
                        <button
                            type="button"
                            disabled={lock.isPending}
                            onClick={() => {
                                lock.mutate();
                            }}
                        >
                            {t('mentor.exitMentor')}
                        </button>
                    </div>
                    {lock.isError && (
                        <p role="alert" style={{ color: 'var(--crit)' }}>
                            {lock.error.detail}
                        </p>
                    )}
                </Card>
            </div>
        </section>
    );
}
