// U8 — Pemeriksa anggaran bundle (docs/UI-SPEC.md bagian PERFORMA).
//
// Mengukur ukuran GZIP yang benar-benar dimuat saat awal (aset yang dirujuk
// langsung oleh dist/index.html: JS entry + CSS). Chunk lazy (mis. editor
// CodeMirror) TIDAK dihitung sebagai initial, tetapi keberadaan code splitting
// ikut divalidasi (chunk editor tidak boleh dirujuk di index.html).
//
// Pakai: node scripts/check-bundle.mjs   (jalankan SETELAH `npm run build`)
// Keluar non-nol + pesan jelas bila melewati anggaran.

import { gzipSync } from 'node:zlib';
import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join, basename } from 'node:path';

/** Anggaran initial JS (gzip) sesuai UI-SPEC. */
export const JS_INITIAL_GZIP_BUDGET = 350 * 1024;
/** Anggaran initial CSS (gzip) — konservatif, dokumen kecil. */
export const CSS_INITIAL_GZIP_BUDGET = 40 * 1024;

const uiDir = join(dirname(fileURLToPath(import.meta.url)), '..');
const distDir = join(uiDir, 'dist');
const indexPath = join(distDir, 'index.html');

if (!existsSync(indexPath)) {
    console.error(`check-bundle: ${indexPath} tidak ada — jalankan "npm run build" dulu.`);
    process.exit(2);
}

const html = readFileSync(indexPath, 'utf8');

// Aset initial: src/href yang menunjuk ke /assets/*.js atau *.css.
function assetsInHtml(ext) {
    const re = new RegExp(`(?:src|href)="([^"]+\\.${ext})"`, 'g');
    const out = new Set();
    for (const m of html.matchAll(re)) out.add(basename(m[1]));
    return [...out];
}

const scripts = assetsInHtml('js');
const styles = assetsInHtml('css');

if (scripts.length === 0) {
    console.error('check-bundle: tidak menemukan <script src> di dist/index.html.');
    process.exit(2);
}

function gzipSize(name) {
    const p = join(distDir, 'assets', name);
    if (!existsSync(p)) {
        console.error(
            `check-bundle: aset initial ${name} tidak ada di ${join(distDir, 'assets')}.`,
        );
        process.exit(2);
    }
    return gzipSync(readFileSync(p)).length;
}

const rows = [];
let jsTotal = 0;
for (const s of scripts) {
    const gz = gzipSize(s);
    jsTotal += gz;
    rows.push(['JS ', s, gz]);
}
let cssTotal = 0;
for (const c of styles) {
    const gz = gzipSize(c);
    cssTotal += gz;
    rows.push(['CSS', c, gz]);
}

for (const [kind, name, gz] of rows.sort((a, b) => b[2] - a[2])) {
    console.log(`${kind} ${(gz / 1024).toFixed(1).padStart(7)} KB gz  ${name}`);
}
console.log(
    `TOTAL initial: JS ${(jsTotal / 1024).toFixed(1)} KB, CSS ${(cssTotal / 1024).toFixed(1)} KB`,
);

// Validasi code splitting: chunk editor besar tidak boleh jadi initial.
const eager = [...scripts, ...styles].find((n) => /CodeMirror/i.test(n));
if (eager !== undefined) {
    console.error(`check-bundle: chunk "${eager}" dirujuk di index.html (harus lazy/code-split).`);
    process.exit(1);
}

let failed = false;
if (jsTotal > JS_INITIAL_GZIP_BUDGET) {
    console.error(
        `check-bundle: JS initial ${(jsTotal / 1024).toFixed(1)} KB > anggaran ` +
            `${(JS_INITIAL_GZIP_BUDGET / 1024).toFixed(0)} KB (UI-SPEC PERFORMA).`,
    );
    failed = true;
}
if (cssTotal > CSS_INITIAL_GZIP_BUDGET) {
    console.error(
        `check-bundle: CSS initial ${(cssTotal / 1024).toFixed(1)} KB > anggaran ` +
            `${(CSS_INITIAL_GZIP_BUDGET / 1024).toFixed(0)} KB.`,
    );
    failed = true;
}

if (failed) process.exit(1);
console.log('check-bundle: OK (dalam anggaran, code splitting terverifikasi).');
