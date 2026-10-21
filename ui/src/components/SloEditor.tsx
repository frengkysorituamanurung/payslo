import {
    Component,
    lazy,
    Suspense,
    useEffect,
    useMemo,
    useRef,
    useState,
    type ReactNode,
} from 'react';
import { useTranslation } from 'react-i18next';
import type { SloValidation } from '../hooks/useSlo';
import { normalizeEditorText } from '../utils/text';

// CodeMirror dimuat malas (React.lazy) agar halaman non-SLO tidak menanggung
// ~bundle editor. Gagal muat (mis. JS parsial) -> fallback textarea biasa.
const LazyCodeMirror = lazy(() => import('./SloCodeMirror'));

export function SloEditor({
    initial,
    onSave,
    saving,
    validation,
    validating,
    onValidate,
}: {
    initial: string;
    onSave: (content: string) => void;
    saving: boolean;
    validation: SloValidation | undefined;
    validating: boolean;
    onValidate: (content: string) => void;
}) {
    const { t } = useTranslation();
    const [value, setValue] = useState(initial);
    const [failed, setFailed] = useState(false);
    const dirty = useRef(false);

    // Sinkron dari server (mis. setelah save + refetch) hanya bila pengguna
    // TIDAK sedang mengedit, agar polling/refetch tak menghapus draf.
    useEffect(() => {
        if (!dirty.current) setValue(initial);
    }, [initial]);

    function edit(next: string): void {
        dirty.current = true;
        // Browser menyisipkan NBSP (U+00A0) untuk spasi ganda di
        // contenteditable (terbukti di e2e Chromium); YAML tak pernah butuh
        // NBSP sebagai indentasi, jadi normalkan ke spasi biasa. Tanpa ini
        // indentasi yang diketik diam-diam menjadi kunci YAML tak dikenal.
        setValue(normalizeEditorText(next));
    }

    const issues = validation?.errors ?? [];
    // Identitas stabil: tanpa ini CodeMirror dikonfigurasi ulang tiap render
    // (array baru tiap render) dan seleksi bisa rusak saat mengetik cepat.
    const issueMarks = useMemo(
        () => issues.map((i) => ({ line: i.line, message: i.message })),
        // eslint-disable-next-line react-hooks/exhaustive-deps
        [validation],
    );

    return (
        <div>
            <label htmlFor="slo-editor">{t('slo.editorLabel')}</label>
            <Suspense
                fallback={
                    <textarea
                        id="slo-editor"
                        rows={24}
                        style={{ width: '100%', fontFamily: 'monospace' }}
                        value={value}
                        onChange={(e) => {
                            edit(e.target.value);
                        }}
                    />
                }
            >
                {failed ? (
                    <textarea
                        id="slo-editor"
                        rows={24}
                        style={{ width: '100%', fontFamily: 'monospace' }}
                        value={value}
                        onChange={(e) => {
                            edit(e.target.value);
                        }}
                    />
                ) : (
                    <ErrorBoundary
                        onFail={() => {
                            setFailed(true);
                        }}
                    >
                        <LazyCodeMirror
                            value={value}
                            onChange={edit}
                            issues={issueMarks}
                            label={t('slo.editorLabel')}
                        />
                    </ErrorBoundary>
                )}
            </Suspense>
            <div style={{ display: 'flex', gap: 8, marginTop: 8, flexWrap: 'wrap' }}>
                <button
                    type="button"
                    onClick={() => {
                        onValidate(value);
                    }}
                    disabled={validating}
                >
                    {validating ? t('slo.validating') : t('slo.validate')}
                </button>
                <button
                    type="button"
                    className="btn-primary"
                    onClick={() => {
                        onSave(value);
                    }}
                    disabled={saving}
                >
                    {saving ? t('slo.saving') : t('slo.save')}
                </button>
            </div>
            {validation !== undefined && (
                <div style={{ marginTop: 8 }}>
                    {validation.valid ? (
                        <p role="status" style={{ color: 'var(--ok)' }}>
                            ✓ {t('slo.validOk')}
                        </p>
                    ) : (
                        <div role="alert">
                            <ul style={{ color: 'var(--crit)' }}>
                                {issues.map((e, i) => (
                                    <li key={`e${String(e.line)}-${String(e.column)}-${String(i)}`}>
                                        {t('slo.issueAt', {
                                            line: e.line,
                                            column: e.column,
                                            message: e.message,
                                        })}
                                        {e.field !== undefined && e.field !== ''
                                            ? ` (${e.field})`
                                            : ''}
                                    </li>
                                ))}
                            </ul>
                        </div>
                    )}
                </div>
            )}
        </div>
    );
}

class ErrorBoundary extends Component<{ onFail: () => void; children: ReactNode }> {
    state = { failed: false };

    static getDerivedStateFromError(): { failed: boolean } {
        return { failed: true };
    }

    componentDidCatch(): void {
        this.props.onFail();
    }

    render(): ReactNode {
        if (this.state.failed) return null;
        return this.props.children;
    }
}
