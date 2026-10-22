import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSaveSettings, useSettings } from '../hooks/useIncidents';
import { useCopyDiagnostics, useDiagnostics } from '../hooks/useObserve';
import { applyTheme, type Theme } from '../utils/theme';
import { ErrorState } from '../components/States';
import { CopyCommand } from '../components/CopyCommand';
import { TableScroll } from '../components/TableScroll';

function Card({ title, children }: { title: string; children: React.ReactNode }) {
    return (
        <section aria-label={title} className="panel">
            <h2 style={{ marginTop: 0 }}>{title}</h2>
            {children}
        </section>
    );
}

function SettingsForm() {
    const { t, i18n } = useTranslation();
    const settings = useSettings();
    const save = useSaveSettings();
    const [name, setName] = useState<string | null>(null);
    const [threshold, setThreshold] = useState<string | null>(null);
    const [theme, setTheme] = useState<Theme>(() =>
        document.documentElement.dataset['theme'] === 'dark' ? 'dark' : 'light',
    );

    if (settings.isPending) return <p>{t('common.loading')}</p>;
    if (settings.isError) {
        return (
            <ErrorState
                onRetry={() => {
                    void settings.refetch();
                }}
            />
        );
    }

    const savedThreshold = settings.data.lowSampleThreshold;
    const shownName = name ?? settings.data.traineeName ?? '';
    const shownThreshold =
        threshold ?? (typeof savedThreshold === 'number' ? String(savedThreshold) : '0.2');
    const parsedThreshold = Number(shownThreshold);
    const thresholdOK =
        shownThreshold.trim() !== '' &&
        Number.isFinite(parsedThreshold) &&
        parsedThreshold >= 0 &&
        parsedThreshold <= 10;

    return (
        <div style={{ display: 'grid', gap: 12, maxWidth: 480 }}>
            <label>
                {t('settings.traineeName')}
                <input
                    value={shownName}
                    onChange={(e) => {
                        setName(e.target.value);
                    }}
                    style={{ width: '100%' }}
                />
            </label>
            <p style={{ margin: 0, color: 'var(--text-muted)', fontSize: 12 }}>
                {t('settings.traineeNameHint')}
            </p>
            <label>
                {t('topbar.language')}{' '}
                <select
                    value={i18n.language}
                    onChange={(e) => {
                        void i18n.changeLanguage(e.target.value);
                    }}
                >
                    <option value="id">{t('topbar.langId')}</option>
                    <option value="en">{t('topbar.langEn')}</option>
                </select>
            </label>
            <label>
                {t('topbar.theme')}{' '}
                <select
                    value={theme}
                    onChange={(e) => {
                        const next = e.target.value === 'dark' ? 'dark' : 'light';
                        setTheme(next);
                        applyTheme(next);
                    }}
                >
                    <option value="light">{t('topbar.themeLight')}</option>
                    <option value="dark">{t('topbar.themeDark')}</option>
                </select>
            </label>
            <label>
                {t('settings.threshold')}
                <input
                    value={shownThreshold}
                    onChange={(e) => {
                        setThreshold(e.target.value);
                    }}
                    inputMode="decimal"
                    aria-describedby="threshold-hint"
                    aria-invalid={!thresholdOK}
                    style={{ width: '100%' }}
                />
            </label>
            <p id="threshold-hint" style={{ margin: 0, color: 'var(--text-muted)', fontSize: 12 }}>
                {t('settings.thresholdHint')}
            </p>
            {!thresholdOK && (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {t('settings.thresholdInvalid')}
                </p>
            )}
            <div>
                <button
                    type="button"
                    className="btn-primary"
                    disabled={save.isPending || !thresholdOK}
                    onClick={() => {
                        save.mutate(
                            {
                                traineeName: shownName.trim() === '' ? undefined : shownName.trim(),
                                lowSampleThreshold: parsedThreshold,
                            },
                            {
                                onSuccess: () => {
                                    setName(null);
                                    setThreshold(null);
                                },
                            },
                        );
                    }}
                >
                    {save.isPending ? t('common.loading') : t('settings.save')}
                </button>
            </div>
            {save.isError && (
                <p role="alert" style={{ color: 'var(--crit)' }}>
                    {save.error.detail}
                </p>
            )}
            {save.isSuccess && <p role="status">{t('settings.saved')}</p>}
        </div>
    );
}

