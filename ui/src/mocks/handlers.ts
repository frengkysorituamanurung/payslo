import { http, HttpResponse } from 'msw';
import type {
  Approval,
  AuditEntry,
  BankStatus,
  CapabilityMatrix,
  IncidentDetail,
  PaymentDetail,
  PaymentList,
  PaymentSummary,
  Postmortem,
  RoutingConfig,
  Session,
  SloDetail,
  SloPlan,
  SloValidation,
  StackHealth,
  TrafficState,
} from '../api/client';
import { errorBudgetSecs } from '../utils/budget';

export const demoSession: Session = {
  mode: 'trainee',
  traineeName: null,
  scenario: null,
  labUp: true,
  checkedAt: '2026-10-16T07:00:00Z',
};

// U6: state mentor (token mock — bukan keamanan) + learn progress.
let mentorUnlocked = false;
const mentorToken = 'mock-mentor-token';
const learnProgressState = new Map<string, boolean>();

export const demoStack: StackHealth = {
  labUp: true,
  checkedAt: '2026-10-16T07:00:00Z',
  services: [
    { name: 'postgres', state: 'up' },
    { name: 'gateway-api', state: 'up' },
    { name: 'envoy', state: 'up' },
  ],
};

function mv(status: string, value: number | null, rate: number | null) {
  return {
    status,
    value,
    sampleRate: rate,
    lastDataAt: status === 'ok' ? '2026-10-16T07:00:00Z' : null,
  };
}

export const demoSummary = {
  checkedAt: '2026-10-16T07:00:00Z',
  window: '5m',
  metrics: {
    http_rps: mv('ok', 2.1, 2.1),
    http_2xx_ratio: mv('ok', 0.999, 2.1),
    http_5xx_ratio: mv('ok', 0.0, 2.1),
    http_p99_ms: mv('ok', 180, 2.1),
    payment_rps: mv('ok', 1.9, 1.9),
    payment_success_ratio: mv('ok', 0.985, 1.9),
    'payment_success_ratio_by_provider:bca': mv('ok', 0.99, 0.8),
    'payment_success_ratio_by_provider:mandiri': mv('ok', 0.98, 0.6),
    'payment_success_ratio_by_provider:ewallet': mv('ok', 0.995, 0.5),
    payment_pending_stuck: mv('ok', 0, null),
    webhook_delivery_ratio: mv('ok', 1.0, 1.9),
  },
};

function demoPoints(n = 12): Array<[string, number | null]> {
  const pts: Array<[string, number | null]> = [];
  for (let i = 0; i < n; i++) {
    const ts = new Date(Date.parse('2026-10-16T07:00:00Z') - (n - 1 - i) * 15000).toISOString();
    pts.push([ts, 0.98 + 0.001 * Math.sin(i)]);
  }
  return pts;
}

// ------------------------------------------------------------------ U2 state
// State in-memory yang DIMUTASI oleh handler mutasi (PUT/POST) agar test UI
// bisa memverifikasi round-trip (UI -> API -> UI). Direset di test setup.

const BASE = '2026-10-16T07:00:00Z';

export const P_BCA_1 = '018f0a2e-1c3d-7a4b-8e5f-000000000001';
export const P_MANDIRI_2 = '018f0a2e-1c3d-7a4b-8e5f-000000000002';
export const P_DUP_4 = '018f0a2e-1c3d-7a4b-8e5f-000000000004';

function summary(
  partial: Partial<PaymentSummary> & Pick<PaymentSummary, 'id' | 'orderId'>,
): PaymentSummary {
  return {
    merchantId: 'merchant-demo',
    channel: 'VA',
    provider: 'bca',
    amount: 150_000,
    fee: 4000,
    status: 'PENDING',
    providerRef: null,
    createdAt: BASE,
    updatedAt: BASE,
    expiresAt: BASE,
    callbackCount: 0,
    webhookDelivered: false,
    duplicateCallback: false,
    ...partial,
  };
}

function fixtures(): {
  payments: PaymentSummary[];
  details: Record<string, PaymentDetail>;
  audit: AuditEntry[];
  routing: RoutingConfig;
  traffic: TrafficState;
  release: 'good' | 'bad';
} {
  const p1 = summary({ id: P_BCA_1, orderId: 'demo-order-001', providerRef: 'P-BCA-0001' });
  const p2 = summary({
    id: P_MANDIRI_2,
    orderId: 'demo-order-002',
    provider: 'mandiri',
    amount: 1_250_000,
    status: 'SUCCESS',
    providerRef: 'P-MANDIRI-0002',
    callbackCount: 1,
    webhookDelivered: true,
  });
  const p4 = summary({
    id: P_DUP_4,
    orderId: 'demo-order-004',
    provider: 'bca',
    amount: 2_000_000,
    status: 'SUCCESS',
    providerRef: 'P-BCA-0004',
    callbackCount: 2,
    webhookDelivered: true,
    duplicateCallback: true,
  });
  const payments: PaymentSummary[] = [p1, p2, p4];

  const details: Record<string, PaymentDetail> = {
    [P_BCA_1]: {
      payment: p1,
      timeline: [
        { type: 'created', at: BASE, derived: false },
        {
          type: 'pending',
          at: null,
          derived: true,
          detail: 'diterima gateway, menunggu callback/expiry',
        },
      ],
      callbackCount: 0,
      webhookCount: 0,
      duplicateCallback: false,
      traceId: null,
      links: { log: null, trace: null },
    },
    [P_MANDIRI_2]: {
      payment: p2,
      timeline: [
        { type: 'created', at: BASE, derived: false },
        {
          type: 'pending',
          at: null,
          derived: true,
          detail: 'diterima gateway, menunggu callback/expiry',
        },
        { type: 'callback', at: BASE, derived: false, provider: 'mandiri', status: 'SUCCESS' },
        { type: 'final', at: BASE, derived: false, status: 'SUCCESS' },
        {
          type: 'webhook',
          at: BASE,
          derived: false,
          detail: 'webhook DELIVERED',
          attempt: 1,
          httpStatus: 200,
        },
      ],
      callbackCount: 1,
      webhookCount: 1,
      duplicateCallback: false,
      traceId: null,
      links: { log: null, trace: null },
    },
    [P_DUP_4]: {
      payment: p4,
      timeline: [
        { type: 'created', at: BASE, derived: false },
        {
          type: 'pending',
          at: null,
          derived: true,
          detail: 'diterima gateway, menunggu callback/expiry',
        },
        { type: 'callback', at: BASE, derived: false, provider: 'bca', status: 'SUCCESS' },
        { type: 'callback', at: BASE, derived: false, provider: 'bca', status: 'SUCCESS' },
        { type: 'final', at: BASE, derived: false, status: 'SUCCESS' },
        {
          type: 'webhook',
          at: BASE,
          derived: false,
          detail: 'webhook DELIVERED',
          attempt: 1,
          httpStatus: 200,
        },
        {
          type: 'webhook',
          at: BASE,
          derived: false,
          detail: 'webhook DELIVERED',
          attempt: 1,
          httpStatus: 200,
        },
      ],
      callbackCount: 2,
      webhookCount: 2,
      duplicateCallback: true,
      traceId: null,
      links: { log: null, trace: null },
    },
  };

  return {
    payments,
    details,
    audit: [
      {
        ts: BASE,
        actor: 'sinta',
        action: 'routing.set',
        target: 'gateway',
        result: 'ok',
        detail: 'bca=false/w60',
      },
      {
        ts: BASE,
        actor: 'trainee',
        action: 'traffic.set',
        target: '3',
        result: 'ok',
        detail: 'rate=3/s',
      },
    ],
    routing: {
      bca: { enabled: true, weight: 60 },
      mandiri: { enabled: true, weight: 40 },
      ewallet: { enabled: true, weight: 100 },
    },
    traffic: { ratePerSec: 2, source: 'loadgen', checkedAt: BASE },
    release: 'good',
  };
}

