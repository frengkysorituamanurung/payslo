import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    useApplySlo,
    useCapabilities,
    useDrift,
    usePlanSlo,
    useSaveSlo,
    useSloDetail,
    useSlos,
    useValidateSlo,
    type SloApply,
    type SloDetail,
    type SloPlan,
    type SloValidation,
} from '../hooks/useSlo';
import { errorBudgetMinutes, formatMinutes, parseDurationSecs } from '../utils/budget';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { DiffViewer } from '../components/DiffViewer';
import { SloEditor } from '../components/SloEditor';
import { EmptyState, ErrorState } from '../components/States';
import { TableScroll } from '../components/TableScroll';

function Card({ title, children }: { title: string; children: React.ReactNode }) {
    return (
        <section
            aria-label={title}
            style={{
                border: '1px solid var(--border)',
                borderRadius: 'var(--radius)',
                padding: 16,
            }}
        >
            <h2 style={{ marginTop: 0 }}>{title}</h2>
            {children}
        </section>
    );
}

function DeployBadge({ status }: { status: string }) {
    const { t } = useTranslation();
    const map: Record<string, { glyph: string; fg: string; bg: string }> = {
        applied: { glyph: '✓', fg: 'var(--ok)', bg: 'var(--ok-bg)' },
        pending: { glyph: '!', fg: 'var(--warn)', bg: 'var(--warn-bg)' },
        drifted: { glyph: '≠', fg: 'var(--crit)', bg: 'var(--crit-bg)' },
        failed: { glyph: '✕', fg: 'var(--crit)', bg: 'var(--crit-bg)' },
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
            <span aria-hidden="true">{m.glyph}</span> {t(`slo.deploy.${status}`, status)}
        </span>
    );
}

function SloListPanel({
    selected,
    onSelect,
}: {
    selected: string | null;
    onSelect: (name: string) => void;
}) {
    const { t } = useTranslation();
    const slos = useSlos();
    if (slos.isPending) return <p>{t('common.loading')}</p>;
    if (slos.isError) return <ErrorState onRetry={() => void slos.refetch()} />;
    if (slos.data.slos.length === 0) return <EmptyState next={t('slo.emptyNext')} />;
    return (
        <Card title={t('slo.listTitle')}>
            <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: 8 }}>
                {slos.data.slos.map((s) => (
                    <li key={s.name}>
                        <button
                            type="button"
                            onClick={() => {
                                onSelect(s.name);
                            }}
                            aria-current={selected === s.name}
                            style={{
                                width: '100%',
                                textAlign: 'start',
                                background: selected === s.name ? 'var(--neutral-bg)' : undefined,
                                border: '1px solid var(--border)',
                                borderRadius: 'var(--radius)',
                                padding: '8px 12px',
                            }}
                        >
                            <strong>{s.name}</strong>
                            <span
                                style={{
                                    display: 'block',
                                    color: 'var(--text-muted)',
                                    fontSize: 12,
                                }}
                            >
                                {s.service} · {t('slo.objectiveLabel')}{' '}
                                {(s.objective * 100).toFixed(2)}% · {s.window}
                                {s.parseError !== undefined && s.parseError !== ''
                                    ? ` · ⚠ ${s.parseError}`
                                    : ''}
                            </span>
                        </button>
                    </li>
                ))}
            </ul>
        </Card>
    );
}

