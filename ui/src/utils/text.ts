// Normalisasi teks editor: browser menyisipkan NBSP (U+00A0) untuk spasi
// ganda di contenteditable (terbukti di e2e Chromium: indentasi YAML yang
// diketik diam-diam menjadi kunci tak dikenal). YAML tak pernah butuh NBSP
// sebagai indentasi, jadi normalkan ke spasi biasa. Ditulis sebagai escape
// \u00a0 (bukan literal) agar lolos no-irregular-whitespace.
export function normalizeEditorText(text: string): string {
  return text.replace(/\u00a0/g, ' ');
}