const initial = fixtures();
export let mockState = initial;
export function resetMockState(): void {
  mockState = fixtures();
  resetSloState();
  resetIncidentState();
  mentorUnlocked = false;
  learnProgressState.clear();
}

function problem(
  status: number,
  title: string,
  detail: string,
  errors?: { field: string; message: string }[],
) {
  return HttpResponse.json(
    {
      type: 'https://payslo-lab.local/problems/request-rejected',
      title,
      status,
      detail,
      ...(errors === undefined ? {} : { errors }),
    },
    { status, headers: { 'Content-Type': 'application/problem+json' } },
  );
}

export const handlers = [
  http.get('*/api/v1/session', () =>
    HttpResponse.json({ ...demoSession, mode: mentorUnlocked ? 'mentor' : 'trainee' }),
  ),
  http.get('*/api/v1/health/stack', () => HttpResponse.json(demoStack)),
  http.get('*/api/v1/metrics/summary', () => HttpResponse.json(demoSummary)),
  http.get('*/api/v1/metrics/series', () =>
    HttpResponse.json({
      name: 'payment_success_ratio',
      step: '15s',
      status: 'ok',
      lastDataAt: '2026-10-16T07:00:00Z',
      points: demoPoints(),
    }),
  ),

  // U2: payments
  http.get('*/api/v1/payments', ({ request }) => {
    const url = new URL(request.url);
    const status = url.searchParams.get('status');
    const provider = url.searchParams.get('provider');
    const channel = url.searchParams.get('channel');
    const items = mockState.payments.filter(
      (p) =>
        (status === null || p.status === status) &&
        (provider === null || p.provider === provider) &&
        (channel === null || p.channel === channel),
    );
    const body: PaymentList = {
      checkedAt: BASE,
      dataStatus: 'ok',
      hasMore: false,
      nextCursor: null,
      items,
    };
    return HttpResponse.json(body);
  }),
  http.post('*/api/v1/payments', async ({ request }) => {
    const body = (await request.json()) as {
      merchantId: string;
      orderId: string;
      amount: number;
      channel: PaymentSummary['channel'];
    };
    if (!Number.isSafeInteger(body.amount) || body.amount < 1) {
      return problem(400, 'Validasi gagal', 'amount harus integer positif', [
        { field: 'amount', message: 'amount harus integer positif' },
      ]);
    }
    const seq = mockState.payments.length + 1;
    const created = summary({
      id: `018f0a2e-1c3d-7a4b-8e5f-${String(900000000000 + seq)}`,
      orderId: body.orderId,
      merchantId: body.merchantId,
      amount: body.amount,
      channel: body.channel,
      provider: body.channel === 'VA' ? 'bca' : 'ewallet',
      providerRef: `P-DEMO-${String(seq)}`,
    });
    mockState.payments = [created, ...mockState.payments];
    mockState.details[created.id] = {
      payment: created,
      timeline: [
        { type: 'created', at: BASE, derived: false },
        {
          type: 'pending',
          at: null,
          derived: true,
          detail: 'diterima gateway, menunggu callback/expiry',
        },
      ],
      callbackCount: 0,
      webhookCount: 0,
      duplicateCallback: false,
      traceId: null,
      links: { log: null, trace: null },
    };
    return HttpResponse.json(created, { status: 201 });
  }),
  http.get('*/api/v1/payments/:id/bank-status', ({ params }) => {
    const detail = mockState.details[String(params['id'])];
    if (detail === undefined) return problem(404, 'Tidak ditemukan', 'transaksi tidak ada');
    if (detail.payment.providerRef === null)
      return problem(409, 'Belum ada provider_ref', 'transaksi belum punya provider_ref');
    const body: BankStatus = {
      provider: detail.payment.provider,
      providerRef: detail.payment.providerRef,
      status: detail.payment.status === 'SUCCESS' ? 'SUCCESS' : 'CHARGED',
      source: 'bank',
      checkedAt: BASE,
    };
    return HttpResponse.json(body);
  }),
  http.get('*/api/v1/payments/:id', ({ params }) => {
    const detail = mockState.details[String(params['id'])];
    if (detail === undefined) return problem(404, 'Tidak ditemukan', 'transaksi tidak ada');
    return HttpResponse.json(detail);
  }),

  // U2: control
  http.get('*/api/v1/traffic', () => HttpResponse.json(mockState.traffic)),
  http.put('*/api/v1/traffic', async ({ request }) => {
    const body = (await request.json()) as TrafficState;
    mockState.traffic = { ratePerSec: body.ratePerSec, source: 'loadgen', checkedAt: BASE };
    mockState.audit = [
      {
        ts: BASE,
        actor: 'trainee',
        action: 'traffic.set',
        target: String(body.ratePerSec),
        result: 'ok',
      },
      ...mockState.audit,
    ];
    return HttpResponse.json(mockState.traffic);
  }),
  http.get('*/api/v1/routing', () => HttpResponse.json(mockState.routing)),
  http.put('*/api/v1/routing', async ({ request }) => {
    const body = (await request.json()) as RoutingConfig;
    mockState.routing = body;
    mockState.audit = [
      { ts: BASE, actor: 'trainee', action: 'routing.set', target: 'gateway', result: 'ok' },
      ...mockState.audit,
    ];
    return HttpResponse.json(mockState.routing);
  }),
  http.put('*/api/v1/release', async ({ request }) => {
    const body = (await request.json()) as { version: 'good' | 'bad' };
    mockState.release = body.version;
    mockState.audit = [
      { ts: BASE, actor: 'trainee', action: 'release.set', target: 'gateway', result: 'ok' },
      ...mockState.audit,
    ];
    return HttpResponse.json({ version: body.version });
  }),
  http.get('*/api/v1/audit', ({ request }) => {
    const url = new URL(request.url);
    const limit = Number(url.searchParams.get('limit') ?? '20');
    return HttpResponse.json({ entries: mockState.audit.slice(0, limit) });
  }),

  // U3: SLO Center (mencerminkan demoSLO Go: save -> validate -> plan -> apply -> no-op).
  http.get('*/api/v1/slos', () => HttpResponse.json({ slos: sloSummaries(), checkedAt: BASE })),
  http.get('*/api/v1/slos/:name', ({ params }) => {
    const d = sloDetailOf(String(params['name']));
    if (d === undefined) return problem(404, 'Tidak ditemukan', 'SLO tidak ada');
    return HttpResponse.json(d);
  }),
  http.put('*/api/v1/slos/:name', async ({ params, request }) => {
    const name = String(params['name']);
    if (sloState.files[name] === undefined) return problem(404, 'Tidak ditemukan', 'SLO tidak ada');
    const body = (await request.json()) as { content: string };
    sloState.files[name] = body.content;
    mockState.audit = [
      { ts: BASE, actor: 'trainee', action: 'slo.save', target: name, result: 'ok' },
      ...mockState.audit,
    ];
    return HttpResponse.json({ name, file: `${name}.yml`, bytes: body.content.length });
  }),
  http.post('*/api/v1/slos/:name/validate', async ({ params, request }) => {
    const name = String(params['name']);
    let content: string | undefined;
    try {
      const body = (await request.json()) as { content?: string };
      content = body.content;
    } catch {
      content = undefined;
    }
    const text = content ?? sloState.files[name];
    if (text === undefined) return problem(404, 'Tidak ditemukan', 'SLO tidak ada');
    const errors = validateSloText(text);
    const body: SloValidation = { valid: errors.length === 0, errors };
    return HttpResponse.json(body);
  }),
  http.post('*/api/v1/slos/:name/plan', ({ params }) => {
    const name = String(params['name']);
    if (sloState.files[name] === undefined) return problem(404, 'Tidak ditemukan', 'SLO tidak ada');
    const current = sloState.files[name] ?? '';
    const issues = validateSloText(current);
    if (issues.length > 0) {
      const first = issues[0]?.message ?? 'tidak valid';
      return problem(400, 'SLO tidak valid', `SLO tidak valid: ${first}`);
    }
    return HttpResponse.json(sloPlanOf(name));
  }),
  http.post('*/api/v1/slos/:name/apply', async ({ params, request }) => {
    const name = String(params['name']);
    if (sloState.files[name] === undefined) return problem(404, 'Tidak ditemukan', 'SLO tidak ada');
    const current = sloState.files[name] ?? '';
    const issues = validateSloText(current);
    if (issues.length > 0) {
      const first = issues[0]?.message ?? 'tidak valid';
      return problem(400, 'SLO tidak valid', `SLO tidak valid: ${first}`);
    }
    let connections: string[] = [];
    try {
      connections = ((await request.json()) as { connections?: string[] }).connections ?? [];
    } catch {
      connections = [];
    }
    const targets = SLO_CONNS.filter(
      (c) => connections.length === 0 || connections.includes(c.name),
    );
    if (targets.length === 0) return problem(400, 'Koneksi buruk', 'unknown connection');
    const changed = sloState.files[name] !== sloState.applied[name];
    const results = targets.map((c) => ({
      slo: name,
      connection: c.name,
      backend: c.backend,
      applied: changed,
      noOp: !changed,
      message: changed
        ? `applied ${name} on ${c.name}`
        : `up-to-date ${name} on ${c.name} (no changes)`,
    }));
    if (changed) sloState.applied[name] = sloState.files[name] ?? '';
    mockState.audit = [
      { ts: BASE, actor: 'trainee', action: 'slo.apply', target: name, result: 'ok' },
      ...mockState.audit,
    ];
    return HttpResponse.json({ slo: name, results, warnings: sloWarnings(name), raw: '' });
  }),
  http.get('*/api/v1/drift', () =>
    HttpResponse.json({
      drifted: [
        {
          slo: 'payment-latency-p99',
          connection: 'datadog-mock',
          reason: 'backend differs from last apply',
        },
      ],
      clean: false,
      raw: 'drift: payment-latency-p99 on datadog-mock (backend differs from last apply)',
    }),
  ),
  http.get('*/api/v1/capabilities', () => {
    const body: CapabilityMatrix = { features: SLO_CAPS, raw: '' };
    return HttpResponse.json(body);
  }),

  // U4: insiden/approval/runbook/postmortem/settings (cerminkan demo Go).
  http.get('*/api/v1/incidents', () =>
    HttpResponse.json({ incidents: incidentList(), checkedAt: BASE }),
  ),
  http.get('*/api/v1/incidents/:id', ({ params }) => {
    const d = incidentDetailOf(String(params['id']));
    if (d === undefined) return problem(404, 'Tidak ditemukan', 'insiden tidak ada');
    return HttpResponse.json(d);
  }),
  http.post('*/api/v1/incidents/:id/notes', async ({ params, request }) => {
    const id = String(params['id']);
    const body = (await request.json()) as { text?: string };
    if ((body.text ?? '').trim() === '')
      return problem(400, 'Catatan buruk', 'teks catatan kosong');
    const notes = incidentState.timeline[id];
    if (notes === undefined) return problem(404, 'Tidak ditemukan', 'insiden tidak ada');
    notes.push({
      ts: BASE,
      kind: 'note',
      actor: 'trainee',
      message: (body.text ?? '').trim(),
    });
    return HttpResponse.json({ ok: true, incident_id: id }, { status: 201 });
  }),
  http.get('*/api/v1/incidents/:id/approvals', ({ params, request }) => {
    const id = String(params['id']);
    if (incidentState.timeline[id] === undefined)
      return problem(404, 'Tidak ditemukan', 'insiden tidak ada');
    const status = new URL(request.url).searchParams.get('status');
    return HttpResponse.json({
      approvals: incidentState.approvals.filter(
        (a) => a.incidentId === id && (status === null || a.status === status),
      ),
    });
  }),
  http.post('*/api/v1/incidents/:id/approvals', async ({ params, request }) => {
    const id = String(params['id']);
    const body = (await request.json()) as { action?: string; step?: number };
    const action = (body.action ?? '').trim();
    if (!isRemediationAction(action))
      return problem(400, 'Aksi buruk', `aksi bukan langkah remediation runbook`);
    const reqs = incidentState.timeline[id];
    if (reqs === undefined) return problem(404, 'Tidak ditemukan', 'insiden tidak ada');
    const existing = incidentState.approvals.find(
      (a) => a.incidentId === id && a.action === action && a.status === 'pending',
    );
    if (existing !== undefined)
      return HttpResponse.json({ approval: existing, deduped: true }, { status: 201 });
    incidentState.seq += 1;
    const ap: Approval = {
      id: `apr-demo-${String(incidentState.seq).padStart(4, '0')}`,
      executionId: `exe-demo-${String(incidentState.seq).padStart(4, '0')}`,
      stepIdx: body.step ?? 4,
      incidentId: id,
      action,
      status: 'pending',
      requestedAt: BASE,
    };
    incidentState.approvals.push(ap);
    reqs.push({
      ts: BASE,
      kind: 'approval_requested',
      actor: 'trainee',
      message: `approval requested: ${action}`,
    });
    mockState.audit = [
      { ts: BASE, actor: 'trainee', action: 'approval.request', target: ap.id, result: 'ok' },
      ...mockState.audit,
    ];
    return HttpResponse.json({ approval: ap, deduped: false }, { status: 201 });
  }),
  http.get('*/api/v1/approvals', ({ request }) => {
    const status = new URL(request.url).searchParams.get('status');
    return HttpResponse.json({
      approvals: incidentState.approvals.filter((a) => status === null || a.status === status),
    });
  }),
  http.post('*/api/v1/approvals/:aid/decision', async ({ params, request }) => {
    const aid = String(params['aid']);
    const ap = incidentState.approvals.find((a) => a.id === aid);
    if (ap === undefined) return problem(404, 'Tidak ditemukan', 'approval tidak ada');
    const body = (await request.json()) as { decision?: string; by?: string; reason?: string };
    const decision = (body.decision ?? '').trim().toLowerCase();
    const by = (body.by ?? '').trim() || incidentState.settings.traineeName;
    if (decision !== 'approved' && decision !== 'rejected')
      return problem(422, 'Keputusan buruk', 'decision must be approved or rejected');
    if (by === '') return problem(400, 'Approver buruk', 'identitas approver wajib');
    if (decision === 'rejected' && (body.reason ?? '').trim() === '')
      return problem(422, 'Alasan wajib', 'reason is required when rejecting');
    if (ap.status !== 'pending')
      return problem(409, 'Sudah diputus', `approval already decided (${ap.status})`);
    ap.status = decision;
    ap.decidedAt = BASE;
    ap.decidedBy = by;
    if ((body.reason ?? '').trim() !== '') ap.reason = (body.reason ?? '').trim();
    const decs = incidentState.timeline[ap.incidentId];
    if (decs === undefined) return problem(404, 'Tidak ditemukan', 'insiden tidak ada');
    decs.push({
      ts: BASE,
      kind: `approval_${decision}`,
      actor: by,
      message: `approval ${decision} by ${by}: ${ap.action}`,
    });
    if (decision === 'approved') {
      incidentState.bcaDisabled = true;
      const rems = incidentState.timeline[ap.incidentId];
      if (rems === undefined) return problem(404, 'Tidak ditemukan', 'insiden tidak ada');
      rems.push({
        ts: BASE,
        kind: 'remediation',
        actor: by,
        message: `remediation executed by ${by}: ${ap.action} (bca disabled, demo)`,
      });
    }
    mockState.audit = [
      { ts: BASE, actor: by, action: 'approval.decide', target: ap.id, result: 'ok' },
      ...mockState.audit,
    ];
    return HttpResponse.json({ approval: ap });
  }),
  http.post('*/api/v1/incidents/:id/runbook/steps/:idx/run', ({ params }) => {
    const id = String(params['id']);
    const idx = Number(params['idx']);
    if (incidentState.timeline[id] === undefined)
      return problem(404, 'Tidak ditemukan', 'insiden tidak ada');
    const step = incidentSteps()[idx];
    if (step === undefined) return problem(404, 'Tidak ditemukan', 'langkah tidak ada');
    if (step.kind === 'remediation')
      return problem(409, 'Butuh approval', 'langkah remediation butuh approval PaySLO dulu');
    if (step.kind !== 'diagnostic')
      return problem(400, 'Langkah manual', 'langkah manual tak dapat dijalankan');
    return HttpResponse.json({
      status: 'ok',
      detail: 'demo output',
      values: { bca: 0.12, mandiri: 0.99 },
    });
  }),
  http.get('*/api/v1/incidents/:id/postmortem', ({ params }) => {
    const id = String(params['id']);
    if (incidentState.timeline[id] === undefined)
      return problem(404, 'Tidak ditemukan', 'insiden tidak ada');
    return HttpResponse.json(postmortemOf(id));
  }),
  http.put('*/api/v1/incidents/:id/postmortem', async ({ params, request }) => {
    const id = String(params['id']);
    if (incidentState.timeline[id] === undefined)
      return problem(404, 'Tidak ditemukan', 'insiden tidak ada');
    const body = (await request.json()) as {
      status?: string;
      sections?: Postmortem['sections'];
    };
    if (body.status !== 'draft' && body.status !== 'published')
      return problem(400, 'Status buruk', 'status harus draft atau published');
    incidentState.postmortem[id] = {
      incidentId: id,
      status: body.status,
      sections: {
        summary: '',
        impact: '',
        timeline: '',
        rootCause: '',
        wentWell: '',
        wentWrong: '',
        actions: [],
        ...(body.sections ?? {}),
      },
      updatedAt: BASE,
      updatedBy: 'trainee',
      markdown: '',
      exists: true,
    };
    const doc = postmortemOf(id);
    mockState.audit = [
      { ts: BASE, actor: 'trainee', action: 'postmortem.save', target: id, result: 'ok' },
      ...mockState.audit,
    ];
    return HttpResponse.json(doc);
  }),
  http.get('*/api/v1/settings', () => HttpResponse.json(incidentState.settings)),
  http.put('*/api/v1/settings', async ({ request }) => {
    const body = (await request.json()) as { traineeName?: string; lowSampleThreshold?: number };
    if (body.traineeName !== undefined) {
      const name = body.traineeName.trim();
      if (name === '' || name.length > 64)
        return problem(400, 'Pengaturan buruk', 'nama trainee 1..64 karakter');
      incidentState.settings.traineeName = name;
    }
    if (body.lowSampleThreshold !== undefined) {
      const v = body.lowSampleThreshold;
      if (!Number.isFinite(v) || v < 0 || v > 10)
        return problem(400, 'Pengaturan buruk', 'ambang harus 0..10');
      incidentState.settings.lowSampleThreshold = v;
    }
    return HttpResponse.json(incidentState.settings);
  }),

  // ------------------------------------------------------------------ U7 observability + diagnostik
  http.get('*/api/v1/observe/status', () =>
    HttpResponse.json({
      services: [
        { name: 'grafana', reachable: true, latencyMs: 3 },
        { name: 'prometheus', reachable: true, latencyMs: 2 },
        { name: 'alertmanager', reachable: true, latencyMs: 2 },
        { name: 'ntfy', reachable: true, latencyMs: 4 },
        {
          name: 'loki',
          reachable: false,
          latencyMs: 1,
          detail: 'profil obs-full tidak aktif (demo)',
        },
        {
          name: 'tempo',
          reachable: false,
          latencyMs: 1,
          detail: 'profil obs-full tidak aktif (demo)',
        },
      ],
      dockerAvailable: false,
      dockerNote: 'Docker tidak tersedia (mode demo)',
      composeProfiles: { 'obs-full': false },
      checkedAt: BASE,
    }),
  ),
  http.get('*/api/v1/observe/grafana', () =>
    HttpResponse.json({
      url: 'http://127.0.0.1:3002',
      kioskUrl: 'http://127.0.0.1:3002/d/payment-overview?orgId=1&kiosk',
      dashboards: [
        { title: 'Payment Overview', uid: 'payment-overview' },
        { title: 'Provider Health', uid: 'provider-health' },
      ],
      embeddingAllowed: true,
    }),
  ),
  http.get('*/api/v1/observe/annotations', () =>
    HttpResponse.json({
      annotations: [
        { id: 1, time: BASE, text: 'gateway deploy good via lab', tags: ['deploy', 'gateway'] },
        { id: 2, time: BASE, text: 'traffic rate=6/s', tags: ['traffic'] },
      ],
    }),
  ),
  http.get('*/api/v1/observe/prometheus/alerts', () =>
    HttpResponse.json({
      alerts: [
        {
          labels: {
            alertname: 'PaySLOBurnRateHigh',
            slo: 'payment-success-rate',
            severity: 'page',
          },
          state: 'firing',
          activeAt: BASE,
        },
      ],
    }),
  ),
  http.get('*/api/v1/observe/prometheus/rules', () =>
    HttpResponse.json({
      groups: [{ name: 'payslo-payment-success-rate', rules: ['PaySLOBurnRateHigh'] }],
    }),
  ),
  http.get('*/api/v1/observe/alertmanager', () =>
    HttpResponse.json({
      alerts: [{ labels: { alertname: 'PaySLOBurnRateHigh', severity: 'page' }, state: 'active' }],
    }),
  ),
  http.get('*/api/v1/observe/ntfy/history', () =>
    HttpResponse.json({
      source: 'mock',
      messages: [
        {
          id: 'm1',
          time: BASE,
          topic: 'payslo-lab',
          message: 'incident opened: payment-success-rate (page)',
        },
      ],
    }),
  ),
  http.get('*/api/v1/diagnostics', () =>
    HttpResponse.json({
      services: [
        { name: 'labserve', version: 'dev-demo', reachable: true, config: {} },
        { name: 'grafana', version: 'unknown', reachable: true, config: {} },
        { name: 'prometheus', version: 'unknown', reachable: true, config: {} },
        { name: 'postgres', version: 'unknown', reachable: false, config: {} },
      ],
      docker: { available: false, note: 'Docker tidak tersedia (mode demo)' },
      checkedAt: BASE,
      generatedBy: 'labserve',
    }),
  ),

  // ------------------------------------------------------------------ U6 mentor + belajar
  http.post('*/api/v1/mentor/unlock', async ({ request }) => {
    const body = (await request.json()) as { pin?: string };
    if (body.pin !== '1234') return problem(401, 'PIN salah', 'PIN salah');
    mentorUnlocked = true;
    return HttpResponse.json({ ok: true, token: mentorToken });
  }),
  http.post('*/api/v1/mentor/lock', () => {
    mentorUnlocked = false;
    return HttpResponse.json({ ok: true });
  }),
  http.get('*/api/v1/mentor/timeline', ({ request }) => {
    if (!mentorUnlocked) return problem(403, 'Mode mentor diperlukan', 'PIN dulu');
    const url = new URL(request.url);
    const limit = Number(url.searchParams.get('limit') ?? '100');
    const entries = [
      { ts: BASE, source: 'lab', actor: 'trainee', action: 'routing.set', detail: 'bca=false' },
      {
        ts: '2026-10-16T07:01:00Z',
        source: 'payslo',
        actor: 'payslo',
        action: 'incident.open',
        detail: 'inc-x',
      },
      {
        ts: '2026-10-16T07:02:00Z',
        source: 'lab',
        actor: 'trainee',
        action: 'traffic.set',
        detail: '6',
      },
    ];
    return HttpResponse.json({ entries: entries.slice(0, limit) });
  }),
  http.post('*/api/v1/mentor/inject/traffic', async ({ request }) => {
    if (!mentorUnlocked) return problem(403, 'Mode mentor diperlukan', 'PIN dulu');
    const body = (await request.json()) as { ratePerSec: number };
    mockState.traffic = { ratePerSec: body.ratePerSec, source: 'loadgen', checkedAt: BASE };
    return HttpResponse.json(mockState.traffic);
  }),
  http.get('*/api/v1/mentor/snapshots', () => {
    if (!mentorUnlocked) return problem(403, 'Mode mentor diperlukan', 'PIN dulu');
    return HttpResponse.json({ snapshots: [{ name: 'demo-snap', savedAt: BASE, dataTables: 3 }] });
  }),
  http.post('*/api/v1/mentor/snapshot/save', async ({ request }) => {
    if (!mentorUnlocked) return problem(403, 'Mode mentor diperlukan', 'PIN dulu');
    const body = (await request.json()) as { name: string };
    return HttpResponse.json({ name: body.name, savedAt: BASE, dataTables: 3 });
  }),
  http.post('*/api/v1/mentor/snapshot/load', async ({ request }) => {
    if (!mentorUnlocked) return problem(403, 'Mode mentor diperlukan', 'PIN dulu');
    const body = (await request.json()) as { name: string };
    if (body.name !== 'demo-snap') return problem(404, 'Snapshot tidak ditemukan', 'tidak ada');
    return HttpResponse.json({ name: body.name, restored: ['routing', 'release', 'traffic'] });
  }),
  http.get('*/api/v1/mentor/sessions', () => {
    if (!mentorUnlocked) return problem(403, 'Mode mentor diperlukan', 'PIN dulu');
    return HttpResponse.json({
      sessions: [
        { scenarioId: '01', startedAt: BASE, endedAt: '2026-10-16T07:30:00Z' },
        { scenarioId: '02', startedAt: '2026-10-16T06:00:00Z', endedAt: null },
      ],
    });
  }),
  http.get('*/api/v1/mentor/session-summary', ({ request }) => {
    if (!mentorUnlocked) return problem(403, 'Mode mentor diperlukan', 'PIN dulu');
    const url = new URL(request.url);
    const scenario = url.searchParams.get('scenario') ?? '01';
    return HttpResponse.json({
      scenarioId: scenario,
      traineeName: 'Trainee Demo',
      startedAt: BASE,
      endedAt: '2026-10-16T07:30:00Z',
      ttdSecs: 60,
      actionCount: 3,
      answerCount: 1,
      hintsUsed: 1,
      actions: [{ ts: '2026-10-16T07:01:00Z', cmd: 'routing', detail: 'bca=false' }],
      answers: { q1: 'jawaban' },
      assessment: {
        scenarioId: scenario,
        communicationScore: 8,
        communicationNote: 'baik',
        postmortemScore: 7,
        postmortemNote: 'lengkap',
      },
    });
  }),
  http.get('*/api/v1/mentor/assessment', ({ request }) => {
    if (!mentorUnlocked) return problem(403, 'Mode mentor diperlukan', 'PIN dulu');
    const url = new URL(request.url);
    const scenario = url.searchParams.get('scenario') ?? '01';
    return HttpResponse.json({
      scenarioId: scenario,
      communicationScore: 8,
      communicationNote: 'baik',
      postmortemScore: 7,
      postmortemNote: 'lengkap',
    });
  }),
  http.put('*/api/v1/mentor/assessment', async ({ request }) => {
    if (!mentorUnlocked) return problem(403, 'Mode mentor diperlukan', 'PIN dulu');
    const url = new URL(request.url);
    const scenario = url.searchParams.get('scenario') ?? '01';
    const body = (await request.json()) as Record<string, unknown>;
    return HttpResponse.json({ scenarioId: scenario, ...body });
  }),
  http.get('*/api/v1/mentor/export', ({ request }) => {
    if (!mentorUnlocked) return problem(403, 'Mode mentor diperlukan', 'PIN dulu');
    const url = new URL(request.url);
    const scenario = url.searchParams.get('scenario') ?? '01';
    const format = url.searchParams.get('format') ?? 'markdown';
    if (format === 'json') {
      return HttpResponse.json({ scenarioId: scenario, traineeName: 'Trainee Demo' });
    }
    return new HttpResponse(
      `# Laporan Sesi Mentor — Skenario ${scenario}\n\n- Trainee: Trainee Demo\n`,
      {
        headers: { 'Content-Type': 'text/markdown; charset=utf-8' },
      },
    );
  }),

  // U6: belajar
  http.get('*/api/v1/learn/curriculum', () =>
    HttpResponse.json({
      modules: [
        {
          day: 'day-01',
          title: 'Hari 1 — Kenali Sistem',
          html: '<h1>Hari 1</h1>\n<p>Kenali sistem.</p>',
          items: [
            { key: 'aktivitas-1', label: 'Baca README', section: 'aktivitas' },
            { key: 'aktivitas-2', label: 'make up-lite', section: 'aktivitas' },
            { key: 'latihan-1', label: 'Kirim pembayaran', section: 'latihan' },
          ],
        },
        {
          day: 'day-02',
          title: 'Hari 2 — Golden Signals',
          html: '<h1>Hari 2</h1>\n<p>Golden signals.</p>',
          items: [{ key: 'aktivitas-1', label: 'Buka Grafana', section: 'aktivitas' }],
        },
      ],
    }),
  ),
  http.get('*/api/v1/learn/glossary', () =>
    HttpResponse.json({
      terms: [
        { term: 'SLI', definition: 'Service Level Indicator.' },
        { term: 'SLO', definition: 'Service Level Objective.' },
        { term: 'Error budget', definition: '1 - SLO.' },
      ],
    }),
  ),
  http.get('*/api/v1/learn/progress', () => {
    const items = [...learnProgressState.entries()]
      .filter(([, done]) => done)
      .map(([k]) => {
        const [module, key] = k.split(':');
        return { module, key, done: true };
      });
    return HttpResponse.json({ items });
  }),
  http.put('*/api/v1/learn/progress', async ({ request }) => {
    const body = (await request.json()) as { module: string; key: string; done: boolean };
    learnProgressState.set(`${body.module}:${body.key}`, body.done);
    const items = [...learnProgressState.entries()]
      .filter(([, done]) => done)
      .map(([k]) => {
        const [module, key] = k.split(':');
        return { module, key, done: true };
      });
    return HttpResponse.json({ items });
  }),
];

