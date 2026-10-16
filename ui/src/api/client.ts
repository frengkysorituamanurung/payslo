import createClient from 'openapi-fetch';
import type { paths } from './schema';

// U7: same-origin saja. Browser hanya bicara ke lab serve.
// window.location.origin absolut (di jsdom = http://localhost:3000) agar
// client juga jalan di Node (vitest/MSW); di browser = origin halaman.
// fetch dibungkus lazy: openapi-fetch menangkap referensi fetch saat
// createClient; tanpa wrapper, referensi pra-patch (MSW listen/HMR)
// dipakai selamanya dan intersepsi terlewat.
//
// U2: mutasi butuh X-Lab-Token (disuntikkan server ke window.__LAB_TOKEN__)
// dan X-Lab-Actor (aktor audit). Origin ditambahkan browser otomatis.
const baseUrl = typeof window === 'undefined' ? '' : window.location.origin;

let actor = 'trainee';
let mentorToken: string | null = null;

/** Dipanggil AppShell setelah sesi dimuat (mode trainee/mentor). */
export function setActor(next: string): void {
  actor = next;
}

/** Dipanggil setelah unlock/lock mentor; header dikirim di SEMUA request. */
export function setMentorToken(token: string | null): void {
  mentorToken = token;
}

function labToken(): string {
  if (typeof window === 'undefined') return '';
  return window.__LAB_TOKEN__ ?? '';
}

function methodOf(input: RequestInfo | URL, init?: RequestInit): string {
  if (init?.method) return init.method.toUpperCase();
  if (typeof Request !== 'undefined' && input instanceof Request) return input.method.toUpperCase();
  return 'GET';
}

const liveFetch: typeof fetch = (input, init) => {
  const method = methodOf(input, init);
  const headers = new Headers(
    init?.headers ??
      (typeof Request !== 'undefined' && input instanceof Request ? input.headers : undefined),
  );
  if (method !== 'GET' && method !== 'HEAD') {
    const token = labToken();
    if (token !== '') headers.set('X-Lab-Token', token);
    headers.set('X-Lab-Actor', actor);
  }
  // Token mentor dikirim di SEMUA request (termasuk GET) agar server bisa
  // memvalidasi mode mentor di endpoint /api/v1/mentor/* dan route /mentor.
  if (mentorToken !== null && mentorToken !== '') {
    headers.set('X-Mentor-Token', mentorToken);
  }
  return globalThis.fetch(input, { ...init, headers });
};

export const api = createClient<paths>({ baseUrl, fetch: liveFetch });

export type Session =
  paths['/api/v1/session']['get']['responses']['200']['content']['application/json'];
export type StackHealth =
  paths['/api/v1/health/stack']['get']['responses']['200']['content']['application/json'];
export type ServiceHealth = StackHealth['services'][number];

// U2.
export type PaymentSummary =
  paths['/api/v1/payments']['get']['responses']['200']['content']['application/json']['items'][number];
export type PaymentList =
  paths['/api/v1/payments']['get']['responses']['200']['content']['application/json'];
export type PaymentDetail =
  paths['/api/v1/payments/{id}']['get']['responses']['200']['content']['application/json'];
export type PaymentEvent = PaymentDetail['timeline'][number];
export type BankStatus =
  paths['/api/v1/payments/{id}/bank-status']['get']['responses']['200']['content']['application/json'];
export type CreatePaymentRequest =
  paths['/api/v1/payments']['post']['requestBody']['content']['application/json'];
export type TrafficState =
  paths['/api/v1/traffic']['get']['responses']['200']['content']['application/json'];
export type RoutingConfig =
  paths['/api/v1/routing']['get']['responses']['200']['content']['application/json'];
export type ProviderRoute = RoutingConfig['bca'];
export type ReleaseState =
  paths['/api/v1/release']['put']['responses']['200']['content']['application/json'];
export type AuditEntry =
  paths['/api/v1/audit']['get']['responses']['200']['content']['application/json']['entries'][number];
export type Problem =
  paths['/api/v1/session']['get']['responses']['500']['content']['application/problem+json'];
export type FieldError = NonNullable<Problem['errors']>[number];

// U3.
export type SloSummary =
  paths['/api/v1/slos']['get']['responses']['200']['content']['application/json']['slos'][number];
