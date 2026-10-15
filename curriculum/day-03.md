# Hari 3 — SLI/SLO & Error Budget, validate → plan → apply

## Tujuan

Memahami SLI sebagai rasio, burn-rate multi-window, dan alur SLO-as-code.

## Aktivitas

1. Baca 3 SLO lab di `payslo-workspace/slo/` + `payslo/CAPABILITIES.md`.
2. `payslo validate`, `payslo render`, `payslo plan`, `payslo apply`
   (ke prometheus-local), `payslo drift`.
3. Lihat recording `payslo:slo_error_ratio:*` dan alert `PaySLOBurnRateHigh`
   di Prometheus. Bedakan fast-burn vs slow-burn.
4. Baca contoh produksi 30d di `payslo/templates/` (render-only).

## Latihan

- Ubah (sementara!) satu threshold di copy SLO, render, jelaskan bedanya,
  lalu kembalikan. Jangan commit perubahan itu.
- Jelaskan mengapa PENDING tidak masuk counter (baca komentar sql_exporter).

## Pertanyaan refleksi

1. Mengapa alert memakai dua window (long + short), bukan satu?
2. Apa yang terjadi bila window Datadog 1h dipaksakan? (Lihat warning apply.)

## Output dikumpulkan

- Output `payslo plan` (0 changes setelah apply) + penjelasan burn-rate
  dengan kata-katamu sendiri.