// ------------------------------------------------------------------ U4 fixtures

const INCIDENT_ID = 'inc-9f2c4a7e1b3d5a6c';

const incidentState: {
  timeline: Record<string, IncidentDetail['timeline']>;
  approvals: Approval[];
  seq: number;
  bcaDisabled: boolean;
  settings: { traineeName: string; lowSampleThreshold?: number };
  postmortem: Record<string, Postmortem>;
} = {
  timeline: {},
  approvals: [],
  seq: 0,
  bcaDisabled: false,
  settings: { traineeName: '' },
  postmortem: {},
};

function resetIncidentState(): void {
  incidentState.timeline = {
    [INCIDENT_ID]: [
      { ts: BASE, kind: 'opened', actor: 'payslo', message: 'incident opened by page alert' },
    ],
  };
  incidentState.approvals = [];
  incidentState.seq = 0;
  incidentState.bcaDisabled = false;
  incidentState.settings = { traineeName: '' };
  incidentState.postmortem = {};
}

resetIncidentState();

function isRemediationAction(action: string): boolean {
  return action === 'disable provider bca';
}

function incidentList(): IncidentDetail['incident'][] {
  return [
    {
      id: INCIDENT_ID,
      title: 'payment-success-rate on gateway-api (page)',
      slo: 'payment-success-rate',
      service: 'gateway-api',
      status: 'open',
      severity: 'page',
      createdAt: BASE,
    },
  ];
}

