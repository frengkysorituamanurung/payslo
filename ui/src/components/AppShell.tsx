import { useEffect, useRef, useState } from 'react';
import { Link, NavLink, Outlet } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useSession, useStackHealth } from '../hooks/useLab';
import { useEvents } from '../hooks/useEvents';
import { usePendingApprovals, useIncidents } from '../hooks/useIncidents';
import { setActor } from '../api/client';
import { applyTheme, initialTheme } from '../utils/theme';
import { StatusBadge } from './StatusBadge';
import { CopyCommand } from './CopyCommand';

const ROUTES = [
    'mission',
    'map',
    'payments',
    'slo',
    'incidents',
    'scenarios',
    'control',
    'observe',
    'learn',
    'mentor',
    'settings',
] as const;

function pathOf(r: (typeof ROUTES)[number]): string {
    return r === 'mission' ? '/' : `/${r}`;
}

export function AppShell() {
    const { t, i18n } = useTranslation();
    const [theme, setTheme] = useState<'light' | 'dark'>(() => {
        const init = initialTheme();
        document.documentElement.dataset['theme'] = init;
        return init;
    });
    const session = useSession();
    const stack = useStackHealth();
    const pending = usePendingApprovals();
    const openPoll = useIncidents('open');
    const events = useEvents();

    // U4: toast in-app untuk insiden/approval BARU. Acuan "sudah ada" adalah
    // snapshot query awal (bukan burst SSE pertama) agar reload/screenshot
    // tak memicu toast untuk insiden lama; StrictMode-safe. aria-live agar
    // diumumkan pembaca layar.
    const toasted = useRef<Set<string>>(new Set());
    const [baseline, setBaseline] = useState<Set<string> | null>(null);
    const [toasts, setToasts] = useState<{ id: string; text: string; to: string }[]>([]);
    const pollReady = openPoll.isSuccess && pending.isSuccess;
    // Baseline dibekukan SEKALI dari snapshot query pertama: id yang sudah
    // ada saat load tak pernah di-toast; id baru (SSE lebih cepat dari poll
    // 10 dtk berikutnya) selalu di-toast.
    useEffect(() => {
        if (pollReady && baseline === null) {
            // eslint-disable-next-line react-hooks/set-state-in-effect
            setBaseline(
                new Set<string>([
                    ...openPoll.data.map((i) => `inc:${i.id}`),
                    ...pending.data.map((a) => `ap:${a.id}`),
                ]),
            );
        }
    }, [pollReady, baseline, openPoll.data, pending.data]);
    useEffect(() => {
        if (baseline === null) return undefined;
        const known = baseline;
        const fresh: { id: string; text: string; to: string }[] = [];
        for (const i of events.openIncidents) {
            const key = `inc:${i.id}`;
            if (!known.has(key) && !toasted.current.has(key)) {
                toasted.current.add(key);
                fresh.push({
                    id: key,
                    text: t('incident.toastIncident', { title: i.title }),
                    to: '/incidents',
                });
            }
        }
        for (const a of events.pendingApprovals) {
            const key = `ap:${a.id}`;
            if (!known.has(key) && !toasted.current.has(key)) {
                toasted.current.add(key);
                fresh.push({
                    id: key,
                    text: t('incident.toastApproval', { action: a.action }),
                    to: '/incidents',
                });
            }
        }
        if (fresh.length > 0) {
            // Notifikasi event-driven (bukan render cascade): daftar toast
            // dikelola di sini karena EventSource hidup di useEvents.
            // eslint-disable-next-line react-hooks/set-state-in-effect
            setToasts((prev) => [...fresh, ...prev].slice(0, 5));
            const timer = setTimeout(() => {
                setToasts((prev) => prev.filter((x) => !fresh.some((f) => f.id === x.id)));
            }, 12000);
            return () => {
                clearTimeout(timer);
            };
        }
        return undefined;
    }, [events.openIncidents, events.pendingApprovals, pollReady, baseline, t]);

    // U2: aktor audit = mode sesi (trainee/mentor). Dikirim sebagai X-Lab-Actor
    // pada setiap mutasi oleh api/client.
    useEffect(() => {
        if (session.data !== undefined) setActor(session.data.mode);
    }, [session.data]);

    const stackUp = stack.data?.labUp ?? false;
    const showOnboarding = stack.isSuccess && !stackUp;

    return (
        <div style={{ display: 'flex', minHeight: '100vh' }}>
            <nav
                aria-label={t('app.title')}
                style={{ width: 220, borderRight: '1px solid var(--border)', padding: 12 }}
            >
                <p style={{ fontWeight: 800, margin: '0 0 4px' }}>{t('app.title')}</p>
                <p style={{ color: 'var(--text-muted)', fontSize: 12, margin: '0 0 12px' }}>
                    {t('app.tagline')}
                </p>
                <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: 2 }}>
                    {ROUTES.map((r) => (
                        <li key={r}>
                            <NavLink
                                to={pathOf(r)}
                                style={({ isActive }) => ({
                                    display: 'block',
                                    padding: '6px 8px',
                                    borderRadius: 'var(--radius)',
                                    background: isActive ? 'var(--neutral-bg)' : undefined,
                                })}
                            >
                                {t(`nav.${r}`)}
                                {r === 'incidents' &&
                                    pending.isSuccess &&
                                    pending.data.length > 0 && (
                                        <span
                                            role="status"
                                            aria-label={t('incident.approvalBadge', {
                                                count: pending.data.length,
                                            })}
                                            style={{
                                                marginLeft: 6,
                                                background: 'var(--warn-bg)',
                                                color: 'var(--warn)',
                                                borderRadius: 'var(--radius)',
                                                padding: '0 6px',
                                                fontWeight: 700,
                                            }}
                                        >
                                            {pending.data.length}
                                        </span>
                                    )}
                            </NavLink>
                        </li>
                    ))}
                </ul>
            </nav>
            <div style={{ flex: 1, minWidth: 0 }}>
                <header
                    style={{
                        display: 'flex',
                        gap: 12,
                        alignItems: 'center',
                        padding: '8px 16px',
                        borderBottom: '1px solid var(--border)',
                        flexWrap: 'wrap',
                    }}
                >
                    {stack.isSuccess ? (
                        <StatusBadge
                            state={stackUp ? 'up' : 'down'}
                            label={stackUp ? t('topbar.stackUp') : t('topbar.stackDown')}
                        />
                    ) : (
                        <StatusBadge state="unknown" />
                    )}
                    {session.isSuccess && (
                        <span style={{ fontWeight: 600 }}>
                            {session.data.mode === 'mentor'
                                ? t('topbar.modeMentor')
                                : t('topbar.modeTrainee')}
                        </span>
                    )}
                    <span style={{ flex: 1 }} />
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
                        {t('topbar.language')}{' '}
                        <select
                            value={i18n.language}
                            onChange={(e) => void i18n.changeLanguage(e.target.value)}
                        >
                            <option value="id">{t('topbar.langId')}</option>
                            <option value="en">{t('topbar.langEn')}</option>
                        </select>
                    </label>
                </header>
                <main style={{ padding: 16, maxWidth: 1280 }}>
                    {showOnboarding ? (
                        <section aria-labelledby="onboarding-title">
                            <h1 id="onboarding-title">{t('onboarding.title')}</h1>
                            <p>{t('onboarding.body')}</p>
                            <CopyCommand command={t('onboarding.command')} />
                        </section>
                    ) : (
                        <Outlet />
                    )}
                </main>
            </div>
            <div
                aria-live="polite"
                style={{
                    position: 'fixed',
                    right: 16,
                    bottom: 16,
                    display: 'grid',
                    gap: 8,
                    zIndex: 50,
                }}
            >
                {toasts.map((toast) => (
                    <div
                        key={toast.id}
                        role="status"
                        style={{
                            background: 'var(--surface-raised)',
                            border: '1px solid var(--border)',
                            borderRadius: 'var(--radius)',
                            padding: '8px 12px',
                            maxWidth: 360,
                        }}
                    >
                        {toast.text} <Link to={toast.to}>{t('nav.incidents')}</Link>
                    </div>
                ))}
            </div>
        </div>
    );
}
