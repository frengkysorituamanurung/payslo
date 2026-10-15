# Hari 2 — Golden Signals

## Tujuan

Membaca RED (rate, error, duration) + saturasi dari stack observability.

## Aktivitas

1. Buka Grafana `:3002` → dashboard Payment Overview & Provider Health.
2. Naikkan trafik (`lab traffic set --rate 10`), amati RPS, p99, 5xx.
3. Kembalikan ke 2/s. Lihat log gateway di Explore (Loki) + satu trace di Tempo.
4. Query manual di Prometheus: success rate per provider, p99 Envoy.

## Latihan

- Jelaskan perbedaan panel "HTTP success vs payment success".
- Temukan `payment_transactions_pending` saat normal (harus kosong/nol).

## Pertanyaan refleksi

1. Metrik apa yang pertama kamu lihat saat pager berbunyi? Mengapa?
2. Kapan log lebih berguna dari metrik, dan sebaliknya?

## Output dikumpulkan

- Screenshot dashboard + 2 query PromQL beserta hasilnya.