function incidentSteps(): IncidentDetail['runbook']['steps'] {
  const withApprovals = (action: string) =>
    incidentState.approvals.find((a) => a.action === action);
  const attach = (
    idx: number,
    name: string,
    kind: 'diagnostic' | 'remediation' | 'manual',
    action: string,
    extra?: Partial<IncidentDetail['runbook']['steps'][number]>,
  ) => ({
    idx,
    name,
    kind,
    action,
    requiresApproval: kind === 'remediation',
    approval: withApprovals(action),
    ...extra,
  });
  return [
    attach(0, 'success-rate per provider', 'diagnostic', 'success-rate per provider'),
    attach(1, 'profil bank-bca', 'diagnostic', 'profil bank-bca'),
    attach(2, 'routing gateway saat ini', 'diagnostic', 'routing gateway saat ini'),
    attach(3, 'catat observasi', 'manual', 'catat observasi', {
      detail: 'Buka Provider Health di Grafana.',
    }),
    attach(4, 'nonaktifkan provider bca', 'remediation', 'disable provider bca', {
      impact: 'Traffic VA dialihkan dari bca ke mandiri.',
    }),
  ];
}

function incidentDetailOf(id: string): IncidentDetail | undefined {
  if (id !== INCIDENT_ID) return undefined;
  const head = incidentList()[0];
  if (head === undefined) return undefined;
  const okPayload = {
    labels: {
      alertname: 'PaySLOBurnRateHigh',
      slo: 'payment-success-rate',
      service: 'gateway-api',
      severity: 'page',
    },
  } as unknown as IncidentDetail['alerts'][number]['payload'];
  const xssPayload = {
    labels: {
      alertname: '<script>alert(1)</script>',
      note: '<img src=x onerror=alert(2)>',
    },
  } as unknown as IncidentDetail['alerts'][number]['payload'];
  return {
    incident: head,
    timeline: incidentState.timeline[id] ?? [],
    alerts: [
      {
        fingerprint: 'fp-demo-1',
        source: 'prometheus',
        slo: 'payment-success-rate',
        service: 'gateway-api',
        severity: 'page',
        status: 'firing',
        payload: okPayload,
        receivedAt: BASE,
      },
      {
        fingerprint: 'fp-demo-xss',
        source: 'datadog',
        slo: 'payment-success-rate',
        service: 'gateway-api',
        severity: 'ticket',
        status: 'firing',
        payload: xssPayload,
        receivedAt: BASE,
      },
    ],
    runbook: { present: true, file: 'payment-success-rate.yml', steps: incidentSteps() },
    approvals: incidentState.approvals.filter((a) => a.incidentId === id),
  };
}