function DiagnosticsPanel() {
    const { t } = useTranslation();
    const diag = useDiagnostics();
    const copy = useCopyDiagnostics();
    const [copied, setCopied] = useState(false);

    if (diag.isPending) return <p>{t('common.loading')}</p>;
    if (diag.isError) return <ErrorState onRetry={() => void diag.refetch()} />;
    const report = diag.data;

    return (
        <div style={{ display: 'grid', gap: 12 }}>
            <h3>{t('settings.services')}</h3>
            <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: 6 }}>
                {report.services.map((s) => (
                    <li
                        key={s.name}
                        style={{
                            display: 'flex',
                            gap: 8,
                            alignItems: 'baseline',
                            flexWrap: 'wrap',
                        }}
                    >
                        <span aria-hidden="true">{s.reachable ? '✓' : '✕'}</span>
                        <strong>{s.name}</strong>
                        <span style={{ color: 'var(--text-muted)', fontSize: 12 }}>
                            {s.version}
                        </span>
                        <span style={{ color: 'var(--text-muted)', fontSize: 12 }}>
                            {s.reachable ? t('status.up') : t('status.down')}
                        </span>
                    </li>
                ))}
            </ul>
            <h3>{t('settings.resources')}</h3>
            {!report.docker.available ? (
                <p role="note" style={{ color: 'var(--text-muted)' }}>
                    {report.docker.note ?? t('settings.dockerUnavailable')}
                </p>
            ) : (
                <TableScroll label={t('settings.resources')}>
                    <table>
                        <thead>
                            <tr>
                                <th scope="col">{t('settings.container')}</th>
                                <th scope="col">{t('settings.cpu')}</th>
                                <th scope="col">{t('settings.memory')}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {(report.docker.stats ?? []).map((s) => (
                                <tr key={s.name}>
                                    <td>{s.name}</td>
                                    <td>{s.cpuPerc}</td>
                                    <td>
                                        {s.memUsage} ({s.memPerc})
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </TableScroll>
            )}
            <h3>{t('settings.reportTitle')}</h3>
            <p style={{ color: 'var(--text-muted)', fontSize: 12 }}>{t('settings.reportHint')}</p>
            <pre
                // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- axe scrollable-region-focusable mewajibkan region scroll dapat fokus keyboard
                tabIndex={0}
                aria-label={t('settings.reportTitle')}
                style={{
                    background: 'var(--neutral-bg)',
                    padding: 12,
                    overflowX: 'auto',
                    maxHeight: 320,
                    overflowY: 'auto',
                }}
            >
                {JSON.stringify(report, null, 2)}
            </pre>
            <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                <button
                    type="button"
                    disabled={copy.isPending}
                    onClick={() => {
                        copy.mutate(report, {
                            onSuccess: () => {
                                setCopied(true);
                                window.setTimeout(() => {
                                    setCopied(false);
                                }, 3000);
                            },
                        });
                    }}
                >
                    {t('settings.copyReport')}
                </button>
                {copied && <span role="status">{t('settings.copied')}</span>}
                {copy.isError && (
                    <span role="alert" style={{ color: 'var(--crit)' }}>
                        {copy.error.detail}
                    </span>
                )}
            </div>
            <CopyCommand command="make status" />
        </div>
    );
}

export function SettingsPage() {
    const { t } = useTranslation();
    return (
        <section className="page" aria-labelledby="settings-title">
            <h1 id="settings-title">{t('nav.settings')}</h1>
            <div style={{ display: 'grid', gap: 16, alignItems: 'start' }}>
                <Card title={t('settings.generalTitle')}>
                    <SettingsForm />
                </Card>
                <Card title={t('settings.diagnosticsTitle')}>
                    <DiagnosticsPanel />
                </Card>
            </div>
        </section>
    );
}
