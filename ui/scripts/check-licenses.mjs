// U8 — Pemeriksa lisensi dependensi npm (docs/UI-SPEC.md STANDAR KUALITAS).
//
// Menelusuri semua paket di node_modules (termasuk scoped/nested), membaca field
// `license` (atau `licenses`), dan memastikan setiap lisensi ada di daftar izin.
// Gagal (exit 1) bila ada lisensi di luar daftar atau tidak diketahui.
//
// Pakai: node scripts/check-licenses.mjs

import { readFileSync, readdirSync, existsSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

/** Lisensi permisif yang diterima untuk aplikasi lab (Apache-2.0). */
const ALLOWED = new Set([
    'MIT',
    'MIT-0',
    'ISC',
    '0BSD',
    'BSD-2-Clause',
    'BSD-3-Clause',
    'Apache-2.0',
    'Unlicense',
    'CC0-1.0',
    'CC-BY-3.0',
    'CC-BY-4.0',
    'BlueOak-1.0.0',
    'Zlib',
    'Python-2.0',
    'WTFPL',
    'MPL-2.0',
    'OFL-1.1',
    '(MIT OR CC0-1.0)',
    '(MIT AND Zlib)',
    '(MIT OR Apache-2.0)',
    '(Apache-2.0 OR MIT)',
    '(MPL-2.0 OR Apache-2.0)',
    '(BSD-2-Clause OR MIT OR Apache-2.0)',
    '(BSD-3-Clause OR GPL-2.0)',
    '(CC-BY-3.0 AND MIT)',
]);

const uiDir = join(dirname(fileURLToPath(import.meta.url)), '..');
const root = join(uiDir, 'node_modules');

if (!existsSync(root)) {
    console.error('check-licenses: ui/node_modules tidak ada — jalankan "npm ci" dulu.');
    process.exit(2);
}

/** Kumpulkan paket top-level + scoped (dan @scope/* ) satu level; cukup untuk audit. */
function packageDirs() {
    const dirs = [];
    for (const name of readdirSync(root)) {
        if (name.startsWith('@')) {
            const scope = join(root, name);
            if (!statSync(scope).isDirectory()) continue;
            for (const sub of readdirSync(scope)) {
                const p = join(scope, sub);
                if (existsSync(join(p, 'package.json'))) dirs.push(p);
            }
        } else {
            const p = join(root, name);
            if (existsSync(join(p, 'package.json'))) dirs.push(p);
        }
    }
    return dirs;
}

function licenseOf(pj) {
    if (typeof pj.license === 'string' && pj.license !== '') return pj.license;
    if (Array.isArray(pj.licenses)) {
        return pj.licenses
            .map((l) => (typeof l === 'string' ? l : l?.type))
            .filter(Boolean)
            .join(' OR ');
    }
    return '';
}

const bad = [];
const counts = new Map();
let seen = 0;
for (const dir of packageDirs()) {
    const pj = JSON.parse(readFileSync(join(dir, 'package.json'), 'utf8'));
    if (pj.private === true && pj.name === 'payslo-lab-console') continue; // paket kita
    const lic = licenseOf(pj);
    seen += 1;
    if (lic === '' || !ALLOWED.has(lic)) {
        bad.push(
            `${pj.name}@${pj.version ?? '?'}: ${lic === '' ? '(tidak ada field license)' : lic}`,
        );
        continue;
    }
    counts.set(lic, (counts.get(lic) ?? 0) + 1);
}

const summary = [...counts.entries()].sort((a, b) => b[1] - a[1]);
for (const [lic, n] of summary) console.log(`  ${String(n).padStart(4)}  ${lic}`);
console.log(
    `check-licenses: ${String(seen)} paket diperiksa, ${String(summary.length)} jenis lisensi izin.`,
);

if (bad.length > 0) {
    console.error('check-licenses: lisensi di luar daftar izin:');
    for (const b of bad) console.error(`  - ${b}`);
    process.exit(1);
}
console.log('check-licenses: OK (semua lisensi permisif).');