function postmortemOf(id: string): Postmortem {
  const saved = incidentState.postmortem[id];
  const actions = saved?.sections.actions ?? [];
  const sections = saved?.sections ?? {
    summary: '',
    impact: '',
    timeline: '',
    rootCause: '',
    wentWell: '',
    wentWrong: '',
    actions: [],
  };
  const lines = [
    `# Postmortem payment-success-rate on gateway-api (page) (${id})`,
    '',
    `- Status: ${saved?.status ?? 'draft'}`,
    '- SLO: payment-success-rate',
    '- Service: gateway-api',
    '- Severity: page',
    '- Status insiden: open',
    '',
    '## Ringkasan',
    '',
    sections.summary === '' ? '_belum diisi_' : sections.summary,
    '',
    '## Dampak',
    '',
    sections.impact === '' ? '_belum diisi_' : sections.impact,
    '',
    '## Kronologi',
    '',
    sections.timeline === '' ? '_belum diisi_' : sections.timeline,
    '',
    '## Akar masalah',
    '',
    sections.rootCause === '' ? '_belum diisi_' : sections.rootCause,
    '',
    '## Yang berjalan baik',
    '',
    sections.wentWell === '' ? '_belum diisi_' : sections.wentWell,
    '',
    '## Yang berjalan buruk',
    '',
    sections.wentWrong === '' ? '_belum diisi_' : sections.wentWrong,
    '',
    '## Action items',
    '',
    ...(actions.length === 0 ? ['_belum ada_'] : actions.map((a) => `- [ ] ${a}`)),
    '',
  ];
  return {
    incidentId: id,
    status: saved?.status ?? 'draft',
    sections,
    updatedAt: saved?.updatedAt ?? BASE,
    updatedBy: saved?.updatedBy ?? '',
    markdown: lines.join('\n'),
    exists: saved !== undefined,
  };
}

