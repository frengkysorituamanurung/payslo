import { useEffect, useRef, useState } from 'react';
import { Link, NavLink, Outlet } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useSession, useStackHealth } from '../hooks/useLab';
import { useEvents } from '../hooks/useEvents';
import { usePendingApprovals, useIncidents } from '../hooks/useIncidents';
import { setActor, setMentorToken } from '../api/client';
import { useMentorLock, useMentorUnlock } from '../hooks/useMentor';
import { applyTheme, initialTheme } from '../utils/theme';
import { StatusBadge } from './StatusBadge';
import { CopyCommand } from './CopyCommand';

type RouteId =
    | 'mission'
    | 'payments'
    | 'control'
    | 'slo'
    | 'incidents'
    | 'observe'
    | 'map'
    | 'learn'
    | 'scenarios'
    | 'mentor'
    | 'settings';

const NAV_GROUPS: ReadonlyArray<{ key: string; routes: readonly RouteId[] }> = [
    { key: 'run', routes: ['mission', 'payments', 'control'] },
    { key: 'reliability', routes: ['slo', 'incidents', 'observe', 'map'] },
    { key: 'learn', routes: ['learn', 'scenarios', 'mentor'] },
    { key: 'system', routes: ['settings'] },
];

function pathOf(r: RouteId): string {
    return r === 'mission' ? '/' : `/${r}`;
}

function LockIcon() {
    return (
        <svg className="icon" viewBox="0 0 16 16" aria-hidden="true" focusable="false">
            <rect
                x="3.25"
                y="7"
                width="9.5"
                height="6.5"
                rx="1.5"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.4"
            />
            <path
                d="M5.4 7V5.1a2.6 2.6 0 0 1 5.2 0V7"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.4"
                strokeLinecap="round"
            />
        </svg>
    );
}

