import { z } from 'zod';

// U8: CSP `lab serve` ketat (script-src 'self' 'nonce-…', TANPA 'unsafe-eval').
//
// Zod v4 memprobe `new Function("")` (`allowsEval`) saat MEMBANGUN skema object
// untuk memutuskan apakah JIT dipakai. Probe itu tertangkap sebagai
// `securitypolicyviolation` walau throw-nya ditelan browser. Mode `jitless`
// mematikan probe (keluaran validasi identik, hanya tanpa JIT).
//
// PENTING: `allowsEval` di-cache pada akses PERTAMA. Skema dibuat di module-eval
// (mis. `src/utils/money.ts` membuat ZodObject saat impor), sehingga konfigurasi
// ini HARUS dievaluasi lebih dulu daripada modul mana pun yang membangun skema.
// Karena itu modul ini diimpor sebagai impor PERTAMA di `src/main.tsx`; memanggil
// `z.config` di badan main.tsx sudah terlambat (impor dievaluasi lebih dulu).
// Ref: node_modules/zod/v4/core/util.js `allowsEval`.
z.config({ jitless: true });