// ------------------------------------------------------------------ U3 fixtures

const SLO_SUCCESS = `apiVersion: payslo/v1
kind: SLO
name: payment-success-rate
service: gateway-api
description: Share of payments reaching final SUCCESS across all channels.
objective: 0.995
window: 1h
sli:
  type: success-ratio
  source: metric
  good: payment_transactions_total{status="SUCCESS"}
  total: payment_transactions_total
alerting:
  runbook: runbooks/payment-success.md
  burnRates:
    - longWindow: 5m
      shortWindow: 1m
      burnRate: 14.4
      severity: page
    - longWindow: 1h
      shortWindow: 10m
      burnRate: 2
      severity: ticket
targets:
  - backend: prometheus
  - backend: datadog
`;

const SLO_LATENCY = `apiVersion: payslo/v1
kind: SLO
name: payment-latency-p99
service: gateway-api
description: Share of gateway requests served within 500ms (Envoy upstream).
objective: 0.99
window: 1h
sli:
  type: latency
  source: metric
  good: envoy_cluster_upstream_rq_time_bucket{envoy_cluster_name="gateway-api",le="500"}
  total: envoy_cluster_upstream_rq_time_bucket{envoy_cluster_name="gateway-api",le="+Inf"}
alerting:
  runbook: runbooks/payment-latency.md
  burnRates:
    - longWindow: 15m
      shortWindow: 5m
      burnRate: 8
      severity: page
targets:
  - backend: prometheus
`;

