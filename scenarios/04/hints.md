# Hints Skenario 04 (dibaca berurutan via `lab hint 04`)

## Hint 1

Envoy 5xx naik tapi bank-bank sehat dan gateway tidak me-return 500 cepat —
request MENGGANTUNG. Periksa `/readyz` gateway dan log lambatnya.

## Hint 2

Tanya Postgres siapa menahan apa (user `lab_ro` cukup untuk membaca):
cari di `pg_locks` + `pg_stat_activity` lock `AccessExclusiveLock` di tabel
`transactions` beserta query pemegangnya.

## Hint 3

Bebaskan dengan `lab chaos reset` (me-terminate pemegang lock lab) atau
`SELECT pg_terminate_backend(<pid>)` manual. Lalu jawab dengan kata kunci
"lock" dan "terminate".