export type SloDetail =
  paths['/api/v1/slos/{name}']['get']['responses']['200']['content']['application/json'];
export type SloValidation =
  paths['/api/v1/slos/{name}/validate']['post']['responses']['200']['content']['application/json'];
export type SloPlan =
  paths['/api/v1/slos/{name}/plan']['post']['responses']['200']['content']['application/json'];
export type SloApply =
  paths['/api/v1/slos/{name}/apply']['post']['responses']['200']['content']['application/json'];
export type SloDrift =
  paths['/api/v1/drift']['get']['responses']['200']['content']['application/json'];
export type CapabilityMatrix =
  paths['/api/v1/capabilities']['get']['responses']['200']['content']['application/json'];
export type SloFileInfo =
  paths['/api/v1/slos/{name}']['put']['responses']['200']['content']['application/json'];

// U4.
export type Incident =
  paths['/api/v1/incidents']['get']['responses']['200']['content']['application/json']['incidents'][number];
export type IncidentDetail =
  paths['/api/v1/incidents/{id}']['get']['responses']['200']['content']['application/json'];
export type TimelineEntry = IncidentDetail['timeline'][number];
export type CorrelatedAlert = IncidentDetail['alerts'][number];
export type Approval = IncidentDetail['approvals'][number];
export type RunbookStep = IncidentDetail['runbook']['steps'][number];
export type StepOutput =
  paths['/api/v1/incidents/{id}/runbook/steps/{idx}/run']['post']['responses']['200']['content']['application/json'];
export type Postmortem =
  paths['/api/v1/incidents/{id}/postmortem']['get']['responses']['200']['content']['application/json'];
export type LabSettings =
  paths['/api/v1/settings']['get']['responses']['200']['content']['application/json'];

// U6 mentor.
export type MentorTimelineEntry =
  paths['/api/v1/mentor/timeline']['get']['responses']['200']['content']['application/json']['entries'][number];
export type SnapshotInfo =
  paths['/api/v1/mentor/snapshots']['get']['responses']['200']['content']['application/json']['snapshots'][number];
export type MentorSessionInfo =
  paths['/api/v1/mentor/sessions']['get']['responses']['200']['content']['application/json']['sessions'][number];
export type MentorSessionSummary =
  paths['/api/v1/mentor/session-summary']['get']['responses']['200']['content']['application/json'];
export type MentorAssessment =
  paths['/api/v1/mentor/assessment']['get']['responses']['200']['content']['application/json'];

// U6 belajar.
export type CurriculumModule =
  paths['/api/v1/learn/curriculum']['get']['responses']['200']['content']['application/json']['modules'][number];
export type ChecklistItem = CurriculumModule['items'][number];
export type GlossaryTerm =
  paths['/api/v1/learn/glossary']['get']['responses']['200']['content']['application/json']['terms'][number];
export type LearnProgressItem =
  paths['/api/v1/learn/progress']['get']['responses']['200']['content']['application/json']['items'][number];

// U7 observability + diagnostik.
export type ObserveStatus =
  paths['/api/v1/observe/status']['get']['responses']['200']['content']['application/json'];
export type ObserveService = ObserveStatus['services'][number];
export type GrafanaEmbedInfo =
  paths['/api/v1/observe/grafana']['get']['responses']['200']['content']['application/json'];
export type Annotation =
  paths['/api/v1/observe/annotations']['get']['responses']['200']['content']['application/json']['annotations'][number];
export type PrometheusAlert =
  paths['/api/v1/observe/prometheus/alerts']['get']['responses']['200']['content']['application/json']['alerts'][number];
export type PrometheusRuleGroup =
  paths['/api/v1/observe/prometheus/rules']['get']['responses']['200']['content']['application/json']['groups'][number];
export type AlertmanagerAlert =
  paths['/api/v1/observe/alertmanager']['get']['responses']['200']['content']['application/json']['alerts'][number];
export type NtfyHistory =
  paths['/api/v1/observe/ntfy/history']['get']['responses']['200']['content']['application/json'];
export type NtfyMessage = NtfyHistory['messages'][number];
export type DiagnosticsReport =
  paths['/api/v1/diagnostics']['get']['responses']['200']['content']['application/json'];

declare global {
  interface Window {
    __LAB_TOKEN__?: string;
  }
}
