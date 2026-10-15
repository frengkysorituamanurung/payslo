// PaySLO Lab load generator (k6). Env: GATEWAY_URL, RATE_PER_SEC.
// Mix: VA 60% / EWALLET 30% / QRIS 10%. Amounts: random int IDR.
// order_id unique per iteration. Expects HTTP 200 (P2: 200 != success).
import http from 'k6/http';
import { check } from 'k6';

const GATEWAY = __ENV.GATEWAY_URL || 'http://gateway-api:3000';
const RATE = parseInt(__ENV.RATE_PER_SEC || '2', 10);

export const options = {
  scenarios: {
    payments: {
      executor: 'constant-arrival-rate',
      rate: RATE,
      timeUnit: '1s',
      duration: '30m',
      preAllocatedVUs: Math.max(2, RATE),
      maxVUs: Math.max(10, RATE * 5),
    },
  },
};

function pickChannel() {
  const r = Math.random();
  if (r < 0.6) return 'VA';
  if (r < 0.9) return 'EWALLET';
  return 'QRIS';
}

function randAmount() {
  // Integer IDR 10_000..1_000_000 rounded to 500 (P7: integer money).
  const raw = 10000 + Math.floor(Math.random() * 990001);
  return Math.round(raw / 500) * 500;
}

export default function () {
  const orderId = `load-${__VU}-${Date.now()}-${__ITER}`;
  const payload = JSON.stringify({
    merchant_id: 'merchant-loadgen',
    order_id: orderId,
    amount: randAmount(),
    channel: pickChannel(),
  });
  const res = http.post(`${GATEWAY}/v1/payments`, payload, {
    headers: { 'Content-Type': 'application/json' },
    timeout: '15s',
  });
  check(res, { 'gateway accepts (200)': (r) => r.status === 200 });
}
