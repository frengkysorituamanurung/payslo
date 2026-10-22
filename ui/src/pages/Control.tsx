import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { RoutingConfig } from '../api/client';
import {
    useAudit,
    useRouting,
    useSetRelease,
    useSetRouting,
    useSetTraffic,
    useTraffic,
} from '../hooks/usePayments';
import { formatRelative } from '../utils/time';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { ErrorState } from '../components/States';
import { TableScroll } from '../components/TableScroll';

const PROVIDERS = ['bca', 'mandiri', 'ewallet'] as const;

function validWeight(w: number): boolean {
    return Number.isInteger(w) && w >= 0 && w <= 100;
}

function TrafficPanel({ ask }: { ask: (target: string, run: () => void) => void }) {
    const { t } = useTranslation();
    const traffic = useTraffic();
    const setTraffic = useSetTraffic();
    const [rate, setRate] = useState('2');
    const [invalid, setInvalid] = useState(false);

    const current = traffic.data?.ratePerSec;

    return (
        <section aria-labelledby="control-traffic-title" className="panel">
            <h2 id="control-traffic-title">{t('control.trafficTitle')}</h2>
            <p>
                {t('control.currentRate')}:{' '}
                {current === null || current === undefined ? (
                    <span style={{ color: 'var(--text-muted)' }}>{t('control.rateUnknown')}</span>
                ) : (
                    <strong>{current}</strong>
                )}
            </p>
            <p style={{ color: 'var(--text-muted)', fontSize: 12 }}>
                {t('control.loadgenRestart')}
            </p>
            <label>
                {t('control.rateLabel')}
                <input
                    name="rate"
                    inputMode="numeric"
                    value={rate}
                    onChange={(e) => {
                        setRate(e.target.value);
                        setInvalid(false);
                    }}
                />
            </label>
            <div style={{ marginTop: 8 }}>
                <button
                    type="button"
                    className="btn-primary"
                    onClick={() => {
                        const n = Number(rate);
                        if (!Number.isInteger(n) || n < 1 || n > 100) {
                            setInvalid(true);
                            return;
                        }
                        ask(t('control.targetTraffic'), () => {
                            setTraffic.mutate(n);
                        });
                    }}
                >
                    {t('control.apply')}
                </button>
            </div>
            {invalid && (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {t('control.rateInvalid')}
                </p>
            )}
            {setTraffic.isError && (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {setTraffic.error.detail}
                </p>
            )}
        </section>
    );
}

