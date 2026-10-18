// U8: tabel lebar tidak boleh membuat halaman overflow horizontal (uji zoom
// 200%/reflow). Wadah ini memberi scroll LOKAL pada tabel; karena area scroll
// harus dapat dijangkau keyboard (WCAG 2.1.1 / axe
// scrollable-region-focusable), wadah diberi tabIndex + peran region + nama
// aksesibel yang unik per tabel.
export function TableScroll({ label, children }: { label: string; children: React.ReactNode }) {
    return (
        <div
            role="region"
            aria-label={label}
            // Area scroll WAJIB dapat difokus keyboard (WCAG 2.1.1 + axe
            // scrollable-region-focusable). jsx-a11y menandai tabIndex pada
            // elemen non-interaktif secara umum; di sini justru disyaratkan.
            // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
            tabIndex={0}
            style={{ maxWidth: '100%', overflowX: 'auto' }}
        >
            {children}
        </div>
    );
}
