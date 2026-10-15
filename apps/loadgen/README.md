# loadgen (k6)

Generator trafik sintetis. Campuran channel: VA 60%, EWALLET 30%, QRIS 10%.
Nominal acak integer IDR 10.000–1.000.000 (kelipatan 500). `order_id` unik
per iterasi (`load-<vu>-<ts>-<iter>`). Mengharapkan HTTP 200 (P2: 200 ≠ sukses).

## Env var (k6 `__ENV`)

| Var | Default | Arti |
|---|---|---|
| `GATEWAY_URL` | http://gateway-api:3000 | Target (di compose: nama service) |
| `RATE_PER_SEC` | 2 | Laju arrival konstan (req/s) |

## Cara run

```sh
# via compose (profil traffic): RATE_PER_SEC=5 make up-lite
# manual: k6 run -e GATEWAY_URL=http://127.0.0.1:3000 -e RATE_PER_SEC=2 apps/loadgen/script.js
```
