# Skenario 09 — Game day (blind dispatcher)

> BUKAN skenario mandiri. `09` adalah dispatcher buta: `lab scenario start 09`
> atau `lab scenario random --seed <N>` memilih salah satu skenario 01-08
> secara deterministik (seed sama → skenario sama).

## Cara main

```sh
lab scenario random --seed 7   # mulai skenario buta
# ... kerjakan seperti insiden nyata (tanpa briefing!) ...
lab verify <id-terpilih>
lab scenario stop <id-terpilih>
```

Deteksi dari dasbor (bukan dari briefing!). Aturan main mengikuti skenario
yang terpilih (lihat README-nya SETELAH selesai untuk retrospektif — jangan
mengintip solution.md sebelum verify).

## Determinisme

Daftar skenario diurutkan lalu `seed % jumlah`. Seed yang sama selalu
memilih skenario yang sama (diuji di unit test `same_seed_same_pick`).