function RoutingPanel({ ask }: { ask: (target: string, run: () => void) => void }) {
    const { t } = useTranslation();
    const routing = useRouting();
    const setRouting = useSetRouting();
    // Overlay edit lokal di atas data server (tidak menimpa bila refetch berkala):
    // cfg = data server + overrides. Setelah sukses simpan, override dibersihkan.
    const [overrides, setOverrides] = useState<Partial<RoutingConfig>>({});
    const [invalid, setInvalid] = useState(false);

    if (routing.isPending) return <p>{t('common.loading')}</p>;
    if (routing.isError)
        return (
            <ErrorState
                onRetry={() => {
                    void routing.refetch();
                }}
            />
        );
    const cfg: RoutingConfig = { ...routing.data, ...overrides };

    function update(name: (typeof PROVIDERS)[number], patch: Partial<RoutingConfig['bca']>): void {
        setOverrides((o) => ({ ...o, [name]: { ...cfg[name], ...patch } }));
        setInvalid(false);
    }

    return (
        <section aria-labelledby="control-routing-title" className="panel">
            <h2 id="control-routing-title">{t('control.routingTitle')}</h2>
            <TableScroll label={t('control.routingTitle')}>
                <table style={{ borderCollapse: 'collapse', width: '100%' }}>
                    <thead>
                        <tr>
                            <th scope="col">{t('payments.provider')}</th>
                            <th scope="col">{t('control.enabled')}</th>
                            <th scope="col">{t('control.weight')}</th>
                        </tr>
                    </thead>
                    <tbody>
                        {PROVIDERS.map((name) => (
                            <tr key={name}>
                                <td>{name}</td>
                                <td>
                                    <input
                                        type="checkbox"
                                        checked={cfg[name].enabled}
                                        aria-label={`${name} ${t('control.enabled')}`}
                                        onChange={(e) => {
                                            update(name, { enabled: e.target.checked });
                                        }}
                                    />
                                </td>
                                <td>
                                    <input
                                        type="number"
                                        min={0}
                                        max={100}
                                        value={cfg[name].weight}
                                        aria-label={`${name} ${t('control.weight')}`}
                                        onChange={(e) => {
                                            update(name, { weight: Number(e.target.value) });
                                        }}
                                        style={{ width: 80 }}
                                    />
                                </td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </TableScroll>
            <div style={{ marginTop: 8 }}>
                <button
                    type="button"
                    className="btn-primary"
                    onClick={() => {
                        if (
                            !validWeight(cfg.bca.weight) ||
                            !validWeight(cfg.mandiri.weight) ||
                            !validWeight(cfg.ewallet.weight)
                        ) {
                            setInvalid(true);
                            return;
                        }
                        ask(t('control.targetRouting'), () => {
                            setRouting.mutate(cfg, {
                                onSuccess: () => {
                                    setOverrides({});
                                },
                            });
                        });
                    }}
                >
                    {t('control.save')}
                </button>
            </div>
            {invalid && (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {t('control.routingInvalid')}
                </p>
            )}
            {setRouting.isError && (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {setRouting.error.detail}
                </p>
            )}
        </section>
    );
}

function ReleasePanel({ ask }: { ask: (target: string, run: () => void) => void }) {
    const { t } = useTranslation();
    const setRelease = useSetRelease();
    return (
        <section aria-labelledby="control-release-title" className="panel">
            <h2 id="control-release-title">{t('control.releaseTitle')}</h2>
            <p style={{ color: 'var(--text-muted)', fontSize: 12 }}>{t('control.releaseHint')}</p>
            <p>
                {t('control.releaseCurrent')}:{' '}
                <strong>{setRelease.data?.version ?? t('control.releaseUnknown')}</strong>
            </p>
            <div style={{ display: 'flex', gap: 8 }}>
                <button
                    type="button"
                    onClick={() => {
                        ask(t('control.targetReleaseGood'), () => {
                            setRelease.mutate('good');
                        });
                    }}
                >
                    {t('control.releaseGood')}
                </button>
                <button
                    type="button"
                    onClick={() => {
                        ask(t('control.targetReleaseBad'), () => {
                            setRelease.mutate('bad');
                        });
                    }}
                >
                    {t('control.releaseBad')}
                </button>
            </div>
            {setRelease.isError && (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {setRelease.error.detail}
                </p>
            )}
        </section>
    );
}

function AuditPanel() {
    const { t, i18n } = useTranslation();
    const audit = useAudit(20);
    return (
        <section aria-labelledby="control-audit-title">
            <h2 id="control-audit-title">{t('control.auditTitle')}</h2>
            {audit.isPending && <p>{t('common.loading')}</p>}
            {audit.isError && (
                <ErrorState
                    onRetry={() => {
                        void audit.refetch();
                    }}
                />
            )}
            {audit.isSuccess && audit.data.length === 0 && (
                <p style={{ color: 'var(--text-muted)' }}>{t('control.auditEmpty')}</p>
            )}
            {audit.isSuccess && audit.data.length > 0 && (
                <TableScroll label={t('control.auditTitle')}>
                    <table style={{ borderCollapse: 'collapse', width: '100%' }}>
                        <thead>
                            <tr>
                                <th scope="col">{t('control.auditTime')}</th>
                                <th scope="col">{t('control.auditActor')}</th>
                                <th scope="col">{t('control.auditAction')}</th>
                                <th scope="col">{t('control.auditTarget')}</th>
                                <th scope="col">{t('control.auditResult')}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {audit.data.map((entry, i) => (
                                <tr key={`${entry.ts}-${String(i)}`}>
                                    <td>
                                        <time dateTime={entry.ts} title={entry.ts}>
                                            {formatRelative(entry.ts, i18n.language)}
                                        </time>
                                    </td>
                                    <td>{entry.actor}</td>
                                    <td>{entry.action}</td>
                                    <td>{entry.target}</td>
                                    <td>
                                        {entry.result === 'ok'
                                            ? t('control.resultOk')
                                            : t('control.resultError')}
                                        {entry.error !== undefined &&
                                            entry.error !== '' &&
                                            ` — ${entry.error}`}
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </TableScroll>
            )}
        </section>
    );
}

export function ControlPage() {
    const { t } = useTranslation();
    const [pending, setPending] = useState<{ target: string; run: () => void } | null>(null);

    function ask(target: string, run: () => void): void {
        setPending({ target, run });
    }

    return (
        <section className="page" aria-labelledby="control-title">
            <h1 id="control-title">{t('nav.control')}</h1>
            <p style={{ color: 'var(--text-muted)' }}>{t('control.intro')}</p>

            <div
                style={{
                    display: 'grid',
                    gridTemplateColumns: 'repeat(auto-fit, minmax(min(280px, 100%), 1fr))',
                    gap: 16,
                }}
            >
                <TrafficPanel ask={ask} />
                <RoutingPanel ask={ask} />
                <ReleasePanel ask={ask} />
            </div>

            <div style={{ marginTop: 16 }}>
                <AuditPanel />
            </div>

            {pending !== null && (
                <ConfirmDialog
                    target={pending.target}
                    open
                    onOpenChange={(open) => {
                        if (!open) setPending(null);
                    }}
                    onConfirm={() => {
                        pending.run();
                        setPending(null);
                    }}
                />
            )}
        </section>
    );
}
