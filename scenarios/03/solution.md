# Solusi Skenario 03 (MENTOR ONLY)

## Setup

ewallet → profil `dup-callback`: setiap charge mengirim callback 2-3x.
Gateway TIDAK dedup (LAB-FLAW, P3) → tiap callback diteruskan sebagai webhook
→ merchant mengkredit tiap webhook → saldo ganda. SLI tetap 100% (tidak ada
alert — pelajaran: tidak semua insiden tertangkap SLO).

## Diagnosis

1. `/state`: order dengan `credits >= 2`, balance kelipatan nominal.
2. `callback_log`: 2-3 baris per transaction_id.
3. Profil bank `dup-callback`.

## Mitigasi

Reset ewallet ke normal. Permanen: idempotency di merchant-sink
(dedup by transaction_id) — di luar scope lab, dicatat sebagai tindak lanjut.

## Verifikasi

- Success ratio > 0.9.
- Jawaban mengandung "duplikat" dan "reset".