function SloDetailPanel({ detail }: { detail: SloDetail }) {
    const { t, i18n } = useTranslation();
    const m = detail.meta;
    return (
        <Card title={t('slo.detailTitle', { name: m.name })}>
            <dl style={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '4px 12px' }}>
                <dt>{t('slo.service')}</dt>
                <dd>{m.service}</dd>
                <dt>{t('slo.objectiveLabel')}</dt>
                <dd>{(m.objective * 100).toFixed(3)}%</dd>
                <dt>{t('slo.window')}</dt>
                <dd>{m.window}</dd>
                <dt>{t('slo.file')}</dt>
                <dd>{m.file}</dd>
                <dt>{t('slo.runbook')}</dt>
                <dd>{detail.runbook}</dd>
            </dl>

            <h3>{t('slo.budgetTitle')}</h3>
            <p>
                {t('slo.budgetAllowed', {
                    minutes: formatMinutes(detail.budget.allowedMinutes, i18n.language),
                    window: m.window,
                })}
            </p>

            <h3>{t('slo.sliTitle')}</h3>
            {detail.sliCurrent.status === 'ok' ? (
                <p role="status">
                    {t('slo.sliNow', {
                        sli: ((detail.sliCurrent.sliValue ?? 0) * 100).toFixed(2),
                        err: ((detail.sliCurrent.errorRatio ?? 0) * 100).toFixed(3),
                    })}
                </p>
            ) : (
                <p style={{ color: 'var(--text-muted)' }}>
                    {detail.sliCurrent.status === 'source_down'
                        ? t('slo.sliDown')
                        : t('slo.sliNoData')}
                    {detail.sliCurrent.detail !== undefined ? ` — ${detail.sliCurrent.detail}` : ''}
                </p>
            )}
            <p style={{ color: 'var(--text-muted)', fontSize: 12 }}>
                {t('slo.sliQuery', { good: detail.sli.good, total: detail.sli.total })}
            </p>

            <h3>{t('slo.burnTitle')}</h3>
            <TableScroll label={t('slo.burnTitle')}>
                <table style={{ borderCollapse: 'collapse', width: '100%' }}>
                    <thead>
                        <tr>
                            <th scope="col">{t('slo.burnLong')}</th>
                            <th scope="col">{t('slo.burnShort')}</th>
                            <th scope="col">{t('slo.burnRate')}</th>
                            <th scope="col">{t('slo.burnThreshold')}</th>
                            <th scope="col">{t('slo.severity')}</th>
                        </tr>
                    </thead>
                    <tbody>
                        {detail.burnRates.map((b, i) => (
                            <tr key={`b${b.longWindow}-${b.shortWindow}-${String(i)}`}>
                                <td>{b.longWindow}</td>
                                <td>{b.shortWindow}</td>
                                <td>{b.burnRate}×</td>
                                <td>{((1 - m.objective) * b.burnRate * 100).toFixed(3)}%</td>
                                <td>{b.severity}</td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </TableScroll>

            <h3>{t('slo.deployTitle')}</h3>
            <TableScroll label={t('slo.deployTitle')}>
                <table style={{ borderCollapse: 'collapse', width: '100%' }}>
                    <thead>
                        <tr>
                            <th scope="col">{t('slo.connection')}</th>
                            <th scope="col">{t('slo.backend')}</th>
                            <th scope="col">{t('slo.status')}</th>
                        </tr>
                    </thead>
                    <tbody>
                        {detail.deployment.map((d) => (
                            <tr key={d.connection}>
                                <td>{d.connection}</td>
                                <td>{d.backend}</td>
                                <td>
                                    <DeployBadge status={d.status} />
                                    {d.detail !== undefined && d.detail !== '' && (
                                        <span
                                            style={{
                                                display: 'block',
                                                fontSize: 12,
                                                color: 'var(--text-muted)',
                                            }}
                                        >
                                            {d.detail}
                                        </span>
                                    )}
                                </td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </TableScroll>

            <h3>{t('slo.alertsTitle')}</h3>
            {detail.alerts.length === 0 ? (
                <p style={{ color: 'var(--text-muted)' }}>{t('slo.alertsEmpty')}</p>
            ) : (
                <div role="alert">
                    <ul>
                        {detail.alerts.map((a, i) => (
                            <li key={`a${a.alert}-${String(i)}`}>
                                {a.state === 'firing' ? '✕' : '!'} {a.alert} · {a.severity} ·{' '}
                                {a.window}
                            </li>
                        ))}
                    </ul>
                </div>
            )}
        </Card>
    );
}

function SloEditPanel({ name, content }: { name: string; content: string }) {
    const { t } = useTranslation();
    const save = useSaveSlo();
    const validate = useValidateSlo();
    const plan = usePlanSlo();
    const apply = useApplySlo();
    const [confirmApply, setConfirmApply] = useState(false);
    const [validation, setValidation] = useState<SloValidation | undefined>(undefined);
    const [planResult, setPlanResult] = useState<SloPlan | undefined>(undefined);
    const [applyResult, setApplyResult] = useState<SloApply | undefined>(undefined);

    function runValidate(text: string): void {
        validate.mutate(
            { name, content: text },
            {
                onSuccess: (v) => {
                    setValidation(v);
                },
            },
        );
    }

    function runPlan(): void {
        plan.mutate(name, {
            onSuccess: (p) => {
                setPlanResult(p);
                setApplyResult(undefined);
            },
        });
    }

    function runApply(): void {
        apply.mutate(
            { name },
            {
                onSuccess: (a) => {
                    setApplyResult(a);
                    setPlanResult(undefined);
                },
            },
        );
    }

    const allNoOp =
        applyResult !== undefined &&
        applyResult.results.length > 0 &&
        applyResult.results.every((r) => r.noOp);

    return (
        <div style={{ display: 'grid', gap: 16 }}>
            <Card title={t('slo.editorTitle')}>
                <SloEditor
                    initial={content}
                    saving={save.isPending}
                    validating={validate.isPending}
                    validation={validation}
                    onValidate={runValidate}
                    onSave={(text) => {
                        save.mutate(
                            { name, content: text },
                            {
                                onSuccess: () => {
                                    setValidation(undefined);
                                    setPlanResult(undefined);
                                    setApplyResult(undefined);
                                },
                            },
                        );
                    }}
                />
                {save.isError && (
                    <p role="alert" style={{ color: 'var(--crit)' }}>
                        {save.error.detail}
                    </p>
                )}
                {save.isSuccess && <p role="status">{t('slo.saved')}</p>}
                {validate.isError && (
                    <p role="alert" style={{ color: 'var(--crit)' }}>
                        {validate.error.detail}
                    </p>
                )}
            </Card>

            <Card title={t('slo.planTitle')}>
                <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                    <button type="button" onClick={runPlan} disabled={plan.isPending}>
                        {plan.isPending ? t('slo.planning') : t('slo.plan')}
                    </button>
                    <button
                        type="button"
                        onClick={() => {
                            setConfirmApply(true);
                        }}
                        disabled={apply.isPending}
                        title={t('slo.applyHint')}
                    >
                        {apply.isPending ? t('slo.applying') : t('slo.apply')}
                    </button>
                </div>
                {plan.isError && (
                    <p role="alert" style={{ color: 'var(--crit)' }}>
                        {plan.error.detail}
                    </p>
                )}
                {planResult !== undefined && (
                    <div style={{ marginTop: 12 }}>
                        {planResult.warnings.map((w, i) => (
                            <p key={i} role="note" style={{ color: 'var(--warn)' }}>
                                ⚠ {w}
                            </p>
                        ))}
                        {planResult.changes.length === 0 ? (
                            <p role="status" style={{ color: 'var(--ok)' }}>
                                ✓ {t('slo.planEmpty')}
                            </p>
                        ) : (
                            <>
                                <ul>
                                    {planResult.changes.map((c) => (
                                        <li key={`${c.slo}-${c.connection}`}>
                                            {t('slo.wouldUpdate', {
                                                slo: c.slo,
                                                connection: c.connection,
                                            })}
                                        </li>
                                    ))}
                                </ul>
                                {planResult.files
                                    .filter((f) => f.backend === 'prometheus' && f.change)
                                    .map((f) => (
                                        <div key={f.connection} style={{ marginTop: 12 }}>
                                            <h4 style={{ marginBottom: 4 }}>{f.connection}</h4>
                                            <DiffViewer
                                                actual={f.actual ?? ''}
                                                desired={f.desired ?? ''}
                                                note={f.note}
                                            />
                                        </div>
                                    ))}
                                {planResult.files
                                    .filter((f) => f.backend !== 'prometheus' && f.change)
                                    .map((f) => (
                                        <p
                                            key={f.connection}
                                            style={{ color: 'var(--text-muted)' }}
                                        >
                                            {f.connection}: {f.note ?? t('slo.datadogChange')}
                                        </p>
                                    ))}
                            </>
                        )}
                    </div>
                )}
            </Card>

            {applyResult !== undefined && (
                <Card title={t('slo.applyTitle')}>
                    {allNoOp && (
                        <p role="status" style={{ color: 'var(--ok)', fontWeight: 700 }}>
                            ✓ {t('slo.applyNoOp')}
                        </p>
                    )}
                    <ul>
                        {applyResult.results.map((r) => (
                            <li key={`${r.slo}-${r.connection}`}>
                                {r.noOp ? '✓ ' : r.applied ? '✓ ' : '✕ '}
                                {r.message}
                            </li>
                        ))}
                    </ul>
                    {applyResult.warnings.map((w, i) => (
                        <p key={i} role="note" style={{ color: 'var(--warn)' }}>
                            ⚠ {w}
                        </p>
                    ))}
                </Card>
            )}
            {apply.isError && (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {apply.error.detail}
                </p>
            )}

            <ConfirmDialog
                target={name}
                open={confirmApply}
                onOpenChange={setConfirmApply}
                onConfirm={runApply}
            />
        </div>
    );
}

function DriftPanel() {
    const { t } = useTranslation();
    const drift = useDrift();
    return (
        <Card title={t('slo.driftTitle')}>
            {drift.isPending && <p>{t('common.loading')}</p>}
            {drift.isError && <ErrorState onRetry={() => void drift.refetch()} />}
            {drift.isSuccess &&
                (drift.data.clean ? (
                    <p role="status" style={{ color: 'var(--ok)' }}>
                        ✓ {t('slo.driftClean')}
                    </p>
                ) : (
                    <div role="alert">
                        <ul>
                            {drift.data.drifted.map((d, i) => (
                                <li key={`d${d.slo}-${d.connection}-${String(i)}`}>
                                    ≠{' '}
                                    {t('slo.driftItem', {
                                        slo: d.slo,
                                        connection: d.connection,
                                        reason: d.reason,
                                    })}
                                </li>
                            ))}
                        </ul>
                    </div>
                ))}
        </Card>
    );
}

function CapabilitiesPanel() {
    const { t } = useTranslation();
    const caps = useCapabilities();
    return (
        <Card title={t('slo.capsTitle')}>
            <p style={{ color: 'var(--text-muted)', fontSize: 12 }}>{t('slo.capsWarning')}</p>
            {caps.isPending && <p>{t('common.loading')}</p>}
            {caps.isError && <ErrorState onRetry={() => void caps.refetch()} />}
            {caps.isSuccess && (
                <TableScroll label={t('slo.capsTitle')}>
                    <table style={{ borderCollapse: 'collapse', width: '100%' }}>
                        <thead>
                            <tr>
                                <th scope="col">{t('slo.feature')}</th>
                                <th scope="col">{t('slo.prometheus')}</th>
                                <th scope="col">{t('slo.datadog')}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {caps.data.features.map((f) => (
                                <tr key={f.feature}>
                                    <td>{f.feature}</td>
                                    <td>{f.prometheus}</td>
                                    <td>{f.datadog}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </TableScroll>
            )}
        </Card>
    );
}

function BudgetCalculator() {
    const { t, i18n } = useTranslation();
    const [objective, setObjective] = useState('99.9');
    const [window, setWindow] = useState('30d');
    const objNum = Number(objective.replace(',', '.')) / 100;
    const windowSecs = parseDurationSecs(window);
    const valid = objNum > 0 && objNum < 1 && windowSecs !== null;
    const minutes = valid ? errorBudgetMinutes(objNum, windowSecs) : null;
    return (
        <Card title={t('slo.calcTitle')}>
            <p style={{ color: 'var(--text-muted)', fontSize: 12 }}>{t('slo.calcHint')}</p>
            <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
                <label>
                    {t('slo.calcObjective')}
                    <input
                        value={objective}
                        inputMode="decimal"
                        onChange={(e) => {
                            setObjective(e.target.value);
                        }}
                        aria-invalid={!(objNum > 0 && objNum < 1)}
                    />
                </label>
                <label>
                    {t('slo.calcWindow')}
                    <input
                        value={window}
                        onChange={(e) => {
                            setWindow(e.target.value);
                        }}
                        aria-invalid={windowSecs === null}
                    />
                </label>
            </div>
            {minutes !== null ? (
                <p role="status">
                    {t('slo.calcResult', {
                        minutes: formatMinutes(minutes, i18n.language),
                        window,
                    })}
                </p>
            ) : (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {t('slo.calcInvalid')}
                </p>
            )}
        </Card>
    );
}

export function SloPage() {
    const { t } = useTranslation();
    const [selected, setSelected] = useState<string | null>(null);
    const detail = useSloDetail(selected);

    return (
        <section aria-labelledby="slo-title">
            <h1 id="slo-title">{t('nav.slo')}</h1>
            <div className="split-layout">
                <div style={{ display: 'grid', gap: 16 }}>
                    <SloListPanel selected={selected} onSelect={setSelected} />
                    <DriftPanel />
                </div>
                <div style={{ display: 'grid', gap: 16 }}>
                    {selected === null && <EmptyState next={t('slo.pickNext')} />}
                    {selected !== null && detail.isPending && <p>{t('common.loading')}</p>}
                    {selected !== null && detail.isError && (
                        <ErrorState onRetry={() => void detail.refetch()} />
                    )}
                    {selected !== null && detail.isSuccess && (
                        <>
                            <SloDetailPanel detail={detail.data} />
                            <SloEditPanel
                                key={selected}
                                name={selected}
                                content={detail.data.content}
                            />
                        </>
                    )}
                    <CapabilitiesPanel />
                    <BudgetCalculator />
                </div>
            </div>
        </section>
    );
}
