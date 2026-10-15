import { useEffect, useMemo, useRef } from 'react';
import CodeMirror from '@uiw/react-codemirror';
import { yaml } from '@codemirror/lang-yaml';
import { Decoration, EditorView, type DecorationSet } from '@codemirror/view';
import { RangeSetBuilder, StateEffect, StateField } from '@codemirror/state';

// Efek: ganti himpunan baris bermasalah. Dekorasi = latar baris (kelas CSS
// di bawah) sehingga lokasi error terlihat di editor, bukan hanya di daftar.
const setIssueLines = StateEffect.define<number[]>();
const issueLineDeco = Decoration.line({ class: 'cm-issue-line' });

const issueField = StateField.define<DecorationSet>({
    create: () => Decoration.none,
    update(deco, tr) {
        for (const e of tr.effects) {
            if (e.is(setIssueLines)) {
                const build = new RangeSetBuilder<Decoration>();
                for (const ln of e.value) {
                    if (ln < 1 || ln > tr.state.doc.lines) continue;
                    const line = tr.state.doc.line(ln);
                    build.add(line.from, line.from, issueLineDeco);
                }
                return build.finish();
            }
        }
        return deco.map(tr.changes);
    },
    provide: (f) => EditorView.decorations.from(f),
});

const issueTheme = EditorView.baseTheme({
    '.cm-issue-line': { background: 'var(--crit-bg)' },
});

export default function SloCodeMirror({
    value,
    onChange,
    issues,
    label,
}: {
    value: string;
    onChange: (v: string) => void;
    issues: { line: number; message: string }[];
    label: string;
}) {
    // Ekstensi stabil per label bahasa (ganti bahasa jarang; tidak tiap
    // ketikan, agar seleksi tak rusak). contentAttributes memberi nama
    // aksesibel pada area edit (axe aria-input-field-name).
    const extensions = useMemo(
        () => [
            yaml(),
            issueTheme,
            issueField,
            EditorView.lineWrapping,
            EditorView.contentAttributes.of({ 'aria-label': label }),
        ],
        [label],
    );
    const viewRef = useRef<EditorView | null>(null);
    // Kunci stabil untuk efek (array issues baru tiap render React).
    const linesKey = issues.map((i) => i.line).join(',');

    useEffect(() => {
        const lines = linesKey === '' ? [] : linesKey.split(',').map(Number);
        viewRef.current?.dispatch({ effects: setIssueLines.of(lines) });
    }, [linesKey]);

    return (
        <div id="slo-editor-wrap">
            <CodeMirror
                value={value}
                height="480px"
                extensions={extensions}
                basicSetup={{ lineNumbers: true, foldGutter: true }}
                onChange={onChange}
                onCreateEditor={(view) => {
                    viewRef.current = view;
                    const lines = linesKey === '' ? [] : linesKey.split(',').map(Number);
                    if (lines.length > 0) view.dispatch({ effects: setIssueLines.of(lines) });
                }}
            />
        </div>
    );
}
