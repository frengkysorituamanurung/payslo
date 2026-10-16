import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import DOMPurify from 'dompurify';
import {
    useCurriculum,
    useGlossary,
    useLearnProgress,
    useLearnProgressPut,
} from '../hooks/useLearn';
import type { CurriculumModule } from '../hooks/useLearn';
import { EmptyState, ErrorState } from '../components/States';

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

// Render HTML yang SUDAH disanitasi server-side. DOMPurify = lapisan kedua
// (UI-SPEC: markdown kurikulum boleh dangerouslySetInnerHTML bila disanitasi
// dan diuji). Tanpa DOMPurify ini, server yang diganti bisa menyuntikkan HTML.
function SafeHtml({ html }: { html: string }) {
    const clean = DOMPurify.sanitize(html, {
        ALLOWED_TAGS: [
            'h1',
            'h2',
            'h3',
            'h4',
            'p',
            'ul',
            'ol',
            'li',
            'strong',
            'em',
            'code',
            'pre',
            'a',
        ],
        ALLOWED_ATTR: ['href'],
        ALLOWED_URI_REGEXP: /^(https?:|mailto:)/i,
    });
    return <div dangerouslySetInnerHTML={{ __html: clean }} />;
}

function ModuleSection({
    mod,
    doneKeys,
    onToggle,
}: {
    mod: CurriculumModule;
    doneKeys: Set<string>;
    onToggle: (key: string, done: boolean) => void;
}) {
    const { t } = useTranslation();
    const [open, setOpen] = useState(true);
    const doneCount = mod.items.filter((it) => doneKeys.has(it.key)).length;
    const pct = mod.items.length === 0 ? 0 : Math.round((doneCount / mod.items.length) * 100);

    return (
        <Card title={`${mod.day} — ${mod.title}`}>
            <button
                type="button"
                onClick={() => {
                    setOpen(!open);
                }}
                aria-expanded={open}
                style={{
                    display: 'flex',
                    gap: 8,
                    alignItems: 'center',
                    background: 'none',
                    border: 'none',
                    padding: 0,
                    cursor: 'pointer',
                }}
            >
                <span aria-hidden="true">{open ? '▾' : '▸'}</span>
                <span style={{ fontWeight: 700 }}>
                    {t('learn.moduleProgress', { done: doneCount, total: mod.items.length, pct })}
                </span>
            </button>
            {open && (
                <>
                    <SafeHtml html={mod.html} />
                    {mod.items.length > 0 && (
                        <fieldset style={{ marginTop: 12, border: 'none', padding: 0 }}>
                            <legend style={{ fontWeight: 700, marginBottom: 4 }}>
                                {t('learn.checklist')}
                            </legend>
                            <ul
                                style={{
                                    listStyle: 'none',
                                    padding: 0,
                                    margin: 0,
                                    display: 'grid',
                                    gap: 4,
                                }}
                            >
                                {mod.items.map((it) => (
                                    <li key={it.key}>
                                        <label
                                            style={{
                                                display: 'flex',
                                                gap: 8,
                                                alignItems: 'start',
                                                cursor: 'pointer',
                                            }}
                                        >
                                            <input
                                                type="checkbox"
                                                checked={doneKeys.has(it.key)}
                                                onChange={(e) => {
                                                    onToggle(it.key, e.target.checked);
                                                }}
                                            />
                                            <span>
                                                <span
                                                    style={{
                                                        color: 'var(--text-muted)',
                                                        fontSize: 12,
                                                    }}
                                                >
                                                    [{t(`learn.section.${it.section}`)}]
                                                </span>{' '}
                                                {it.label}
                                            </span>
                                        </label>
                                    </li>
                                ))}
                            </ul>
                        </fieldset>
                    )}
                </>
            )}
        </Card>
    );
}

export function LearnPage() {
    const { t } = useTranslation();
    const curriculum = useCurriculum();
    const glossary = useGlossary();
    const progress = useLearnProgress();
    const putProgress = useLearnProgressPut();

    const doneKeys = new Set(
        (progress.data ?? []).filter((i) => i.done).map((i) => `${i.module}:${i.key}`),
    );

    function toggle(module: string, key: string, done: boolean) {
        putProgress.mutate({ module, key, done });
    }

    if (curriculum.isPending) return <p>{t('common.loading')}</p>;
    if (curriculum.isError) return <ErrorState onRetry={() => void curriculum.refetch()} />;
    if (curriculum.data.length === 0) return <EmptyState next={t('learn.empty')} />;

    return (
        <section aria-labelledby="learn-title">
            <h1 id="learn-title">{t('nav.learn')}</h1>
            <p style={{ color: 'var(--text-muted)' }}>{t('learn.intro')}</p>
            <div style={{ display: 'grid', gap: 16, alignItems: 'start' }}>
                {curriculum.data.map((mod) => (
                    <ModuleSection
                        key={mod.day}
                        mod={mod}
                        doneKeys={doneKeys}
                        onToggle={(k, d) => {
                            toggle(mod.day, k, d);
                        }}
                    />
                ))}
                <Card title={t('learn.glossaryTitle')}>
                    {glossary.isPending && <p>{t('common.loading')}</p>}
                    {glossary.isSuccess && (
                        <dl
                            style={{
                                display: 'grid',
                                gridTemplateColumns: 'max-content 1fr',
                                gap: '6px 16px',
                            }}
                        >
                            {glossary.data.map((g) => (
                                <div key={g.term} style={{ display: 'contents' }}>
                                    <dt style={{ fontWeight: 700 }}>{g.term}</dt>
                                    <dd style={{ margin: 0, color: 'var(--text-muted)' }}>
                                        {g.definition}
                                    </dd>
                                </div>
                            ))}
                        </dl>
                    )}
                    {putProgress.isError && (
                        <p role="alert" style={{ color: 'var(--crit)' }}>
                            {putProgress.error.detail}
                        </p>
                    )}
                </Card>
            </div>
        </section>
    );
}