const SLO_CONNS = [
  { name: 'prometheus-local', backend: 'prometheus' },
  { name: 'datadog-mock', backend: 'datadog' },
];

const SLO_CAPS = [
  {
    feature: 'recording rules',
    prometheus: 'yes (PromQL)',
    datadog: 'no (Datadog evaluates SLI itself)',
  },
  {
    feature: 'burn-rate alerts',
    prometheus: 'yes (MWMB, any window ≥1m)',
    datadog: 'yes (error_budget monitors, timeframe 7d/30d/90d only)',
  },
  {
    feature: 'objective windows',
    prometheus: '1m..90d',
    datadog: '7d/30d/90d only (lab windows mapped to 7d + warning)',
  },
  {
    feature: 'SLI source',
    prometheus: 'metric (PromQL, native)',
    datadog: 'metric (Datadog queries; PromQL stored opaquely, not evaluated)',
  },
  { feature: 'drift check', prometheus: 'yes (file hash)', datadog: 'yes (API content hash)' },
  {
    feature: 'atomic apply',
    prometheus: 'yes (temp + rename + reload)',
    datadog: 'yes (SLO first, then monitors; retry 429/5xx)',
  },
  {
    feature: 'runbook link',
    prometheus: 'yes (runbook_url annotation)',
    datadog: 'yes (message + tag)',
  },
];

function sloFixtures(): Record<string, string> {
  return { 'payment-success-rate': SLO_SUCCESS, 'payment-latency-p99': SLO_LATENCY };
}

const sloState: { files: Record<string, string>; applied: Record<string, string> } = {
  files: sloFixtures(),
  applied: {
    'payment-success-rate': SLO_SUCCESS,
    'payment-latency-p99': SLO_LATENCY.replace('burnRate: 8', 'burnRate: 6'),
  },
};

export function resetSloState(): void {
  sloState.files = sloFixtures();
  sloState.applied = {
    'payment-success-rate': SLO_SUCCESS,
    'payment-latency-p99': SLO_LATENCY.replace('burnRate: 8', 'burnRate: 6'),
  };
}

function lineOf(content: string, re: RegExp): number {
  const lines = content.split('\n');
  const i = lines.findIndex((l) => re.test(l));
  return i < 0 ? 1 : i + 1;
}

function validateSloText(
  content: string,
): { line: number; column: number; field?: string; message: string }[] {
  const errors: { line: number; column: number; field?: string; message: string }[] = [];
  const lines = content.split('\n');
  const topKeys = new Set<string>();
  for (const l of lines) {
    const m = /^([A-Za-z0-9_]+):/.exec(l);
    if (m !== null && m[1] !== undefined) topKeys.add(m[1]);
  }
  const allowed = new Set([
    'apiVersion',
    'kind',
    'name',
    'service',
    'description',
    'objective',
    'window',
    'sli',
    'alerting',
    'targets',
  ]);
  for (const k of topKeys) {
    if (!allowed.has(k)) {
      errors.push({
        line: lineOf(content, new RegExp(`^${k}:`)),
        column: 1,
        field: k,
        message: 'field tak dikenal (schema menolak additionalProperties)',
      });
    }
  }
  for (const k of [
    'apiVersion',
    'kind',
    'name',
    'service',
    'objective',
    'window',
    'sli',
    'alerting',
    'targets',
  ]) {
    if (!topKeys.has(k)) {
      errors.push({ line: 1, column: 1, field: k, message: `field wajib ${k} hilang` });
    }
  }
  const obj = /^objective:\s*(\S+)/m.exec(content);
  if (obj !== null) {
    const n = Number(obj[1]);
    if (!(n > 0 && n < 1)) {
      errors.push({
        line: lineOf(content, /^objective:/),
        column: 1,
        field: 'objective',
        message: 'harus angka di (0, 1), mis. 0.995',
      });
    }
  }
  const win = /^window:\s*(\S+)/m.exec(content);
  const winVal = win?.[1] ?? '';
  if (win !== null && !/^([0-9]+)([smhdw])$/.test(winVal) && winVal !== '0') {
    errors.push({
      line: lineOf(content, /^window:/),
      column: 1,
      field: 'window',
      message: 'durasi buruk; format <int><s|m|h|d|w>, mis. 1h atau 30d',
    });
  }
  return errors.slice(0, 20);
}