// Dialog PIN mentor: pembatas spoiler (bukan keamanan). Dokumentasi jujur
// di docs/mentor-guide.md.
function MentorUnlockDialog({ onClose }: { onClose: () => void }) {
    const { t } = useTranslation();
    const unlock = useMentorUnlock();
    const [pin, setPin] = useState('');
    const [error, setError] = useState('');

    return (
        <div
            className="dialog-backdrop"
            role="dialog"
            aria-modal="true"
            aria-label={t('mentor.unlockTitle')}
        >
            <div className="dialog">
                <h2>{t('mentor.unlockTitle')}</h2>
                <p className="dialog-hint">{t('mentor.unlockHint')}</p>
                <form
                    onSubmit={(e) => {
                        e.preventDefault();
                        setError('');
                        unlock.mutate(pin, {
                            onSuccess: (data) => {
                                setMentorToken(data.token);
                                onClose();
                            },
                            onError: (err) => {
                                setError(err.message);
                            },
                        });
                    }}
                >
                    <label>
                        {t('mentor.unlockPin')}
                        <input
                            type="password"
                            value={pin}
                            onChange={(e) => {
                                setPin(e.target.value);
                                setError('');
                            }}
                        />
                    </label>
                    {error !== '' && (
                        <p role="alert" style={{ color: 'var(--crit)' }}>
                            {error}
                        </p>
                    )}
                    <div className="dialog-actions">
                        <button type="submit" disabled={unlock.isPending || pin.trim() === ''}>
                            {unlock.isPending ? t('common.loading') : t('mentor.unlockButton')}
                        </button>
                        <button type="button" onClick={onClose}>
                            {t('common.cancel')}
                        </button>
                    </div>
                </form>
            </div>
        </div>
    );
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
    const [navOpen, setNavOpen] = useState(false);

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
    const isMentor = session.data?.mode === 'mentor';
    const [unlockOpen, setUnlockOpen] = useState(false);
    const lock = useMentorLock();

    return (
        <div className="app">
            <aside className="sidebar">
                <div className="brand">
                    <div>
                        <p className="brand-name">{t('app.brand')}</p>
                        <p className="brand-sub">{t('app.subtitle')}</p>
                    </div>
                    <button
                        type="button"
                        className="nav-toggle"
                        aria-expanded={navOpen}
                        aria-controls="app-nav"
                        onClick={() => {
                            setNavOpen((v) => !v);
                        }}
                    >
                        {t('nav.menu')}
                    </button>
                </div>
                <nav
                    id="app-nav"
                    className={navOpen ? 'sidebar-nav is-open' : 'sidebar-nav'}
                    aria-label={t('app.title')}
                >
                    {NAV_GROUPS.map((group) => {
                        const routes = group.routes.filter((r) => r !== 'mentor' || isMentor);
                        if (routes.length === 0) return null;
                        return (
                            <div className="nav-group" key={group.key}>
                                <p className="nav-label">{t(`navGroup.${group.key}`)}</p>
                                <ul className="nav-list">
                                    {routes.map((r) => (
                                        <li key={r}>
                                            <NavLink
                                                to={pathOf(r)}
                                                className={({ isActive }) =>
                                                    isActive ? 'nav-link active' : 'nav-link'
                                                }
                                                onClick={() => {
                                                    setNavOpen(false);
                                                }}
                                            >
                                                <span>{t(`nav.${r}`)}</span>
                                                {r === 'incidents' &&
                                                    pending.isSuccess &&
                                                    pending.data.length > 0 && (
                                                        <span
                                                            role="status"
                                                            className="nav-count"
                                                            aria-label={t(
                                                                'incident.approvalBadge',
                                                                {
                                                                    count: pending.data.length,
                                                                },
                                                            )}
                                                        >
                                                            {pending.data.length}
                                                        </span>
                                                    )}
                                            </NavLink>
                                        </li>
                                    ))}
                                </ul>
                            </div>
                        );
                    })}
                </nav>
            </aside>
            <div className="content">
                <header className="topbar">
                    {stack.isSuccess ? (
                        <StatusBadge
                            state={stackUp ? 'up' : 'down'}
                            label={stackUp ? t('topbar.stackUp') : t('topbar.stackDown')}
                        />
                    ) : (
                        <StatusBadge state="unknown" />
                    )}
                    {session.isSuccess && (
                        <span role="status" className={isMentor ? 'chip chip-warn' : 'chip'}>
                            {isMentor ? t('topbar.modeMentor') : t('topbar.modeTrainee')}
                        </span>
                    )}
                    {isMentor && (
                        <button
                            type="button"
                            onClick={() => {
                                lock.mutate(undefined, {
                                    onSettled: () => {
                                        setMentorToken(null);
                                    },
                                });
                            }}
                        >
                            {t('mentor.exitMentor')}
                        </button>
                    )}
                    <span className="topbar-spacer" />
                    {!isMentor && (
                        <button
                            type="button"
                            onClick={() => {
                                setUnlockOpen(true);
                            }}
                            aria-label={t('mentor.unlockButton')}
                        >
                            <LockIcon />
                            {t('mentor.unlockButton')}
                        </button>
                    )}
                    <label>
                        {t('topbar.theme')}
                        <select
                            data-testid="theme-select"
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
                        {t('topbar.language')}
                        <select
                            data-testid="lang-select"
                            value={i18n.language}
                            onChange={(e) => void i18n.changeLanguage(e.target.value)}
                        >
                            <option value="id">{t('topbar.langId')}</option>
                            <option value="en">{t('topbar.langEn')}</option>
                        </select>
                    </label>
                </header>
                <main className="main">
                    {showOnboarding ? (
                        <section className="page" aria-labelledby="onboarding-title">
                            <h1 id="onboarding-title">{t('onboarding.title')}</h1>
                            <div className="panel">
                                <p>{t('onboarding.body')}</p>
                                <CopyCommand command={t('onboarding.command')} />
                            </div>
                        </section>
                    ) : (
                        <Outlet />
                    )}
                </main>
            </div>
            <div className="toast-region" aria-live="polite">
                {toasts.map((toast) => (
                    <div key={toast.id} role="status" className="toast">
                        {toast.text} <Link to={toast.to}>{t('nav.incidents')}</Link>
                    </div>
                ))}
            </div>
            {unlockOpen && (
                <MentorUnlockDialog
                    onClose={() => {
                        setUnlockOpen(false);
                    }}
                />
            )}
        </div>
    );
}
