# Solusi Skenario 07 (MENTOR ONLY)

## Setup

ewallet → profil `flaky` error_rate 0.2: ~8% error global (40% trafik × 20%),
mengambang di sekitar threshold page 0.072 → fast-burn (5m/1m) flap,
slow-burn ticket (threshold 0.01) menyala mantap.

## Diagnosis

1. `ALERTS`: `window="5m"` flap (pending↔firing), `window="1h"` stabil.
2. Recording `payslo:slo_error_ratio:payment_success_rate:5m` berosilasi.
3. Akar: ewallet flaky — bukan outage, melainkan error kecil intermiten.

## Mitigasi

Reset ewallet ke normal (obati akar, bukan alert).

## Tuning (diusulkan di jawaban, tidak diterapkan di lab)

- Naikkan burn page 14.4 → 20, atau
- Paging hanya bila fast DAN slow burn menyala (AND dua window), atau
- Tambah `for` lebih panjang pada fast-burn.

## Verifikasi

- Success ratio > 0.9.
- Jawaban mengandung "window" dan ("threshold" + angka).