function sloMetaOf(name: string, content: string): SloDetail['meta'] {
  const pick = (re: RegExp): string => {
    const m = re.exec(content);
    return m?.[1]?.trim() ?? '';
  };
  const backends: string[] = [];
  for (const m of content.matchAll(/^ {2}- backend: (\w+)/gm)) {
    if (m[1] !== undefined) backends.push(m[1]);
  }
  return {
    name,
    service: pick(/^service:\s*(.+)$/m),
    objective: Number(pick(/^objective:\s*(\S+)$/m) || '0'),
    window: pick(/^window:\s*(\S+)$/m),
    file: `${name}.yml`,
    description: pick(/^description:\s*(.+)$/m),
    backends,
  };
}

function sloBurnsOf(content: string): SloDetail['burnRates'] {
  const out: SloDetail['burnRates'] = [];
  const lines = content.split('\n');
  let cur: {
    longWindow?: string;
    shortWindow?: string;
    burnRate?: number;
    severity?: string;
  } | null = null;
  const flush = (): void => {
    if (
      cur !== null &&
      cur.longWindow &&
      cur.shortWindow &&
      cur.burnRate !== undefined &&
      cur.severity
    ) {
      out.push({
        longWindow: cur.longWindow,
        shortWindow: cur.shortWindow,
        burnRate: cur.burnRate,
        severity: cur.severity as 'page' | 'ticket',
      });
    }
    cur = null;
  };
  for (const l of lines) {
    const lw = /^ {4}- longWindow:\s*(\S+)/.exec(l);
    if (lw !== null && lw[1] !== undefined) {
      flush();
      cur = { longWindow: lw[1] };
      continue;
    }
    if (cur === null) continue;
    const sw = /^ {6}shortWindow:\s*(\S+)/.exec(l);
    if (sw !== null && sw[1] !== undefined) {
      cur.shortWindow = sw[1];
      continue;
    }
    const br = /^ {6}burnRate:\s*(\S+)/.exec(l);
    if (br !== null && br[1] !== undefined) {
      cur.burnRate = Number(br[1]);
      continue;
    }
    const sv = /^ {6}severity:\s*(\S+)/.exec(l);
    if (sv !== null && sv[1] !== undefined) {
      cur.severity = sv[1];
      flush();
    }
  }
  flush();
  return out;
}

function sloSummaries(): SloDetail['meta'][] {
  return Object.keys(sloState.files)
    .sort()
    .map((n) => sloMetaOf(n, sloState.files[n] ?? ''));
}

function sloWarnings(name: string): string[] {
  const content = sloState.files[name] ?? '';
  if (content.includes('datadog')) {
    return [
      `warning [${name}:datadog]: datadog SLO timeframes are 7d/30d/90d only: window 1h mapped to 7d`,
    ];
  }
  return [];
}

function sloDetailOf(name: string): SloDetail | undefined {
  const content = sloState.files[name];
  if (content === undefined) return undefined;
  const meta = sloMetaOf(name, content);
  const pick = (re: RegExp): string => {
    const m = re.exec(content);
    return m?.[1]?.trim() ?? '';
  };
  const burns = sloBurnsOf(content);
  const changed = content !== sloState.applied[name];
  const deployment: SloDetail['deployment'] = SLO_CONNS.map((c) => ({
    connection: c.name,
    backend: c.backend,
    status:
      name === 'payment-latency-p99' && c.name === 'datadog-mock'
        ? 'drifted'
        : changed
          ? 'pending'
          : 'applied',
    detail:
      name === 'payment-latency-p99' && c.name === 'datadog-mock'
        ? 'backend berbeda dari apply terakhir (lihat drift)'
        : changed
          ? `would update ${name} on ${c.name}`
          : '',
  }));
  const per: Record<string, number> = { s: 1, m: 60, h: 3600, d: 86400, w: 604800 };
  const winMatch = /^([0-9]+)([smhdw])$/.exec(meta.window);
  const winUnit = winMatch?.[2] ?? 'h';
  const winNum = winMatch?.[1] ?? '1';
  const windowSecs = (per[winUnit] ?? 3600) * Number(winNum);
  const allowed = errorBudgetSecs(meta.objective, windowSecs);
  return {
    meta,
    content,
    sli: {
      type: pick(/^ {2}type:\s*(\S+)$/m),
      source: pick(/^ {2}source:\s*(\S+)$/m),
      good: pick(/^ {2}good:\s*(.+)$/m),
      total: pick(/^ {2}total:\s*(.+)$/m),
    },
    burnRates: burns,
    runbook: pick(/^ {2}runbook:\s*(\S+)$/m),
    targets: [...new Set(meta.backends)].map((b) => ({ backend: b })),
    deployment,
    sliCurrent: { status: 'ok', errorRatio: 0.002, sliValue: 0.998, lastDataAt: BASE },
    budget: { windowSecs, allowedSecs: allowed, allowedMinutes: allowed / 60 },
    alerts:
      name === 'payment-latency-p99'
        ? [
            {
              alert: 'PaySLOBurnRateHigh',
              state: 'firing',
              severity: 'page',
              window: '15m',
              activeAt: BASE,
            },
          ]
        : [],
  };
}

function sloPlanOf(name: string): SloPlan {
  const content = sloState.files[name] ?? '';
  const changed = content !== sloState.applied[name];
  const changes = changed
    ? SLO_CONNS.map((c) => ({ slo: name, connection: c.name, backend: c.backend }))
    : [];
  const files = SLO_CONNS.map((c) => ({
    connection: c.name,
    backend: c.backend,
    change: changed,
    desired:
      changed && c.backend === 'prometheus'
        ? `# rendered prometheus rules untuk ${name}\n${content}`
        : '',
    actual:
      changed && c.backend === 'prometheus'
        ? `# rules ter-apply sebelumnya\n${sloState.applied[name] ?? ''}`
        : '',
    diff: '',
    note:
      changed && c.backend === 'datadog'
        ? 'diff teks tak tersedia untuk backend datadog (perubahan terdeteksi via hash kanonik); lihat drift setelah apply'
        : '',
  }));
  return {
    slo: name,
    changes,
    warnings: changed ? sloWarnings(name) : [],
    files,
    raw: changed ? `${String(changes.length)} change(s) planned` : '0 change(s) planned',
  };
}
